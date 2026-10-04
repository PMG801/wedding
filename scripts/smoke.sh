#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "$repo_root"

smoke_env_file="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke.XXXXXX")"
chmod 600 "$smoke_env_file"
cat >"$smoke_env_file" <<'EOF'
BODA_ADDR=:8080
BODA_DATA_DIR=/tmp
BODA_EVENT_TOKEN=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
BODA_ADMIN_PASSWORD_HASH='$argon2id$v=19$m=1,t=1,p=1$YQ$YQ'
BODA_SESSION_KEY=0123456789abcdef0123456789abcdef
EOF
export BODA_ENV_FILE="$smoke_env_file"
export BODA_DOMAIN=:80

cleanup() {
  local status=$?
  trap - EXIT

  if (( status != 0 )); then
    echo "Smoke test failed; printing Docker Compose logs:" >&2
    docker compose logs || true
  fi

  docker compose down -v || {
    local cleanup_status=$?
    if (( status == 0 )); then
      status=$cleanup_status
    fi
  }
  rm -f -- "$smoke_env_file"
  exit "$status"
}
trap cleanup EXIT

docker compose up -d --build --wait

health_ok=false
for ((attempt = 1; attempt <= 30; attempt++)); do
  if health_response="$(curl -fsS http://localhost:8081/api/health 2>/dev/null)" && [[ "$health_response" == *'"status":"ok"'* ]]; then
    health_ok=true
    break
  fi
  if (( attempt < 30 )); then
    sleep 1
  fi
done
if [[ "$health_ok" != true ]]; then
  echo 'Health endpoint did not return a body containing "status":"ok" within 30 attempts.' >&2
  exit 1
fi

echo 'Health endpoint passed.'

homepage="$(curl -fsS http://localhost:8081/)"
if [[ "$homepage" != *'<div id="app"'* ]]; then
  echo 'Homepage did not contain the frontend mount element <div id="app".' >&2
  exit 1
fi

echo 'Homepage mount element passed.'
