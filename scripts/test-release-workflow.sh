#!/bin/bash

set -euo pipefail

workflow=.github/workflows/release.yml
goreleaser_config=.goreleaser.yml
tmp_dir=$(mktemp -d)
trap 'rm -rf "$tmp_dir"' EXIT

assert_auto_prerelease() {
  local config=$1
  awk '
    $0 == "release:" {
      in_release = 1
      next
    }
    in_release && /^[^[:space:]#]/ {
      in_release = 0
    }
    in_release && /^  prerelease:[[:space:]]+auto([[:space:]]*(#.*)?)?$/ {
      matches++
    }
    END {
      exit matches == 1 ? 0 : 1
    }
  ' "$config"
}

if ! assert_auto_prerelease "$goreleaser_config"; then
  echo "GoReleaser must automatically mark semantic prerelease tags" >&2
  exit 1
fi

sed 's/^  prerelease: auto$/  prerelease: false/' \
  "$goreleaser_config" >"$tmp_dir/stable-only-goreleaser.yml"
if assert_auto_prerelease "$tmp_dir/stable-only-goreleaser.yml"; then
  echo "accepted disabled automatic prerelease marking" >&2
  exit 1
fi

accepted=(
  v0.9.1
  v0.10.0-alpha.1
  v1.2.3-rc.0
)
rejected=(
  0.10.0-alpha.1
  v01.2.3
  v1.02.3
  v1.2.03
  v1.2
  v1.2.3-
  v1.2.3-alpha..1
  v1.2.3+build
  "v1.2.3 alpha"
  "v1.2.3; echo unsafe"
  'v1.2.3$(echo unsafe)'
)

for version in "${accepted[@]}"; do
  ./scripts/validate-release-version.sh "$version" >/dev/null
done

for version in "${rejected[@]}"; do
  if ./scripts/validate-release-version.sh "$version" >/dev/null 2>&1; then
    echo "accepted invalid release version: $version" >&2
    exit 1
  fi
done

./scripts/validate-release-workflow.sh

awk '
  $0 == "      - name: Run GoReleaser" { publishing = 1 }
  publishing && /^        if:/ {
    print "        if: github.event_name == '\''push'\'' && startsWith(github.ref, '\''refs/tags/v'\'')"
    publishing = 0
    next
  }
  { print }
' "$workflow" >"$tmp_dir/tag-push-only.yml"

if ./scripts/validate-release-workflow.sh "$tmp_dir/tag-push-only.yml" >/dev/null 2>&1; then
  echo "accepted tag-push-only publication regression" >&2
  exit 1
fi
