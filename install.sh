#!/usr/bin/env bash
set -euo pipefail
umask 077
# Local bundle by default; optional downloads are restricted to this repository.
start=0; rollback=0; download=''; prefix=''
while (($#)); do
  case "$1" in
    --start) start=1; shift ;;
    --rollback) rollback=1; shift ;;
    --download) download="${2:?release tag required}"; shift 2 ;;
    --root) prefix="${2:?absolute installation root required}"; shift 2 ;;
    --help) echo 'Usage: bash install.sh [--download nightly|vVERSION] [--start] [--rollback] [--root /path]'; exit ;;
    *) echo "Unknown argument: $1" >&2; exit 1 ;;
  esac
done
[[ "$EUID" == 0 && "$(uname -s)" == Linux ]] || { echo 'Linux and root privileges required.' >&2; exit 1; }
[[ -z "$prefix" || "$prefix" == /* && "$prefix" != / && "$prefix" != *'/../'* && "$prefix" != */.. ]] || { echo 'Invalid installation root' >&2; exit 1; }
if [[ -n "$prefix" && "${NODESWEEP_INSTALL_TEST:-0}" != 1 ]]; then echo '--root is restricted to isolated installer tests' >&2; exit 1; fi
command -v systemctl >/dev/null
command -v flock >/dev/null
source_dir="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
work=$(mktemp -d)
stopped=0; installed=0
cleanup() {
  code=$?
  if [[ "$code" != 0 && "$stopped" == 1 && "$was_active" == 1 ]]; then
    if [[ "$installed" == 1 && "$had_binary" == 1 ]]; then
      install -m 0755 "$work/old-binary" "$binary.new" && mv -f -- "$binary.new" "$binary"
    fi
    systemctl restart nodesweep || true
  fi
  rm -rf -- "$work"
}
trap cleanup EXIT
trap 'exit 130' INT
trap 'exit 143' TERM
if [[ -n "$download" ]]; then
  [[ "$rollback" == 0 ]] || { echo 'Download and rollback cannot be combined' >&2; exit 1; }
  [[ "$download" == nightly || "$download" =~ ^v[0-9]+\.[0-9]+\.[0-9]+(-[A-Za-z0-9]+(\.[A-Za-z0-9]+)*)?$ ]] || { echo 'Invalid release tag' >&2; exit 1; }
  case "$(uname -m)" in x86_64) arch=amd64 ;; aarch64|arm64) arch=arm64 ;; *) echo 'Unsupported CPU architecture' >&2; exit 1 ;; esac
  asset="nodesweep-linux-$arch.tar.gz"
  url="https://github.com/While-Shark/NodeSweep/releases/download/$download"
  curl --fail --location --proto '=https' --proto-redir '=https' --connect-timeout 10 --max-time 180 --max-filesize 67108864 "$url/$asset" -o "$work/$asset"
  curl --fail --location --proto '=https' --proto-redir '=https' --connect-timeout 10 --max-time 30 --max-filesize 4096 "$url/SHA256SUMS" -o "$work/sums"
  hash=$(awk -v name="$asset" '$2==name {print $1}' "$work/sums")
  [[ "$hash" =~ ^[a-fA-F0-9]{64}$ ]] || { echo 'Missing or invalid checksum' >&2; exit 1; }
  printf '%s  %s\n' "$hash" "$asset" > "$work/SHA256SUMS"
  (cd "$work" && sha256sum -c SHA256SUMS)
  mkdir "$work/bundle"
  tar --no-same-owner --no-same-permissions -xzf "$work/$asset" -C "$work/bundle"
  source_dir="$work/bundle"
fi
binary="$prefix/usr/local/bin/nodesweep"
config_dir="$prefix/etc/nodesweep"
state_dir="$prefix/var/lib/nodesweep"
unit="$prefix/etc/systemd/system/nodesweep.service"
for directory in "$config_dir" "$state_dir" "$prefix/usr/local/bin" "$prefix/etc/systemd/system"; do
  [[ ! -L "$directory" ]] || { echo 'Installation directories must not be symlinks' >&2; exit 1; }
done
install -d -m 0700 "$config_dir" "$state_dir"
install -d "$prefix/usr/local/bin" "$prefix/etc/systemd/system"
exec 9>"$state_dir/.install.lock"
flock -n 9 || { echo 'Another install is running' >&2; exit 1; }
if [[ "$rollback" == 1 ]]; then
  source_binary="$state_dir/previous-binary"
else
  source_binary="$source_dir/nodesweep"
fi
[[ -f "$source_binary" && ! -L "$source_binary" ]] || { echo 'A local binary or previous version is required' >&2; exit 1; }
"$source_binary" -version
install -m 0755 "$source_binary" "$work/new-binary"
was_active=0
systemctl is-active --quiet nodesweep && was_active=1
had_binary=0
if [[ -e "$binary" ]]; then
  [[ -f "$binary" && ! -L "$binary" ]] || { echo 'Installed binary must be a regular file' >&2; exit 1; }
  had_binary=1
  install -m 0755 "$binary" "$work/old-binary"
fi
if [[ "$was_active" == 1 ]]; then systemctl stop nodesweep; stopped=1; fi
# Snapshot only stopped standard state. Custom data paths require an operator backup.
backup="$state_dir/backups/$(date -u +%Y%m%dT%H%M%SZ)-$$"
install -d -m 0700 "$backup"
[[ ! -e "$config_dir/config.json" ]] || cp -p -- "$config_dir/config.json" "$backup/config.json"
if [[ -d "$state_dir/data" ]]; then cp -a -- "$state_dir/data" "$backup/data"; fi
activate() {
  install -m 0755 "$work/new-binary" "$binary.new" || return 1
  mv -f -- "$binary.new" "$binary" || return 1
  installed=1
  if [[ ! -e "$config_dir/config.json" ]]; then
    (cd "$state_dir" && "$binary" -config "$config_dir/config.json" -init) || return 1
  fi
  if [[ ! -e "$unit" ]]; then
    install -m 0644 "$source_dir/deploy/nodesweep.service" "$unit" || return 1
  fi
  systemctl daemon-reload || return 1
  if [[ "$was_active" == 1 || "$start" == 1 ]]; then
    systemctl enable nodesweep || return 1
    systemctl restart nodesweep || return 1
    # Ensure the new process survives initial startup, including config/database validation.
    sleep 2 || return 1
    systemctl is-active --quiet nodesweep
  fi
}
if ! activate; then
  echo 'Activation failed; restoring the previous binary.' >&2
  if [[ "$had_binary" == 1 ]]; then
    install -m 0755 "$work/old-binary" "$binary.new"
    mv -f -- "$binary.new" "$binary"
    if [[ "$was_active" == 1 ]]; then systemctl restart nodesweep; fi
  else
    systemctl stop nodesweep || true
    systemctl disable nodesweep || true
    rm -f -- "$binary"
  fi
  stopped=0
  echo "Backup retained at $backup" >&2
  exit 1
fi
if [[ "$had_binary" == 1 ]]; then
  install -m 0755 "$work/old-binary" "$state_dir/previous-binary.new"
  mv -f -- "$state_dir/previous-binary.new" "$state_dir/previous-binary"
fi
printf '%s\n' "Installed: $binary" "Backup: $backup" 'Configuration preserved. Binary rollback: bash install.sh --rollback' 'Backups are retained; remove obsolete snapshots after verification.'
if [[ "$was_active" == 0 && "$start" == 0 ]]; then
  echo 'Review /etc/nodesweep/config.json, then run: systemctl enable --now nodesweep'
fi
