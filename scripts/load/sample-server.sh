#!/usr/bin/env bash
# Server-side sampler for T3/T5 (run ON the QA host during the test, read-only).
#
# Every interval it appends one line to samples.csv with:
#   - docker stats (CPU %, memory MiB) of the compose services "app" and "web"
#   - vmstat: CPU us/sy/id and I/O wait wa (1 s sample)
#   - iostat -dx: highest %util across disks (only when iostat is installed)
#   - df of the data volume: available MiB and used %
#   - number of entries in <data>/tmp (staged uploads; should drain when idle)
# The raw iostat output is kept in iostat.log.
#
# It only reads: it never writes to the data directory, never restarts or stops
# containers, and writes only inside its own output folder.
#
# Usage (on the server, e.g. from /srv/boda):
#   scripts/load/sample-server.sh --data-dir /srv/boda/data --interval 15
#   scripts/load/sample-server.sh --once --data-dir /srv/boda/data   # one sample, then exit
#
# Options (environment variables in parentheses):
#   --project NAME     only containers of this compose project (LOAD_SAMPLE_PROJECT, default: any)
#   --data-dir DIR     host folder mounted as /srv/evento (LOAD_SAMPLE_DATA_DIR, default /srv/boda/data)
#   --interval SEC     seconds between samples             (LOAD_SAMPLE_INTERVAL, default 15)
#   --duration SEC     stop after this long, safety cap    (LOAD_SAMPLE_DURATION, default 10800)
#   --out DIR          output folder                       (LOAD_SAMPLE_OUT, default ./loadtest-server-samples-<UTC>)
#   --once             take a single sample and exit
#   -h, --help         show this help
#
# Stop it earlier with Ctrl-C; the CSV is appended line by line, so nothing is lost.
# Reading /srv/boda/data/tmp usually needs sudo (the directory belongs to the app UID);
# without it the tmp column reads "n/a" and the rest still works.
set -euo pipefail

usage() {
  sed -n '2,/^set -euo pipefail/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'
}

data_dir="${LOAD_SAMPLE_DATA_DIR:-/srv/boda/data}"
interval="${LOAD_SAMPLE_INTERVAL:-15}"
duration="${LOAD_SAMPLE_DURATION:-10800}"
out_dir="${LOAD_SAMPLE_OUT:-./loadtest-server-samples-$(date -u +%Y%m%dT%H%M%SZ)}"
project="${LOAD_SAMPLE_PROJECT:-}"
once=0

while (( $# > 0 )); do
  case "$1" in
    --data-dir) data_dir="$2"; shift 2 ;;
    --interval) interval="$2"; shift 2 ;;
    --duration) duration="$2"; shift 2 ;;
    --out) out_dir="$2"; shift 2 ;;
    --project) project="$2"; shift 2 ;;
    --once) once=1; shift ;;
    -h|--help) usage; exit 0 ;;
    *) echo "sample-server: unknown option $1 (see --help)" >&2; exit 2 ;;
  esac
done

[[ "$interval" =~ ^[0-9]+$ ]] && (( interval >= 1 )) || { echo "sample-server: --interval must be a positive integer" >&2; exit 2; }
[[ "$duration" =~ ^[0-9]+$ ]] && (( duration >= 1 )) || { echo "sample-server: --duration must be a positive integer" >&2; exit 2; }

# to_mib 123.4MiB -> 123.4 ; understands B, kB/KiB, MB/MiB, GB/GiB, TB/TiB.
to_mib() {
  awk -v v="$1" 'BEGIN {
    n = v + 0; u = v; sub(/^[0-9.]+/, "", u)
    if (u == "B") m = 1 / 1048576
    else if (u == "kB" || u == "KB" || u == "KiB") m = 1 / 1024
    else if (u == "MB" || u == "MiB") m = 1
    else if (u == "GB" || u == "GiB") m = 1024
    else if (u == "TB" || u == "TiB") m = 1048576
    else { print ""; exit }
    printf "%.1f", n * m
  }'
}

mkdir -p "$out_dir"
csv="$out_dir/samples.csv"
iostat_log="$out_dir/iostat.log"

project_filter=()
if [[ -n "$project" ]]; then project_filter=(--filter "label=com.docker.compose.project=$project"); fi
app_id="$(docker ps -q ${project_filter[@]+"${project_filter[@]}"} --filter label=com.docker.compose.service=app 2>/dev/null | head -n1 || true)"
web_id="$(docker ps -q ${project_filter[@]+"${project_filter[@]}"} --filter label=com.docker.compose.service=web 2>/dev/null | head -n1 || true)"
[[ -n "$app_id" ]] || echo "sample-server: no running 'app' container found; app columns will be empty" >&2
[[ -n "$web_id" ]] || echo "sample-server: no running 'web' container found; web columns will be empty" >&2
[[ -d "$data_dir" ]] || echo "sample-server: data dir $data_dir not found; df/tmp columns will be empty" >&2

if [[ ! -s "$csv" ]]; then
  echo 'timestamp,app_cpu_pct,app_mem_mib,web_cpu_pct,web_mem_mib,vm_us,vm_sy,vm_id,vm_wa,disk_max_util_pct,data_avail_mib,data_used_pct,tmp_entries' >"$csv"
fi

ids=()
[[ -n "$app_id" ]] && ids+=("$app_id")
[[ -n "$web_id" ]] && ids+=("$web_id")

# One docker stats call for both services (it takes about a second per call).
stats=""
if (( ${#ids[@]} > 0 )); then
  stats="$(docker stats --no-stream --format '{{.Container}}|{{.CPUPerc}}|{{.MemUsage}}' "${ids[@]}" 2>/dev/null || true)"
fi

# stat_for <container id> -> "cpu_pct,mem_mib" or ",".
stat_for() {
  local id="$1" line cpu mem
  if [[ -z "$id" ]]; then echo ","; return; fi
  line="$(awk -F'|' -v id="$id" 'NF == 3 && index(id, $1) == 1 { print; exit }' <<<"$stats")"
  if [[ -z "$line" ]]; then echo ","; return; fi
  IFS='|' read -r _ cpu mem <<<"$line"
  cpu="${cpu%\%}"
  mem="${mem%% / *}"
  printf '%s,%s\n' "$cpu" "$(to_mib "$mem")"
}

sample_once() {
  local ts app web vm disk iostat_out avail used tmp df_line
  ts="$(date -u +%FT%TZ)"

  app="$(stat_for "$app_id")"
  web="$(stat_for "$web_id")"

  # vmstat 1 2: the second data row is the 1-second sample (the first is since boot).
  vm="$(vmstat 1 2 2>/dev/null | awk '
    /(^| )us( |$)/ && /wa/ { for (i = 1; i <= NF; i++) col[$i] = i; next }
    col["wa"] && NF > 0 { row = $0 }
    END {
      if (row == "") { printf ",,,"; exit }
      n = split(row, f, " ")
      printf "%s,%s,%s,%s", f[col["us"]], f[col["sy"]], f[col["id"]], f[col["wa"]]
    }' || true)"
  [[ -n "$vm" ]] || vm=",,,"

  # iostat -dx 1 2: keep the raw report, and the highest %util of the last (1 s) report.
  disk=""
  if command -v iostat >/dev/null 2>&1; then
    iostat_out="$(iostat -dx 1 2 2>/dev/null || true)"
    printf '== %s\n%s\n' "$ts" "$iostat_out" >>"$iostat_log"
    disk="$(awk '
      /%util/ { for (i = 1; i <= NF; i++) if ($i == "%util") u = i; max = ""; next }
      u && NF > 0 && $u ~ /^[0-9.]+$/ { if (max == "" || $u + 0 > max + 0) max = $u }
      END { print max }' <<<"$iostat_out")"
  fi

  avail=""; used=""
  if [[ -d "$data_dir" ]]; then
    df_line="$(df -Pk "$data_dir" 2>/dev/null | awk 'NR == 2 { print $4, $5 }' || true)"
    if [[ -n "$df_line" ]]; then
      avail="$(awk '{ printf "%.0f", $1 / 1024 }' <<<"$df_line")"
      used="$(awk '{ sub(/%/, "", $2); print $2 }' <<<"$df_line")"
    fi
  fi

  # An unreadable tmp/ (owned by the app UID) reads as n/a, never as 0.
  tmp="n/a"
  if [[ -d "$data_dir/tmp" ]] && ls -1A "$data_dir/tmp" >/dev/null 2>&1; then
    tmp="$(ls -1A "$data_dir/tmp" | wc -l | tr -d ' ')"
  fi

  printf '%s,%s,%s,%s,%s,%s,%s,%s\n' "$ts" "$app" "$web" "$vm" "$disk" "$avail" "$used" "$tmp" >>"$csv"
}

echo "sampler: writing to $out_dir (data dir: $data_dir)"

if (( once == 1 )); then
  sample_once
  cat "$csv"
  exit 0
fi

trap 'echo; echo "sampler: stopped; samples in $csv"; exit 0' INT TERM
start="$(date +%s)"
while :; do
  sample_once
  if (( $(date +%s) - start >= duration )); then
    echo "sampler: duration cap ${duration}s reached; samples in $csv"
    break
  fi
  sleep "$interval"
done
