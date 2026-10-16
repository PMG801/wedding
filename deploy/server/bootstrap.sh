#!/usr/bin/env bash
# One-time setup of /srv/boda: data directories and a generated .env.
# Idempotent: never overwrites an existing .env.
set -euo pipefail
cd "$(dirname "$0")"

sudo mkdir -p data/originals data/derived
sudo chown 65532:65532 data data/originals data/derived
sudo chmod 0700 data
sudo chmod 0755 data/originals data/derived

if [ -f .env ]; then
  echo ".env already exists, leaving it untouched"
  exit 0
fi

admin_password=$(openssl rand -base64 18 | tr -d '/+=\n')
admin_hash=$(docker run --rm --entrypoint caddy ghcr.io/pmg801/boda-web:rc \
  hash-password --plaintext "$admin_password")

umask 077
{
  echo "BODA_DOMAIN=:80"
  echo "BODA_DATA_DIR=/srv/evento"
  echo "BODA_EVENT_TOKEN=$(openssl rand -base64 32 | tr '+/' '-_' | tr -d '=\n')"
  echo "BODA_SESSION_KEY=$(openssl rand -hex 16)"
  echo "BODA_ADMIN_PASSWORD_HASH='$admin_hash'"
} > .env
printf '%s\n' "$admin_password" > .admin-password
echo "Created .env (600) and .admin-password (600). Neither is in Git."
