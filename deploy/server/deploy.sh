#!/usr/bin/env bash
# Pull the latest images and (re)start the stack. Run from /srv/boda.
set -euo pipefail
cd "$(dirname "$0")"
[ -f .env ] || { echo "missing .env (run ./bootstrap.sh first)" >&2; exit 1; }
# provision-host.sh puts the data root on a block volume listed in fstab with
# nofail. If that volume is missing at boot, refuse to write to the boot disk.
data_dir="$(pwd -P)/data"
if awk -v dir="$data_dir" '$2 == dir { found = 1 } END { exit !found }' /etc/fstab 2>/dev/null &&
  ! mountpoint -q "$data_dir"; then
  echo "$data_dir is in /etc/fstab but not mounted; refusing to deploy onto the boot volume" >&2
  exit 1
fi
docker compose pull
docker compose up -d --remove-orphans
docker compose ps
