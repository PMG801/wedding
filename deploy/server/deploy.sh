#!/usr/bin/env bash
# Pull the latest images and (re)start the stack. Run from /srv/boda.
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] || { echo "missing .env (run ./bootstrap.sh first)" >&2; exit 1; }
docker compose pull
docker compose up -d --remove-orphans
docker compose ps
