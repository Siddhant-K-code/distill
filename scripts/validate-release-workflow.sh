#!/bin/bash

set -euo pipefail

workflow=${1:-.github/workflows/release.yml}
publish_gate="if: github.event_name == 'push' || (github.event_name == 'workflow_dispatch' && !inputs.dry_run)"

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
assert_line '  group: release-${{ github.event_name == '\''push'\'' && github.ref_name || inputs.version }}'
assert_line '  cancel-in-progress: false'
assert_line '        run: ./scripts/validate-release-version.sh "$RELEASE_VERSION"'
assert_line '          GH_TOKEN: ${{ github.token }}'
assert_line '          if gh api --silent "repos/${GITHUB_REPOSITORY}/releases/tags/${RELEASE_VERSION}" 2>"$release_check_error"; then'
assert_line '          if git show-ref --verify --quiet "refs/tags/$RELEASE_VERSION"; then'
assert_line '            tag_type=$(git cat-file -t "refs/tags/$RELEASE_VERSION")'
assert_line '            if [[ "$tag_type" != "tag" ]]; then'
assert_line '            tag_commit=$(git rev-parse "${RELEASE_VERSION}^{commit}")'
assert_line '            git tag -a "$RELEASE_VERSION" "$dispatch_commit" -m "Release $RELEASE_VERSION"'
assert_line '            git push origin "refs/tags/$RELEASE_VERSION"'
assert_line '          git checkout --detach "refs/tags/$RELEASE_VERSION"'
assert_line '          args: release --clean'
assert_line '          args: release --clean --snapshot --skip=publish'

assert_step_gate "Refuse duplicate GitHub release" \
  "$publish_gate"
assert_step_gate "Prepare dispatch release tag" \
  "if: github.event_name == 'workflow_dispatch' && !inputs.dry_run"
assert_step_gate "Run GoReleaser" \
  "$publish_gate"
assert_step_gate "Run GoReleaser (dry run)" \
  "if: github.event_name == 'workflow_dispatch' && inputs.dry_run"
assert_step_gate "Validate release archives" \
  "if: github.event_name == 'workflow_dispatch' && inputs.dry_run"

if ! grep -Fq "      - 'v*'" "$workflow"; then
  echo "direct v-prefixed tag pushes must remain supported" >&2
  exit 1
fi

if grep -Eq 'git (tag -d|tag .* -f|push .*--force|push .*--delete|push .*:refs/tags/|update-ref -d)' "$workflow"; then
  echo "release workflow must not delete or force-update tags" >&2
  exit 1
fi

if grep -E 'git (tag|push).*\$\{\{' "$workflow" >/dev/null; then
  echo "release input must not be interpolated directly into git commands" >&2
  exit 1
fi

validate_line=$(grep -nF '      - name: Validate release version' "$workflow" | cut -d: -f1)
duplicate_line=$(grep -nF '      - name: Refuse duplicate GitHub release' "$workflow" | cut -d: -f1)
tag_line=$(grep -nF '      - name: Prepare dispatch release tag' "$workflow" | cut -d: -f1)
publish_line=$(grep -nF '      - name: Run GoReleaser' "$workflow" | head -1 | cut -d: -f1)
if [[ "$validate_line" -ge "$duplicate_line" || "$duplicate_line" -ge "$tag_line" || "$tag_line" -ge "$publish_line" ]]; then
  echo "release validation, duplicate refusal, tag preparation, and publication are out of order" >&2
  exit 1
fi

echo "validated release workflow event gates"
