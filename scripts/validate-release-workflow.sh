#!/bin/bash

set -euo pipefail

workflow=.github/workflows/release.yml

assert_line() {
  local text=$1
  if ! grep -Fqx "$text" "$workflow"; then
    echo "release workflow must contain: $text" >&2
    exit 1
  fi
}

assert_step_gate() {
  local step=$1
  local gate=$2
  local block
  block=$(
    awk -v header="      - name: $step" '
      $0 == header { found = 1; next }
      found && /^      - name:/ { exit }
      found { print }
    ' "$workflow"
  )
  if [[ -z "$block" ]] || ! grep -Fqx "        $gate" <<<"$block"; then
    echo "step '$step' must use release gate: $gate" >&2
    exit 1
  fi
}

assert_line '      RELEASE_VERSION: ${{ github.event_name == '\''push'\'' && github.ref_name || inputs.version }}'
assert_line '        run: ./scripts/validate-release-version.sh "$RELEASE_VERSION"'
assert_line '          git tag -a "$RELEASE_VERSION" -m "Release $RELEASE_VERSION"'
assert_line '          git push origin "$RELEASE_VERSION"'

assert_step_gate "Create and push tag" \
  "if: github.event_name == 'workflow_dispatch' && !inputs.dry_run"
assert_step_gate "Run GoReleaser" \
  "if: github.event_name == 'push' && startsWith(github.ref, 'refs/tags/v')"
assert_step_gate "Run GoReleaser (dry run)" \
  "if: github.event_name == 'workflow_dispatch' && inputs.dry_run"
assert_step_gate "Validate release archives" \
  "if: github.event_name == 'workflow_dispatch' && inputs.dry_run"

if grep -Fq 'if: ${{ !inputs.dry_run }}' "$workflow"; then
  echo "non-snapshot publishing must not run from workflow_dispatch" >&2
  exit 1
fi

if grep -E 'git (tag|push).*\$\{\{' "$workflow" >/dev/null; then
  echo "release input must not be interpolated directly into git commands" >&2
  exit 1
fi

validate_line=$(grep -nF '      - name: Validate release version' "$workflow" | cut -d: -f1)
tag_line=$(grep -nF '      - name: Create and push tag' "$workflow" | cut -d: -f1)
publish_line=$(grep -nF '      - name: Run GoReleaser' "$workflow" | head -1 | cut -d: -f1)
if [[ "$validate_line" -ge "$tag_line" || "$validate_line" -ge "$publish_line" ]]; then
  echo "release version validation must run before tag and publish steps" >&2
  exit 1
fi

echo "validated release workflow event gates"
