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
BODA_DISK_MIN_FREE_BYTES=1
EOF
export BODA_ENV_FILE="$smoke_env_file"
export BODA_DOMAIN=:80
smoke_base_url=http://localhost:8081
smoke_event_token=AAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAAA
smoke_photo_id=123e4567-e89b-42d3-a456-426614174000
smoke_photo_file="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke-photo.XXXXXX")"
smoke_guest_headers="$(mktemp "${TMPDIR:-/tmp}/wedding-smoke-guest.XXXXXX")"

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
  rm -f -- "$smoke_env_file" "$smoke_photo_file" "$smoke_guest_headers"
  exit "$status"
}
trap cleanup EXIT

docker compose up -d --build --wait

health_ok=false
for ((attempt = 1; attempt <= 30; attempt++)); do
  if health_response="$(curl -fsS "$smoke_base_url/api/health" 2>/dev/null)" && [[ "$health_response" == *'"status":"ok"'* ]]; then
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

homepage="$(curl -fsS "$smoke_base_url/")"
if [[ "$homepage" != *'<div id="app"'* ]]; then
  echo 'Homepage did not contain the frontend mount element <div id="app".' >&2
  exit 1
fi

echo 'Homepage mount element passed.'

guest_status="$(curl -sS -D "$smoke_guest_headers" -o /dev/null -w '%{http_code}' \
  "$smoke_base_url/e/$smoke_event_token")"
if [[ "$guest_status" != 303 ]]; then
  echo "Guest QR entry returned HTTP $guest_status; want 303." >&2
  exit 1
fi
guest_cookie="$(awk 'tolower($1) == "set-cookie:" && $2 ~ /^guest_session=/ { sub(/;.*/, "", $2); print $2; exit }' "$smoke_guest_headers")"
if [[ -z "$guest_cookie" ]]; then
  echo 'Guest QR entry did not set a guest session cookie.' >&2
  exit 1
fi
echo 'Guest QR entry passed.'

printf '%s' 'iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAIAAACQd1PeAAAADUlEQVR4nGP4z8AAAAMBAQDJ/pLvAAAAAElFTkSuQmCC' |
  base64 --decode >"$smoke_photo_file"

unauthenticated_upload_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H 'Content-Type: image/png' --data-binary "@$smoke_photo_file" \
  "$smoke_base_url/api/media/$smoke_photo_id")"
if [[ "$unauthenticated_upload_status" != 401 ]]; then
  echo "Unauthenticated photo upload returned HTTP $unauthenticated_upload_status; want 401." >&2
  exit 1
fi

# Forward the cookie manually because the local smoke test uses HTTP and the cookie remains Secure.
upload_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H "Cookie: $guest_cookie" -H 'Content-Type: image/png' \
  --data-binary "@$smoke_photo_file" "$smoke_base_url/api/media/$smoke_photo_id")"
if [[ "$upload_status" != 201 ]]; then
  echo "Authenticated photo upload returned HTTP $upload_status; want 201." >&2
  exit 1
fi
echo 'Authenticated photo upload passed.'

retry_status="$(curl -sS -o /dev/null -w '%{http_code}' \
  -X PUT -H "Cookie: $guest_cookie" -H 'Content-Type: image/png' \
  --data-binary "@$smoke_photo_file" "$smoke_base_url/api/media/$smoke_photo_id")"
if [[ "$retry_status" != 200 ]]; then
  echo "Idempotent photo retry returned HTTP $retry_status; want 200." >&2
  exit 1
fi
echo 'Idempotent photo retry passed.'
