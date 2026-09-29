#!/bin/bash

set -euo pipefail

if [[ $# -ne 1 ]]; then
  echo "usage: $0 <v-prefixed-version>" >&2
  exit 1
fi

version=$1
number='(0|[1-9][0-9]*)'
identifier='(0|[1-9][0-9]*|[0-9A-Za-z-]*[A-Za-z-][0-9A-Za-z-]*)'
pattern="^v${number}\.${number}\.${number}(-${identifier}(\.${identifier})*)?$"

if [[ ! "$version" =~ $pattern ]]; then
  echo "invalid release version: $version" >&2
  exit 1
fi

echo "validated release version: $version"
