#!/bin/bash

set -euo pipefail

dist_dir=${1:-dist}
expected_targets=(
  darwin_amd64
  darwin_arm64
  linux_amd64
  linux_arm64
)

if [[ ! -f "$dist_dir/checksums.txt" ]]; then
  echo "missing $dist_dir/checksums.txt" >&2
  exit 1
fi

if command -v sha256sum >/dev/null 2>&1; then
  (cd "$dist_dir" && sha256sum --check checksums.txt)
else
  (cd "$dist_dir" && shasum -a 256 --check checksums.txt)
fi

shopt -s nullglob
for target in "${expected_targets[@]}"; do
  archives=("$dist_dir"/distill_*_"$target".tar.gz)
  if [[ ${#archives[@]} -ne 1 ]]; then
    echo "expected one $target archive, found ${#archives[@]}" >&2
    exit 1
  fi
  if ! tar -tzf "${archives[0]}" | grep -qx distill; then
    echo "${archives[0]} does not contain distill" >&2
    exit 1
  fi
done

goos=$(go env GOOS)
goarch=$(go env GOARCH)
native_archives=("$dist_dir"/distill_*_"${goos}_${goarch}".tar.gz)
if [[ ${#native_archives[@]} -ne 1 ]]; then
  echo "expected one native ${goos}_${goarch} archive, found ${#native_archives[@]}" >&2
  exit 1
fi

tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT
tar -xzf "${native_archives[0]}" -C "$tmp_dir"

"$tmp_dir/distill" handoff --help >/dev/null
"$tmp_dir/distill" handoff prepare --help >/dev/null
"$tmp_dir/distill" handoff verify --help >/dev/null

echo "validated checksums, macOS/Linux targets, and handoff help for ${goos}_${goarch}"
