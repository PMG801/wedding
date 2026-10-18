#!/usr/bin/env bash
# Opt-in host provisioning for the Oracle Always Free A1 server (Ubuntu arm64;
# Oracle Linux is tolerated). Run it once, with sudo, BEFORE ./bootstrap.sh:
#
#   sudo ./provision-host.sh --dry-run
#   sudo ./provision-host.sh
#   sudo ./provision-host.sh --data-device /dev/sdb
#   sudo ./provision-host.sh --data-device /dev/sdb --format-data-device  # first use only; erases /dev/sdb
#
# Each step inspects the current state before changing anything, so rerunning
# is safe. The script never formats a device without --format-data-device.
# The OCI security list is not touched here: open TCP 80 and 443 there yourself.
set -euo pipefail

# OS_RELEASE_FILE exists so the dry-run can be exercised on other distributions.
OS_RELEASE_FILE=${OS_RELEASE_FILE:-/etc/os-release}
SWAP_FILE=/swapfile
SWAP_SIZE=2G
SWAPPINESS=10
SYSCTL_FILE=/etc/sysctl.d/99-boda.conf
DATA_MOUNT=/srv/boda/data
DATA_DEVICE=
FORMAT_DATA_DEVICE=false
DRY_RUN=false
OS_ID=

log() { printf '\n==> %s\n' "$*"; }
note() { printf '    %s\n' "$*"; }
die() { printf 'provision-host: %s\n' "$*" >&2; exit 1; }

# run executes a state-changing command, or only prints it in --dry-run mode.
run() {
  if [[ $DRY_RUN == true ]]; then
    printf '    [dry-run] %s\n' "$*"
  else
    printf '    + %s\n' "$*"
    "$@"
  fi
}

# write_file replaces PATH with CONTENT only when the content differs.
write_file() {
  local path=$1 content=$2
  if [[ -f $path && "$(cat -- "$path")" == "$content" ]]; then
    note "$path already up to date"
    return 0
  fi
  if [[ $DRY_RUN == true ]]; then
    printf '    [dry-run] write %s:\n' "$path"
    printf '%s\n' "$content" | sed 's/^/        | /'
  else
    printf '    + write %s\n' "$path"
    printf '%s\n' "$content" >"$path"
  fi
}

# append_once appends LINE to FILE unless an identical line already exists.
append_once() {
  local file=$1 line=$2
  if grep -qxF -- "$line" "$file" 2>/dev/null; then
    note "$file already has: $line"
    return 0
  fi
  if [[ $DRY_RUN == true ]]; then
    printf '    [dry-run] append to %s: %s\n' "$file" "$line"
  else
    printf '    + append to %s: %s\n' "$file" "$line"
    printf '%s\n' "$line" >>"$file"
  fi
}

parse_args() {
  while (($# > 0)); do
    case $1 in
      --dry-run) DRY_RUN=true ;;
      --data-device)
        (($# >= 2)) || die "--data-device needs a device path"
        DATA_DEVICE=$2
        shift
        ;;
      --data-mount)
        (($# >= 2)) || die "--data-mount needs a path"
        DATA_MOUNT=$2
        shift
        ;;
      --format-data-device) FORMAT_DATA_DEVICE=true ;;
      -h | --help)
        sed -n '2,11p' "$0"
        exit 0
        ;;
      *) die "unknown argument: $1" ;;
    esac
    shift
  done

  if [[ $FORMAT_DATA_DEVICE == true && -z $DATA_DEVICE ]]; then
    die "--format-data-device requires --data-device"
  fi
  [[ $DATA_MOUNT == /* ]] || die "--data-mount must be an absolute path"
}

preflight() {
  if [[ $DRY_RUN != true && $EUID -ne 0 ]]; then
    die "run as root: sudo $0"
  fi
  [[ -r $OS_RELEASE_FILE ]] || die "$OS_RELEASE_FILE not found"
  # shellcheck source=/dev/null
  . "$OS_RELEASE_FILE"
  OS_ID=${ID:-unknown}
  case $OS_ID in
    ubuntu | ol) ;;
    *) die "unsupported OS '$OS_ID' (Ubuntu and Oracle Linux only)" ;;
  esac
  local arch
  arch=$(uname -m)
  if [[ $arch != aarch64 ]]; then
    printf 'warning: this host is %s; the target is aarch64 (Ampere A1)\n' "$arch" >&2
  fi
  # firewalld would reject the rules this script adds and own the same ports.
  if systemctl is-active --quiet firewalld 2>/dev/null; then
    die "firewalld is active. Open the ports with: firewall-cmd --permanent --add-service=http --add-service=https && firewall-cmd --reload, then rerun"
  fi
}

install_packages() {
  if [[ $OS_ID == ubuntu ]]; then
    export DEBIAN_FRONTEND=noninteractive
    run apt-get update
    # Answer the iptables-persistent prompt in advance so the install never blocks.
    run sh -c 'printf "%s\n" "iptables-persistent iptables-persistent/autosave_v4 boolean true" "iptables-persistent iptables-persistent/autosave_v6 boolean false" | debconf-set-selections'
    run apt-get install -y --no-install-recommends ca-certificates curl iptables-persistent unattended-upgrades
  else
    run dnf install -y curl chrony iptables-services dnf-automatic
  fi
}

ensure_docker() {
  if command -v docker >/dev/null 2>&1 && docker compose version >/dev/null 2>&1; then
    note "Docker Engine and the Compose plugin are installed"
  elif [[ $OS_ID == ubuntu ]]; then
    # Official convenience script. It is downloaded to a file first, so a failed
    # download stops the run instead of executing a partial script.
    run install -d -m 0700 /var/cache/boda-provision
    run curl -fsSL -o /var/cache/boda-provision/get-docker.sh https://get.docker.com
    run sh /var/cache/boda-provision/get-docker.sh
  elif [[ $DRY_RUN == true ]]; then
    note "Docker is missing: on Oracle Linux, install docker-ce and docker-compose-plugin from the Docker repository first"
  else
    die "Docker Engine with the Compose plugin is missing. On Oracle Linux, install docker-ce and docker-compose-plugin from the Docker repository, then rerun"
  fi
  run systemctl enable --now docker
}

persist_firewall() {
  if [[ $OS_ID == ubuntu ]]; then
    run netfilter-persistent save
  else
    run systemctl enable iptables
    run sh -c 'iptables-save > /etc/sysconfig/iptables'
  fi
}

# open_web_ports accepts new TCP 80 and 443 before the first REJECT in INPUT.
# Oracle images end INPUT with a REJECT, so an appended rule would never match.
open_web_ports() {
  local port rule_line
  for port in 80 443; do
    if iptables -C INPUT -p tcp --dport "$port" -m conntrack --ctstate NEW -j ACCEPT 2>/dev/null; then
      note "TCP $port already accepted in INPUT"
      continue
    fi
    rule_line=$(iptables -L INPUT -n --line-numbers 2>/dev/null |
      awk '$2 == "REJECT" { if (!n) n = $1 } END { print n }' || true)
    if [[ -n $rule_line ]]; then
      run iptables -I INPUT "$rule_line" -p tcp --dport "$port" -m conntrack --ctstate NEW -j ACCEPT
    else
      run iptables -A INPUT -p tcp --dport "$port" -m conntrack --ctstate NEW -j ACCEPT
    fi
  done
  persist_firewall
}

ensure_swap() {
  local active
  active=$(awk 'NR > 1' /proc/swaps | wc -l)
  if ((active > 0)); then
    note "swap is already active ($active device(s)); no swapfile added"
    return 0
  fi
  if [[ ! -e $SWAP_FILE ]]; then
    run fallocate -l "$SWAP_SIZE" "$SWAP_FILE"
    run chmod 0600 "$SWAP_FILE"
    run mkswap "$SWAP_FILE"
  fi
  run swapon "$SWAP_FILE"
  append_once /etc/fstab "$SWAP_FILE none swap sw 0 0"
}

ensure_swappiness() {
  write_file "$SYSCTL_FILE" "vm.swappiness = $SWAPPINESS"
  run sysctl -q -p "$SYSCTL_FILE"
}

ensure_data_volume() {
  if [[ -z $DATA_DEVICE ]]; then
    note "no --data-device given: the data root stays on the boot volume"
    return 0
  fi
  [[ -b $DATA_DEVICE ]] || die "$DATA_DEVICE is not a block device"

  local mount_point uuid fstype contents
  while IFS= read -r mount_point; do
    [[ -z $mount_point || $mount_point == "$DATA_MOUNT" ]] ||
      die "$DATA_DEVICE is mounted at $mount_point; refusing to touch it"
  done < <(lsblk -nr -o MOUNTPOINT "$DATA_DEVICE" 2>/dev/null)

  uuid=$(blkid -s UUID -o value "$DATA_DEVICE" 2>/dev/null || true)
  if [[ -z $uuid ]]; then
    [[ $FORMAT_DATA_DEVICE == true ]] ||
      die "$DATA_DEVICE has no filesystem. Rerun with --format-data-device to format it as ext4 (this erases the device)"
    run mkfs.ext4 -L boda-data "$DATA_DEVICE"
    if [[ $DRY_RUN == true ]]; then
      note "dry-run: the UUID exists only after formatting, so the fstab and mount steps are skipped in this preview"
      return 0
    fi
    uuid=$(blkid -s UUID -o value "$DATA_DEVICE")
  fi
  fstype=$(blkid -s TYPE -o value "$DATA_DEVICE" 2>/dev/null || true)
  [[ -n $fstype ]] || die "could not detect the filesystem type of $DATA_DEVICE"

  run mkdir -p "$DATA_MOUNT"
  if ! mountpoint -q "$DATA_MOUNT"; then
    # Mounting over a non-empty directory would hide its contents.
    contents=$(find "$DATA_MOUNT" -mindepth 1 -maxdepth 1 ! -name lost+found -print -quit 2>/dev/null || true)
    [[ -z $contents ]] ||
      die "$DATA_MOUNT is not empty; provision the volume before bootstrap.sh creates data/"
  fi
  # nofail: a missing volume must not block boot. _netdev: the block volume is network-attached.
  append_once /etc/fstab "UUID=$uuid $DATA_MOUNT $fstype defaults,nofail,_netdev 0 2"
  if mountpoint -q "$DATA_MOUNT"; then
    note "$DATA_MOUNT is already mounted"
  else
    run mount "$DATA_MOUNT"
  fi
}

ensure_auto_updates() {
  if [[ $OS_ID == ubuntu ]]; then
    write_file /etc/apt/apt.conf.d/20auto-upgrades \
      'APT::Periodic::Update-Package-Lists "1";'$'\n''APT::Periodic::Unattended-Upgrade "1";'
    run systemctl enable --now unattended-upgrades
  else
    run sed -i 's/^apply_updates = .*/apply_updates = yes/' /etc/dnf/automatic.conf
    run systemctl enable --now dnf-automatic-install.timer
  fi
}

ensure_time_sync() {
  if [[ $OS_ID == ubuntu ]]; then
    run timedatectl set-ntp true
  else
    run systemctl enable --now chronyd
  fi
  note "NTP synchronized: $(timedatectl show -p NTPSynchronized --value 2>/dev/null || echo unknown)"
}

main() {
  parse_args "$@"
  preflight
  if [[ $DRY_RUN == true ]]; then
    note "dry-run: nothing will be changed"
  fi

  log "packages"
  install_packages
  log "docker engine and compose plugin"
  ensure_docker
  log "firewall: TCP 80 and 443 before the REJECT rule, persisted"
  open_web_ports
  log "swap (only when the host has none)"
  ensure_swap
  log "vm.swappiness"
  ensure_swappiness
  log "data volume"
  ensure_data_volume
  log "automatic security updates"
  ensure_auto_updates
  log "time synchronization"
  ensure_time_sync

  log "done"
  cat <<EOF
Next steps (manual):
  - OCI console: add ingress TCP 80 and 443 from 0.0.0.0/0 to the subnet security list (or NSG).
  - Operator access to Docker is root-equivalent. Only if you accept that: sudo usermod -aG docker <user>, then log in again.
  - Then run ./bootstrap.sh from /srv/boda.
EOF
}

main "$@"
