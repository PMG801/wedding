#!/usr/bin/env bash
# T3 driver: N concurrent slow video uploads through the real guest flow.
#
# Each worker enters through /e/{token} (guest cookie), PUTs one synthetic MP4
# to /api/media/{uuid} with an explicit Content-Length, then optionally re-PUTs
# the same id expecting 200 (idempotent retry). curl streams the body from one
# sparse file on this machine (--upload-file) and throttles it per connection
# (--limit-rate), so memory stays flat whatever the size of the files.
#
# Run it on the load generator, never on the server.
#
# Required:
#   LOAD_BASE_URL            explicit target, e.g. https://qa.example.org or http://192.168.1.50:8081
#   LOAD_EVENT_TOKEN         the BODA_EVENT_TOKEN of that target (never defaulted)
#
# Plan (all optional):
#   LOAD_SLOW_COUNT          concurrent uploads                          (default 25)
#   LOAD_SLOW_BYTES          size of each video; K/M/G suffixes allowed  (default 200M)
#   LOAD_SLOW_MBPS           per-connection rate limit in Mbit/s         (default 3)
#   LOAD_SLOW_RETRY          re-PUT each video expecting 200 (1/0)       (default 0)
#   LOAD_SLOW_TIMEOUT        per-request time cap in seconds             (default 1800)
#
# Safety caps (the run refuses to start, or stops early, instead of overrunning):
#   LOAD_MAX_TOTAL_BYTES     byte budget for the whole run; the planned bytes
#                            (count x size x (1 + retry)) must fit     (default 12G)
#   LOAD_MAX_DURATION        wall-clock cap in seconds; workers that would start
#                            after it are recorded as skipped          (default 5400)
#   LOAD_STOP_ON_STATUS      stop starting new requests once the server answers
#                            any of these codes                        (default 507)
#
# Output and misc:
#   LOAD_RUN_DIR             result folder (default loadtest/results/<UTC>-upload-slow)
#   LOAD_PAYLOAD_DIR         where the sparse synthetic file lives      (default loadtest/fixtures)
#   LOAD_INSECURE=1          accept self-signed HTTPS certificates
#   LOAD_DRY_RUN=1           print the plan and build the payload, send nothing
#
# Results (in LOAD_RUN_DIR):
#   uploads.csv   one line per request: timestamp,index,id,step,http_code,seconds,bytes,bytes_per_s,note
#   summary.txt   counts by step and status, p50/p95/max of successful uploads, failures
#   params.txt    the plan that was run (the token is never written)
#
# Note: the server still writes every byte it accepts, so each video consumes disk
# on the target (count x size). Check free space against BODA_DISK_MIN_FREE_BYTES first.
set -euo pipefail

usage() {
  sed -n '2,/^set -euo pipefail/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'
}

if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
self="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)/$(basename "${BASH_SOURCE[0]}")"

fail() {
  echo "slow-upload: $*" >&2
  exit 1
}

# parse_bytes 200M -> 209715200. Accepts plain bytes or K/M/G with optional B/iB.
parse_bytes() {
  local raw
  raw="$(printf '%s' "$1" | tr '[:lower:]' '[:upper:]')"
  raw="${raw%B}"
  raw="${raw%I}"
  if [[ ! "$raw" =~ ^([0-9]+(\.[0-9]+)?)([KMG]?)$ ]]; then
    return 1
  fi
  awk -v n="${BASH_REMATCH[1]}" -v u="${BASH_REMATCH[3]}" 'BEGIN {
    m = 1
    if (u == "K") m = 1024
    else if (u == "M") m = 1048576
    else if (u == "G") m = 1073741824
    printf "%.0f", n * m
  }'
}

positive_int() { [[ "$2" =~ ^[0-9]+$ ]] && (( $2 > 0 )) || fail "$1 must be a positive integer, got \"$2\""; }

new_uuid() {
  if [[ -r /proc/sys/kernel/random/uuid ]]; then
    tr 'A-Z' 'a-z' </proc/sys/kernel/random/uuid
  elif command -v uuidgen >/dev/null 2>&1; then
    uuidgen | tr 'A-Z' 'a-z'
  else
    python3 -c 'import uuid; print(uuid.uuid4())'
  fi
}

# record <index> <id> <step> <http_code> <seconds> <bytes> <bytes_per_s> <note>
record() {
  local note="${8//,/ }"
  printf '%s,%s,%s,%s,%s,%s,%s,%s,%s\n' "$(date -u +%FT%TZ)" "$1" "$2" "$3" "$4" "$5" "$6" "$7" "$note" >>"$RUN_DIR/uploads.csv"
}

is_stop_status() {
  local code="$1" s
  IFS=',' read -r -a stop_codes <<<"$STOP_ON_STATUS"
  for s in "${stop_codes[@]}"; do
    [[ "$s" == "$code" ]] && return 0
  done
  return 1
}

worker() {
  local idx="$1" id out code cookie body_file error_note remaining max_time
  local -a insecure=()
  local PUT_CODE=""
  if [[ "${LOAD_INSECURE:-}" == 1 ]]; then insecure=(-k); fi
  id="$(new_uuid)"

  if [[ -e "$RUN_DIR/STOP" ]]; then
    record "$idx" "$id" enter skipped 0 0 0 stop_flag_set
    return 0
  fi

  # 1. Guest entry, exactly as a phone scanning the QR does.
  out="$(curl -sS "${insecure[@]+"${insecure[@]}"}" -D - -o /dev/null -w '%{http_code}' \
    --max-time 30 "$LOAD_BASE_URL/e/$LOAD_EVENT_TOKEN" 2>/dev/null || true)"
  code="${out##*$'\n'}"
  cookie="$(printf '%s\n' "$out" | tr -d '\r' | sed -n 's/^[Ss]et-[Cc]ookie: guest_session=\([^;]*\).*/\1/p' | head -n1)"
  out=""
  if [[ "$code" != 303 || -z "$cookie" ]]; then
    record "$idx" "$id" enter "$code" 0 0 0 no_guest_cookie
    if is_stop_status "$code"; then : >"$RUN_DIR/STOP"; fi
    return 0
  fi

  local step
  for step in put retry; do
    if [[ "$step" == retry ]]; then
      if [[ "$LOAD_SLOW_RETRY" != 1 || "$PUT_CODE" != 201 ]]; then break; fi
    fi
    if [[ -e "$RUN_DIR/STOP" ]]; then
      record "$idx" "$id" "$step" skipped 0 0 0 stop_flag_set
      return 0
    fi
    remaining=$(( DEADLINE - $(date +%s) ))
    if (( remaining <= 0 )); then
      record "$idx" "$id" "$step" skipped 0 0 0 max_duration_reached
      return 0
    fi
    max_time=$(( remaining < LOAD_SLOW_TIMEOUT ? remaining : LOAD_SLOW_TIMEOUT ))

    body_file="$(mktemp "${TMPDIR:-/tmp}/slow-upload-body.XXXXXX")"
    out="$(curl -sS "${insecure[@]+"${insecure[@]}"}" -o "$body_file" \
      -w '%{http_code} %{time_total} %{size_upload} %{speed_upload}' \
      --max-time "$max_time" --limit-rate "$RATE_BPS" \
      -X PUT \
      -H "Cookie: guest_session=$cookie" \
      -H 'Content-Type: video/mp4' \
      -H "Content-Length: $LOAD_SLOW_BYTES" \
      --upload-file "$PAYLOAD" \
      "$LOAD_BASE_URL/api/media/$id" 2>/dev/null || true)"
    read -r code seconds sent speed <<<"$out" || true
    error_note=""
    if [[ "$code" != 201 && "$code" != 200 ]]; then
      error_note="$(sed -n 's/.*"error":"\([^"]*\)".*/\1/p' "$body_file" 2>/dev/null | head -c 80)"
    fi
    rm -f -- "$body_file"

    record "$idx" "$id" "$step" "${code:-000}" "${seconds:-0}" "${sent:-0}" "${speed:-0}" "$error_note"
    if [[ "$step" == put ]]; then PUT_CODE="$code"; fi
    if is_stop_status "$code"; then : >"$RUN_DIR/STOP"; fi
    # A non-2xx first PUT means there is nothing to retry.
    if [[ "$step" == put && "$code" != 201 ]]; then break; fi
  done
  return 0
}

summary() {
  local csv="$RUN_DIR/uploads.csv" out="$RUN_DIR/summary.txt"
  {
    echo "T3 slow uploads: $(date -u +%FT%TZ)"
    echo "target: $LOAD_BASE_URL (token redacted)"
    echo "plan: $LOAD_SLOW_COUNT x $LOAD_SLOW_BYTES bytes, ${LOAD_SLOW_MBPS} Mbit/s per connection, retry=$LOAD_SLOW_RETRY"
    echo
    echo "counts by step and http_code:"
    awk -F, 'NR > 1 { c[$4 " " $5]++ } END { for (k in c) print "  " k ": " c[k] }' "$csv" | sort
    echo
    echo "successful first PUTs (201):"
    awk -F, 'NR > 1 && $4 == "put" && $5 == 201 { n++; b += $7 } END { printf "  count=%d bytes=%d\n", n + 0, b + 0 }' "$csv"
    awk -F, 'NR > 1 && $4 == "put" && $5 == 201 { print $6 }' "$csv" | sort -n | awk '
      { a[NR] = $1 }
      END {
        if (NR == 0) { print "  no successful uploads"; exit }
        i50 = int(0.5 * NR); if (i50 < 0.5 * NR) i50++; if (i50 < 1) i50 = 1
        i95 = int(0.95 * NR); if (i95 < 0.95 * NR) i95++; if (i95 < 1) i95 = 1
        printf "  seconds p50=%.1f p95=%.1f max=%.1f\n", a[i50], a[i95], a[NR]
      }'
    echo
    echo "idempotent re-PUT (expects 200):"
    awk -F, 'NR > 1 && $4 == "retry" { c[$5]++ } END { for (k in c) print "  " k ": " c[k] }' "$csv" | sort
    echo "  re-PUT answered before the whole body was sent (bytes < size): $(awk -F, -v want="$LOAD_SLOW_BYTES" 'NR > 1 && $4 == "retry" && $7 + 0 < want + 0 { n++ } END { print n + 0 }' "$csv")"
    echo
    echo "non-success rows (step, code, note):"
    awk -F, 'NR > 1 && !($4 == "put" && $5 == 201) && !($4 == "retry" && $5 == 200) { print "  " $4 ", " $5 ", " $9 }' "$csv" | sort | uniq -c
    if [[ -e "$RUN_DIR/STOP" ]]; then echo; echo "STOP flag was set: a server status in LOAD_STOP_ON_STATUS ended the run early."; fi
  } | tee "$out"
}

# Worker mode: invoked by xargs with the same environment.
if [[ "${1:-}" == "__worker" ]]; then
  worker "$2"
  exit 0
fi

# ---- configuration and validation -------------------------------------------
: "${LOAD_BASE_URL:?LOAD_BASE_URL is required (explicit target, see --help)}"
: "${LOAD_EVENT_TOKEN:?LOAD_EVENT_TOKEN is required (see --help)}"
LOAD_BASE_URL="${LOAD_BASE_URL%/}"
[[ "$LOAD_BASE_URL" =~ ^https?://[^[:space:]]+$ ]] || fail "LOAD_BASE_URL must start with http:// or https://"

LOAD_SLOW_COUNT="${LOAD_SLOW_COUNT:-25}"
LOAD_SLOW_BYTES_RAW="${LOAD_SLOW_BYTES:-200M}"
LOAD_SLOW_MBPS="${LOAD_SLOW_MBPS:-3}"
LOAD_SLOW_RETRY="${LOAD_SLOW_RETRY:-0}"
LOAD_SLOW_TIMEOUT="${LOAD_SLOW_TIMEOUT:-1800}"
LOAD_MAX_TOTAL_BYTES_RAW="${LOAD_MAX_TOTAL_BYTES:-12G}"
LOAD_MAX_DURATION="${LOAD_MAX_DURATION:-5400}"
STOP_ON_STATUS="${LOAD_STOP_ON_STATUS:-507}"

positive_int LOAD_SLOW_COUNT "$LOAD_SLOW_COUNT"
positive_int LOAD_SLOW_TIMEOUT "$LOAD_SLOW_TIMEOUT"
positive_int LOAD_MAX_DURATION "$LOAD_MAX_DURATION"
[[ "$LOAD_SLOW_RETRY" == 0 || "$LOAD_SLOW_RETRY" == 1 ]] || fail "LOAD_SLOW_RETRY must be 0 or 1"
LOAD_SLOW_BYTES="$(parse_bytes "$LOAD_SLOW_BYTES_RAW")" || fail "LOAD_SLOW_BYTES is not a size: \"$LOAD_SLOW_BYTES_RAW\""
LOAD_MAX_TOTAL_BYTES="$(parse_bytes "$LOAD_MAX_TOTAL_BYTES_RAW")" || fail "LOAD_MAX_TOTAL_BYTES is not a size: \"$LOAD_MAX_TOTAL_BYTES_RAW\""
(( LOAD_SLOW_BYTES >= 1048576 )) || fail "LOAD_SLOW_BYTES must be at least 1M"
RATE_BPS="$(awk -v m="$LOAD_SLOW_MBPS" 'BEGIN { if (m + 0 > 0) printf "%d", m * 125000; else printf "0" }')"
(( RATE_BPS > 0 )) || fail "LOAD_SLOW_MBPS must be a positive number, got \"$LOAD_SLOW_MBPS\""

PLANNED_BYTES=$(( LOAD_SLOW_COUNT * LOAD_SLOW_BYTES * (1 + LOAD_SLOW_RETRY) ))
if (( PLANNED_BYTES > LOAD_MAX_TOTAL_BYTES )); then
  fail "planned bytes $PLANNED_BYTES exceed LOAD_MAX_TOTAL_BYTES=$LOAD_MAX_TOTAL_BYTES; lower the count or size, or raise the budget deliberately"
fi

PAYLOAD_DIR="${LOAD_PAYLOAD_DIR:-$repo_root/loadtest/fixtures}"
PAYLOAD="$PAYLOAD_DIR/synthetic-isom-$LOAD_SLOW_BYTES.mp4"
RUN_DIR="${LOAD_RUN_DIR:-$repo_root/loadtest/results/$(date -u +%Y%m%dT%H%M%SZ)-upload-slow}"
DEADLINE=$(( $(date +%s) + LOAD_MAX_DURATION ))

echo "plan: $LOAD_SLOW_COUNT uploads x $LOAD_SLOW_BYTES bytes at ${LOAD_SLOW_MBPS} Mbit/s (${RATE_BPS} B/s), retry=$LOAD_SLOW_RETRY, planned ${PLANNED_BYTES} bytes (budget ${LOAD_MAX_TOTAL_BYTES}), max duration ${LOAD_MAX_DURATION}s, stop on [${STOP_ON_STATUS}]"

# ---- synthetic payload: ftyp box (brand isom) + sparse filler, streamed from disk ----
mkdir -p "$PAYLOAD_DIR"
if [[ ! -s "$PAYLOAD" || "$(wc -c <"$PAYLOAD")" -ne "$LOAD_SLOW_BYTES" ]]; then
  # dd with count=0 seek=N creates a sparse file of N bytes without writing data.
  dd if=/dev/null of="$PAYLOAD" bs=1 count=0 seek="$LOAD_SLOW_BYTES" status=none
  # 28-byte ftyp box: size 0x1c, type 'ftyp', major brand 'isom', minor 0x200, compatible brands isom/iso2/mp41.
  printf '\x00\x00\x00\x1cftypisom\x00\x00\x02\x00isomiso2mp41' | dd of="$PAYLOAD" bs=28 count=1 conv=notrunc status=none
fi
[[ "$(head -c 8 "$PAYLOAD" | tail -c 4)" == ftyp ]] || fail "payload $PAYLOAD does not start with an ftyp box"

if [[ "${LOAD_DRY_RUN:-}" == 1 ]]; then
  echo "dry run: payload ready at $PAYLOAD ($(wc -c <"$PAYLOAD") bytes); nothing was sent"
  exit 0
fi

mkdir -p "$RUN_DIR"
{
  echo "started_utc=$(date -u +%FT%TZ)"
  echo "base_url=$LOAD_BASE_URL"
  echo "token=redacted"
  echo "count=$LOAD_SLOW_COUNT"
  echo "bytes=$LOAD_SLOW_BYTES"
  echo "mbps_per_connection=$LOAD_SLOW_MBPS"
  echo "rate_bytes_per_s=$RATE_BPS"
  echo "retry=$LOAD_SLOW_RETRY"
  echo "timeout_s=$LOAD_SLOW_TIMEOUT"
  echo "max_duration_s=$LOAD_MAX_DURATION"
  echo "budget_bytes=$LOAD_MAX_TOTAL_BYTES"
  echo "planned_bytes=$PLANNED_BYTES"
  echo "stop_on_status=$STOP_ON_STATUS"
  echo "payload=$PAYLOAD"
} >"$RUN_DIR/params.txt"
printf 'timestamp,index,id,step,http_code,seconds,bytes,bytes_per_s,note\n' >"$RUN_DIR/uploads.csv"

export LOAD_BASE_URL LOAD_EVENT_TOKEN LOAD_SLOW_BYTES LOAD_SLOW_RETRY LOAD_SLOW_TIMEOUT
export LOAD_INSECURE="${LOAD_INSECURE:-}" RATE_BPS STOP_ON_STATUS PAYLOAD RUN_DIR DEADLINE

echo "running in $RUN_DIR"
start_epoch="$(date +%s)"
seq 1 "$LOAD_SLOW_COUNT" | xargs -P "$LOAD_SLOW_COUNT" -I{} bash "$self" __worker {} || true
echo "wall clock: $(( $(date +%s) - start_epoch )) s"

summary
