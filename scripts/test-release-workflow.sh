#!/bin/bash

set -euo pipefail

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
