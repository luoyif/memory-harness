#!/usr/bin/env bash
set -euo pipefail

addr="${MEMORYOS_ADDR:-127.0.0.1:19777}"
if [[ -r /etc/memory-harness/memory-harness.env ]]; then
  configured_addr="$(sed -n 's/^MEMORYOS_ADDR=//p' /etc/memory-harness/memory-harness.env | tail -n 1)"
  if [[ -n "$configured_addr" ]]; then
    addr="$configured_addr"
  fi
fi

if command -v curl >/dev/null 2>&1; then
  curl --fail --silent --show-error "http://$addr/health"
  printf '\n'
elif command -v wget >/dev/null 2>&1; then
  wget -qO- "http://$addr/health"
  printf '\n'
else
  echo "curl or wget is required for this health check" >&2
  exit 1
fi
