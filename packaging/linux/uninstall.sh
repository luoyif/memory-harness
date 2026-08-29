#!/usr/bin/env bash
set -euo pipefail

service_name="memory-harness.service"

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "please run as root: sudo ./uninstall.sh" >&2
  exit 1
fi

if command -v systemctl >/dev/null 2>&1; then
  systemctl disable --now "$service_name" 2>/dev/null || true
fi
rm -f "/etc/systemd/system/$service_name" "/usr/local/bin/memoryosd"
if command -v systemctl >/dev/null 2>&1; then
  systemctl daemon-reload
fi

echo "Memory Harness service and binary were removed."
echo "Data and configuration were preserved:"
echo "  /var/lib/memory-harness"
echo "  /etc/memory-harness"
