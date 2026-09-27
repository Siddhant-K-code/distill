# Distill Handoff v0 alpha

Status: experimental review-only contract for Markdown in local repositories.
This is not general availability and does not indicate practitioner adoption.

Handoff addresses a narrow failure mode: a consequential decision is made in a
long conversation, but the reference documentation is not updated. It freezes
the conversation and docs into a portable request for any external coding
agent, then deterministically verifies and packages that agent's proposal for a
human reviewer.

## Guarantee and non-guarantees

For the same normalized request bundle and canonical proposal on a supported
macOS/Linux runtime, Handoff produces byte-identical review artifacts,
receipts, patch files, and checksums. It validates exact identities, evidence
bytes and ranges, safe target paths, source hashes, a bounded unified-diff
syntax, clean in-memory application, non-overlap, and review-only routing.

Decision extraction and patch drafting are probabilistic external-agent work.
They are outside this guarantee. Distill does not call a model or provider,
access the network, infer a decision, draft a patch, modify the docs tree, apply
a patch, approve a proposal, commit, push, or merge. Every accepted proposal
and candidate route is exactly `require_review`.

V0 alpha supports one exported `.md` conversation and a non-empty local tree
containing only `.md` files. It is not an agent framework, repository writer,
general patch engine, adoption claim, or release-readiness signal.

## Commands and lifecycle

```text
distill handoff prepare --conversation <file> --docs <dir> --out <fresh-dir>
distill handoff verify --request <bundle/handoff.request.json> \
  --proposal <proposal.json> --out <fresh-dir>
```

`prepare` validates every input and path, normalizes text with
`utf8-nfc-lf-v1`, records original and normalized byte identities, copies only
normalized bytes, and atomically publishes:

```text
handoff.request.json
agent-instructions.md
proposal.schema.json
inputs/conversation.md
inputs/docs/<relative Markdown paths>
SHA256SUMS
```

The request contains no absolute paths, cwd, timestamp, hostname, username,
inode, or environment data. Its `request_id` is the lowercase SHA-256 of:

```text
distill-handoff/request/v0alpha1\n
```

followed by canonical request JSON with `request_id` set to the empty string.
Documents are sorted by bytewise portable path. `SHA256SUMS` sorts every other
file by path and records lowercase SHA-256, byte length, and path.

The normalized files inside this request bundle are the authoritative snapshot.
`verify` never revisits the original working tree. If the live docs change,
prepare a new request before asking for review. Any change to a bundled input,
request field, schema, instruction, checksum, or extra file makes verification
fail.

An external agent writes one canonical JSON proposal against the included
draft-2020-12 schema. A proposal is either:

- `candidate_decisions` with one or more candidates sorted by candidate ID; or
- `no_decision` with no candidates and a non-empty reason.

Each candidate binds:

- decision text and one bounded type;
- the exact conversation quote, normalized source digest, half-open UTF-8 byte
  range, and 1-based inclusive line range;
- one docs-relative target path and its original and normalized frozen hashes;
- one embedded normalized unified diff and SHA-256;
- `route: "require_review"` and a non-empty reason.

To compute a candidate ID, encode the candidate in the schema's Go-struct field
order with `id` set to the empty string, two-space indentation, HTML escaping
disabled, and one final LF. Hash:

```text
distill-handoff/candidate/v0alpha1\n
```

plus those canonical bytes, then prefix the lowercase digest with `decision_`.

The accepted patch subset modifies exactly one existing Markdown target.
Headers must be exactly `--- a/<target>` and `+++ b/<target>`. Hunks must be
ordered, non-overlapping, count-correct, LF-terminated, and match exact target
context/removal lines. File creation/deletion, traversal, rename/copy, mode
changes, binary/submodule patches, Git metadata, and no-newline markers are
unsupported and fail closed.

`verify` applies nothing to disk. It applies each patch only in memory against
the frozen normalized target, rejects duplicate or overlapping candidates, and
atomically publishes:

```text
review.md
handoff.receipt.json
patches/<candidate-id>.patch
SHA256SUMS
```

No-decision output omits `patches/`. The receipt binds the request and proposal,
the review, each candidate's verified facts and patched-result hash, and every
published patch identity. `SHA256SUMS` binds the receipt, review, and patch
files. Reviewers must still judge whether the quote represents a real committed
decision and whether the proposed content is correct.

## Five-to-ten-minute offline trial

This flow uses only repository fixtures and local commands:

```bash
make build
work="$(cd "$(mktemp -d)" && pwd -P)"

./distill handoff prepare \
  --conversation testdata/handoff-v0/retry-policy/conversation.md \
  --docs testdata/handoff-v0/retry-policy/docs \
  --out "$work/request"

cp testdata/handoff-v0/retry-policy/proposal.json "$work/proposal.json"

./distill handoff verify \
  --request "$work/request/handoff.request.json" \
  --proposal "$work/proposal.json" \
  --out "$work/review"

cat "$work/review/review.md"
cat "$work/review/handoff.receipt.json"
cat "$work/review/SHA256SUMS"
```

The checked-in proposal stands in for external-agent output; Distill does not
invoke that agent. Repeat the same flow with `api-deprecation` or
`brainstorming`. The latter produces a valid no-decision package. Run the
deterministic three-fixture demo with:

```bash
make handoff-v0-demo
```

Negative proposals are under
`testdata/handoff-v0/negative/{unsupported-evidence,stale-target}`.

## Failure, security, and privacy boundary

All validation failures are explicit and nonzero. Handoff rejects path
traversal, absolute/backslash paths, Unicode-canonical collisions, unsupported
files, invalid UTF-8, NUL bytes, special files, every symlink (including
in-tree links), unsafe ownership/mode/ACL conditions, stale identities,
fabricated quotes, malformed patches, direct-allow routes, existing outputs,
and unexpected bundle files.

Outputs are written to a sibling staging directory, synchronized, reverified,
and published by native atomic no-replace rename. Failure before publication
leaves no success-shaped destination. A rare post-publication parent-directory
sync failure reports that publication occurred but durability is unconfirmed;
inspect the named output rather than retrying blindly.

The request deliberately contains the full normalized conversation and docs
snapshot. Treat it as potentially sensitive source material: store, transmit,
and retain it under the same controls as the originals. Handoff performs no
redaction, secret scanning, encryption, upload, telemetry, or network access.
Processes running as the same OS account remain inside the local trust
boundary.
