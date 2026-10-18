#!/usr/bin/env bash
# T5 wrapper: runs scripts/load/readers.js with k6 (local binary if present, otherwise
# the pinned grafana/k6 Docker image). Run it on the load generator, never on the server,
# and start it at the same time as scripts/load/slow-upload.sh (T3).
#
# Required:
#   LOAD_BASE_URL            explicit target, e.g. https://qa.example.org or http://192.168.1.50:8081
#   LOAD_EVENT_TOKEN         the BODA_EVENT_TOKEN of that target (never defaulted)
#
# Load shape (all optional):
#   LOAD_DURATION            how long both groups run, k6 duration       (default 10m)
#   LOAD_GALLERY_VUS         virtual users walking the gallery           (default 30)
#   LOAD_RANGE_VUS           virtual users issuing Range reads           (default 10)
#   LOAD_GALLERY_LIMIT       items per gallery page, 1-100               (default 24)
#   LOAD_GALLERY_PAGES       pages walked per gallery iteration          (default 5)
#   LOAD_RANGE_BYTES         bytes per Range request                     (default 1048576)
#   LOAD_RANGE_MAX_VIDEOS    seeded videos the range VUs may pick from   (default 50)
#
# Pass/fail (k6 exits 99 when a threshold fails; the summary is still written):
#   LOAD_GALLERY_P95_MS      p95 of gallery API requests, ms             (default 500)
#   LOAD_MAX_ERROR_RATE      abort when bad responses exceed this rate   (default 0.05)
#   A "bad response" is anything but 200 JSON from the gallery or 206 with the exact
#   Content-Range and Content-Length from a Range read.
#
# Output and misc:
#   LOAD_RUN_DIR             result folder (default loadtest/results/<UTC>-readers)
#   LOAD_INSECURE=1          accept self-signed HTTPS certificates
#   LOAD_DRY_RUN=1           k6 inspect only: validates the script, sends nothing
#
# Results (in LOAD_RUN_DIR): readers-summary.json (k6 summary export) and readers.log.
set -euo pipefail

usage() {
  sed -n '2,/^set -euo pipefail/p' "${BASH_SOURCE[0]}" | sed '$d' | sed 's/^# \{0,1\}//'
}
if [[ "${1:-}" == "--help" || "${1:-}" == "-h" ]]; then
  usage
  exit 0
fi

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
: "${LOAD_BASE_URL:?LOAD_BASE_URL is required (explicit target, see --help)}"
: "${LOAD_EVENT_TOKEN:?LOAD_EVENT_TOKEN is required (see --help)}"

run_dir="${LOAD_RUN_DIR:-$repo_root/loadtest/results/$(date -u +%Y%m%dT%H%M%SZ)-readers}"
script="$repo_root/scripts/load/readers.js"

k6_args=(run)
if [[ "${LOAD_INSECURE:-}" == 1 ]]; then k6_args+=(--insecure-skip-tls-verify); fi

# Names only (-e NAME): values reach the container through its environment, not argv.
env_args=()
while IFS='=' read -r name _; do
  if [[ "$name" == LOAD_* ]]; then env_args+=(-e "$name"); fi
done < <(env)

if [[ "${LOAD_DRY_RUN:-}" == 1 ]]; then
  # k6 inspect ignores the OS environment, so pass placeholders: this checks the
  # script and its options without contacting any target or printing the real token.
  dry_env=(-e LOAD_BASE_URL=http://dry-run.invalid -e LOAD_EVENT_TOKEN=dry-run)
  if command -v k6 >/dev/null 2>&1; then
    exec k6 inspect "${dry_env[@]}" "$script"
  fi
  exec docker run --rm -v "$repo_root/scripts/load:/load:ro" grafana/k6:1.0.0 inspect "${dry_env[@]}" /load/readers.js
fi

mkdir -p "$run_dir"
run_dir="$(cd "$run_dir" && pwd)"   # docker -v needs an absolute path
echo "results: $run_dir"

if command -v k6 >/dev/null 2>&1; then
  k6 "${k6_args[@]}" --summary-export "$run_dir/readers-summary.json" "$script" 2>&1 | tee "$run_dir/readers.log"
  exit "${PIPESTATUS[0]}"
fi

# --network host lets the container reach the target on this machine (e.g. http://localhost:8081).
# The results folder is mounted writable for the invoking user only.
docker run --rm --network host --user "$(id -u):$(id -g)" "${env_args[@]}" \
  -v "$repo_root/scripts/load:/load:ro" -v "$run_dir:/out" grafana/k6:1.0.0 \
  "${k6_args[@]}" --summary-export /out/readers-summary.json /load/readers.js 2>&1 | tee "$run_dir/readers.log"
exit "${PIPESTATUS[0]}"
