#!/usr/bin/env bash
set -euo pipefail
# Installs the binary bundled with a release; never downloads or executes remote code.
if [[ "${EUID}" -ne 0 ]]; then echo "Run with sudo or as root." >&2; exit 1; fi
if [[ "$(uname -s)" != Linux ]]; then echo "Linux only." >&2; exit 1; fi
command -v systemctl >/dev/null || { echo "systemd is required." >&2; exit 1; }
SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
[[ -f "$SCRIPT_DIR/nodesweep" ]] || { echo "Place install.sh beside the nodesweep binary." >&2; exit 1; }
if systemctl is-active --quiet nodesweep; then
    echo "Stop NodeSweep and back up its data before upgrading: systemctl stop nodesweep" >&2
    exit 1
fi
install -d -m 0700 /etc/nodesweep /var/lib/nodesweep
install -m 0755 "$SCRIPT_DIR/nodesweep" /usr/local/bin/nodesweep
if [[ ! -e /etc/nodesweep/config.json ]]; then
    /usr/local/bin/nodesweep -config /etc/nodesweep/config.json -init
fi
install -m 0644 "$SCRIPT_DIR/deploy/nodesweep.service" /etc/systemd/system/nodesweep.service
systemctl daemon-reload
printf '%s\n' 'Installed. Review /etc/nodesweep/config.json, then run:' '  systemctl enable --now nodesweep'
