#!/usr/bin/env bash
set -euo pipefail

script_dir="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
service_name="memory-harness.service"
service_user="memory-harness"
service_group="memory-harness"
data_root="/var/lib/memory-harness"
config_dir="/etc/memory-harness"
config_file="$config_dir/memory-harness.env"
binary_path="/usr/local/bin/memoryosd"
unit_path="/etc/systemd/system/$service_name"
start_service=true

if [[ "${1:-}" == "--no-start" ]]; then
  start_service=false
elif [[ $# -gt 0 ]]; then
  echo "usage: sudo ./install.sh [--no-start]" >&2
  exit 2
fi

if [[ "${EUID:-$(id -u)}" -ne 0 ]]; then
  echo "please run as root: sudo ./install.sh" >&2
  exit 1
fi
if ! command -v systemctl >/dev/null 2>&1; then
  echo "systemd is required; systemctl was not found" >&2
  exit 1
fi
if [[ ! -x "$script_dir/memoryosd" ]]; then
  echo "memoryosd is missing from this package" >&2
  exit 1
fi

if ! getent group "$service_group" >/dev/null 2>&1; then
  groupadd --system "$service_group"
fi
if ! id -u "$service_user" >/dev/null 2>&1; then
  useradd --system --gid "$service_group" --home-dir "$data_root" --shell /usr/sbin/nologin "$service_user"
fi

install -d -m 0750 -o "$service_user" -g "$service_group" "$data_root" "$data_root/data"
install -d -m 0750 -o root -g "$service_group" "$config_dir"
if [[ ! -f "$config_file" ]]; then
  install -m 0640 -o root -g "$service_group" /dev/null "$config_file"
  printf '%s\n' \
    'MEMORYOS_HOME=/var/lib/memory-harness/data' \
    'MEMORYOS_ADDR=127.0.0.1:19777' >"$config_file"
fi

if systemctl is-active --quiet "$service_name"; then
  systemctl stop "$service_name"
fi
install -m 0755 -o root -g root "$script_dir/memoryosd" "$binary_path"
install -m 0644 -o root -g root "$script_dir/memory-harness.service" "$unit_path"
systemctl daemon-reload

if [[ "$start_service" == true ]]; then
  systemctl enable --now "$service_name"
  echo "Memory Harness is running on 127.0.0.1:19777"
  echo "health: $script_dir/healthcheck.sh"
else
  systemctl enable "$service_name"
  echo "Memory Harness installed but not started. Restore data if needed, then run:"
  echo "  sudo systemctl start $service_name"
fi
echo "data is stored in $data_root/data and is preserved by upgrades or uninstall"
