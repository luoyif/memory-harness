#!/usr/bin/env bash
set -euo pipefail

addr="${MEMORYOS_ADDR:-127.0.0.1:19777}"
if [[ -r /etc/memory-harness/memory-harness.env ]]; then
  configured_addr="$(sed -n 's/^MEMORYOS_ADDR=//p' /etc/memory-harness/memory-harness.env | tail -n 1)"
  if [[ -n "$configured_addr" ]]; then
    addr="$configured_addr"
  fi
fi

check_url="http://$addr/health"
if command -v curl >/dev/null 2>&1; then
  for attempt in {1..40}; do
    if response="$(curl --fail --silent "$check_url" 2>/dev/null)"; then
      printf '%s\n' "$response"
      exit 0
    fi
    sleep 0.25
  done
  curl --fail --silent --show-error "$check_url"
  printf '\n'
elif command -v wget >/dev/null 2>&1; then
  for attempt in {1..40}; do
    if response="$(wget -qO- "$check_url" 2>/dev/null)"; then
      printf '%s\n' "$response"
      exit 0
    fi
    sleep 0.25
  done
  wget -O- "$check_url"
  printf '\n'
else
  echo "curl or wget is required for this health check" >&2
  exit 1
fi
