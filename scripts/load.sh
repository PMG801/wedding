#!/usr/bin/env bash
# Runs scripts/load/upload.js with k6 (local binary if present, otherwise Docker).
#
# Required:
#   LOAD_BASE_URL            e.g. https://1.2.3.4 or http://localhost:8081
#   LOAD_EVENT_TOKEN         the BODA_EVENT_TOKEN of the target server
#
# Limits (all optional; the test stops at the first one reached):
#   LOAD_PHOTO_BYTES         size of each synthetic photo        (default 2097152)
#   LOAD_MAX_TOTAL_BYTES     byte budget sent in total           (default 104857600)
#   LOAD_VUS                 concurrent virtual guests           (default 5)
#   LOAD_MAX_DURATION        wall-clock cap, k6 duration         (default 5m)
#   LOAD_MAX_ERROR_RATE      abort above this unexpected-error rate (default 0.05)
#   LOAD_STOP_ON_STATUS      abort when the server answers any of these (default 507)
#   LOAD_IDEMPOTENT_RETRY    re-send each photo expecting 200    (default true)
#   LOAD_GALLERY_READS       gallery GETs per iteration          (default 1)
#
# Server-side limits (disk floor, max photo size, concurrency) are the BODA_*
# variables of the target; lower them there to make the server refuse early.
#
# Insecure/self-signed HTTPS: LOAD_INSECURE=1
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
: "${LOAD_BASE_URL:?LOAD_BASE_URL is required}"
: "${LOAD_EVENT_TOKEN:?LOAD_EVENT_TOKEN is required}"

k6_args=(run)
if [[ "${LOAD_INSECURE:-}" == 1 ]]; then k6_args+=(--insecure-skip-tls-verify); fi

if command -v k6 >/dev/null 2>&1; then
  exec k6 "${k6_args[@]}" "$repo_root/scripts/load/upload.js" "$@"
fi

env_args=()
while IFS='=' read -r name _; do
  if [[ "$name" == LOAD_* ]]; then env_args+=(-e "$name"); fi
done < <(env)

# --network host lets the container reach http://localhost:8081 from the compose stack.
exec docker run --rm -i --network host "${env_args[@]}" \
  -v "$repo_root/scripts/load:/load:ro" grafana/k6:1.0.0 \
  "${k6_args[@]}" /load/upload.js "$@"
