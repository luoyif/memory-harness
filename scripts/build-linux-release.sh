#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
version="${1:-2.2.0}"
output_arg="${2:-build/release/linux}"

if [[ "$output_arg" = /* ]]; then
  output_dir="$output_arg"
else
  output_dir="$repo_root/$output_arg"
fi

case "$version" in
  *[!0-9A-Za-z._-]*|'')
    echo "invalid version: $version" >&2
    exit 2
    ;;
esac

mkdir -p "$output_dir"
work_dir="$(mktemp -d)"
cleanup() {
  rm -rf "$work_dir"
}
trap cleanup EXIT

build_bundle() {
  local label="$1"
  local goarch="$2"
  local package_dir="$work_dir/Memory-Harness-$version-linux-$label"
  local archive="$output_dir/Memory-Harness-$version-linux-$label.tar.gz"

  mkdir -p "$package_dir"
  (
    cd "$repo_root"
    CGO_ENABLED=0 GOOS=linux GOARCH="$goarch" go build \
      -trimpath \
      -ldflags="-s -w -X github.com/luoyif/memory-harness/internal/buildinfo.Version=$version-memory-harness" \
      -o "$package_dir/memoryosd" \
      ./cmd/memoryosd
  )

  cp "$repo_root/packaging/linux/install.sh" "$package_dir/install.sh"
  cp "$repo_root/packaging/linux/uninstall.sh" "$package_dir/uninstall.sh"
  cp "$repo_root/packaging/linux/healthcheck.sh" "$package_dir/healthcheck.sh"
  cp "$repo_root/packaging/linux/memory-harness.service" "$package_dir/memory-harness.service"
  cp "$repo_root/packaging/linux/README.txt" "$package_dir/README.txt"
  cp "$repo_root/LICENSE" "$package_dir/LICENSE"
  chmod 0755 "$package_dir/memoryosd" "$package_dir/install.sh" "$package_dir/uninstall.sh" "$package_dir/healthcheck.sh"
  chmod 0644 "$package_dir/memory-harness.service" "$package_dir/README.txt" "$package_dir/LICENSE"

  tar -C "$work_dir" -czf "$archive" "$(basename "$package_dir")"
  echo "created $archive"
}

build_bundle x64 amd64
build_bundle arm64 arm64

(
  cd "$output_dir"
  if command -v sha256sum >/dev/null 2>&1; then
    sha256sum Memory-Harness-"$version"-linux-*.tar.gz > SHA256SUMS-linux.txt
  else
    shasum -a 256 Memory-Harness-"$version"-linux-*.tar.gz > SHA256SUMS-linux.txt
  fi
)

echo "checksums: $output_dir/SHA256SUMS-linux.txt"
