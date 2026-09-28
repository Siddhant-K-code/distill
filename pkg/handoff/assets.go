package handoff

import _ "embed"

//go:embed proposal.schema.json
var proposalSchemaBytes []byte

var instructionsBytes = []byte(`# Distill Handoff v0 alpha: external-agent instructions

This bundle is an immutable, normalized snapshot. Read handoff.request.json,
inputs/conversation.md, and inputs/docs/**. Distill does not call you and will
not apply your output.

Write one canonical JSON proposal matching proposal.schema.json:

1. Bind the exact request_id and SHA-256 of handoff.request.json.
2. Use outcome candidate_decisions only for committed decisions supported by an
   exact conversation quote, UTF-8 byte range, and line range. Otherwise use
   no_decision with an empty candidates array.
3. For each candidate, bind the frozen target path and both target digests.
4. Propose exactly one existing-Markdown-file unified diff with headers
   --- a/<target> and +++ b/<target>. Do not create, delete, rename, copy, change
   modes, or emit binary patches.
5. SHA-256 the exact unified_diff UTF-8 bytes and compute the stable candidate ID
   as documented in docs/handoff-v0.md.
6. Set every proposal and candidate route to require_review.
7. Use strict two-space canonical JSON with the schema field order and one final
   LF. Do not add fields.

Probabilistic extraction and patch drafting are outside Distill's deterministic
guarantee. A human must review every accepted candidate and patch.
`)
