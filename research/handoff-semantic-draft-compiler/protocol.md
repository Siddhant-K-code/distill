# Frozen Handoff semantic-draft compiler protocol

Frozen before any model invocation on 2026-09-30.

## Question, hypotheses, and scope

This preregistered research experiment tests whether a model can supply only a
small semantic draft while trusted deterministic code constructs the complete
Handoff proposal.

- Primary hypothesis: at least 2 of 3 positive-control drafts compile into the
  expected verifier-valid retry-policy candidate.
- Preservation hypothesis: all 3 negative-control drafts abstain and compile
  into verifier-valid `no_decision` proposals.
- Trust hypothesis: the intervention yields zero fabricated candidates and
  zero request-copy, canonical-request, source-repository, or trust-boundary
  mutations.
- Determinism hypothesis: replaying every retained valid draft yields
  byte-identical proposal bytes, proposal SHA-256, candidate IDs, and patch
  SHA-256 values.
- Cost and review-burden hypothesis: the intervention causes no material
  regression under the quantitative rule below.

This is a bounded diagnostic, not a product change. Passing authorizes only a
later experimental product-design proposal. Failure is published as observed.
The study does not change the Handoff schema, verifier, CLI, Pilot Kit,
fixtures, or product documentation.

Repository baseline:
`6aaefbb53acc5cb177d617292f96cd3b87f693ba`, which is the project default
branch state containing merged PR #120.

Historical baseline:
`research/handoff-capability-control` observed 0 of 3 positive expected
candidates, 3 of 3 negative correct abstentions, zero fabricated candidates,
and zero mutations. Its frozen classification was
`construction_bottleneck_observed`.

## Frozen fixtures and identities

The study reuses these exact immutable fixtures without editing them:

- Positive: `testdata/handoff-v0/retry-policy`.
- Negative: `testdata/handoff-v0/brainstorming`.

Fresh Handoff preparation must reproduce these retained identities before any
model invocation:

| Condition | Request ID | Request SHA-256 | Conversation SHA-256 | Document SHA-256 |
|---|---|---|---|---|
| Positive | `94730d11355f823740c1a4d9a7ff73c8841bc6c0038e8f6ac1c0d57829a75487` | `cd32864f9e42b44e0341bb9199dabb28a032103b8e2d9e9b4f4c19d61d3aea38` | `94a8d1951427b388d37919fecee4d68ae2f0d4398199b29214ce5230ef74077a` | `21f128d20f43ebeaa8a37a8519363103ea8cf4d57b126c7643879bfc07fbea70` |
| Negative | `eb613088a5e48b03b2f1cac9c47a6a6338f02dc859e3f3e6afd0b686cb36df47` | `45d333595c146fcd8ade578f6015258dda8c57275b75352d63ca08f8c89fb4d2` | `a706e599c3a4b29b8c7e289e21ed2b9fd8376e4e6c7ca69a7dd25ec308d05326` | `12f10d33f8c077c0611e58bcdee398c9d6e93641fbf91e62aa6a31a5cc64a4c7` |

The canonical request and its digest remain outside each model workspace. A
byte-for-byte copy of the corresponding request bundle is the only study input
placed inside that workspace. No derived fixture is needed.

## Frozen semantic-draft contract

The exact agent prompt is [`prompt.txt`](prompt.txt), SHA-256
`f348b76c98ed70a09835e17c14d9b281da113dc45ab142a1a382829899f9838d`.
The normative JSON Schema is
[`semantic-draft.schema.json`](semantic-draft.schema.json), SHA-256
`7f05117827b50c7ddec7ac33fbab9b58194aa272a698588e870f93fec34ce5a6`.

The model writes exactly one file at `output/semantic-draft.json`. Its top-level
fields are `schema_version`, `outcome`, `reason`, and `candidate`.

- `outcome` is exactly `abstain` or `candidate_intent`.
- `abstain` requires `candidate: null`.
- `candidate_intent` requires one candidate with decision text and type, an
  exact evidence quote and occurrence, a safe target path, an exact target
  anchor quote and occurrence, an operation, replacement text, and rationale.
- The only supported operation is exact full-line `replace`. This is the
  smallest operation needed by the positive fixture. Insert and delete are not
  supported and are rejected rather than guessed.
- Evidence and target quotes must be non-empty normalized UTF-8, match exactly
  once, and have occurrence `1`. The target anchor must be one complete line
  excluding its newline. Replacement text must be one non-empty line without
  CR or LF.
- Paths must be canonical relative POSIX Markdown paths already present in the
  frozen request.
- Unknown fields, extra JSON values, invalid decision types, unsupported
  operations, missing quotes, non-unique quotes, target mismatch, unsafe paths,
  and malformed schema fail closed.

The raw draft need not reproduce a byte-level JSON presentation. Draft
canonicalization is deterministic typed decoding followed by the repository's
`go-struct-json-indent-v1` encoding: UTF-8, NFC text values, LF line endings,
fixed struct field order, two-space indentation, no HTML escaping, and one
terminal LF. No semantic value is defaulted, repaired, trimmed, inferred, or
silently changed. Raw draft bytes are always retained separately.

The model is explicitly not asked to calculate request hashes, input hashes,
byte or line ranges, unified diffs, patch hashes, candidate IDs, or verification
results.

## Frozen deterministic compiler behavior

The compiler is research-only tooling under this directory and is not wired to
the public `distill` command.

Given a canonical request path, an independently retained expected request
SHA-256, and a raw semantic draft, the compiler:

1. Validates the expected digest syntax, request bytes, request schema identity,
   and exact request SHA-256.
2. Validates the raw draft against the frozen schema and strict typed decoder,
   then derives canonical semantic-draft bytes without modifying the raw file.
3. For abstention, constructs one canonical Handoff `no_decision` proposal with
   an empty candidate array and the draft reason.
4. For candidate intent, matches the exact evidence quote once in the frozen
   conversation, derives byte and line ranges, resolves the exact requested
   document identity, and matches the exact complete-line anchor once.
5. Replaces only that line, emits a deterministic unified diff with the complete
   small fixture as one hunk, calculates the patch SHA-256, copies trusted
   request and target identities, calculates the candidate ID through the
   production Handoff helper, sorts candidates, and emits canonical proposal
   bytes.
6. Writes no source file and never applies a patch.

The trusted driver then feeds the compiled proposal to the unchanged production
Handoff verifier using the retained canonical request and independently
retained request digest. Compiler and verifier errors are recorded explicitly.
The driver never repairs output, extracts a partial object from terminal text,
or treats a failure as success.

## Frozen runtime, restrictions, and run order

Exactly six independent, single-shot GitHub Copilot CLI invocations run in this
fixed alternating order:

1. `positive-1`
2. `negative-1`
3. `positive-2`
4. `negative-2`
5. `positive-3`
6. `negative-3`

Every invocation uses:

- Runtime: GitHub Copilot CLI `1.0.90-5`.
- Resolved executable SHA-256:
  `a21a8624194aabc709cdd8ec564b653efb0d771057607b040047e9a1c569a25b`.
- Model: `gpt-5.6-sol`, selected explicitly.
- Available model tools: exactly `view` and `apply_patch`.
- `--allow-all-tools`, `--no-custom-instructions`,
  `--disable-builtin-mcps`, `--no-auto-update`, `--no-ask-user`,
  `--disallow-temp-dir`, `--no-remote`, and JSON output.
- Memory disabled by omission of `--enable-memory`.
- Runtime usage written directly to a tester-owned path outside the model
  workspace.

Before each invocation the launcher resolves the retained executable again,
checks its exact version and SHA-256, and stops before the model call on any
mismatch. It does not substitute another runtime. If `gpt-5.6-sol` is
unavailable, the study stops without replacement.

## Frozen isolation and mutation accounting

Each run starts in a fresh mode-0700 disposable parent containing only:

- `request/`, a byte-for-byte copy of the condition request.
- `output/`, an empty mode-0700 directory.

The exact prompt is supplied as the non-interactive CLI prompt. Runtime logs,
usage, canonical requests, retained digests, compiler and verifier results,
source repository, other runs, expected answers, prior studies, and controls
remain outside model file access. Runtime metadata may exist beside, but never
inside, the disposable parent.

The sole intended model output is `output/semantic-draft.json`. Missing output,
extra output, malformed output, and runtime failure are final outcomes and are
not retried. Terminal content is not reconstructed into a draft.

For each run the driver hashes the request copy before and after, checks the
canonical request, source fixture, and repository tree before and after, and
checks for files outside the sole output path. It records four separate
mutation counts:

1. Request-copy mutations.
2. Canonical-request mutations.
3. Source-repository mutations.
4. Trust-boundary mutations.

The disposable workspace is removed only after raw output and sanitized
evidence are retained outside it. Deleting runtime traces never changes a
failed run into a success.

## Frozen failure rules and stop conditions

Each of these is retained as a failed stage without retry: missing or extra
output, invalid JSON, schema failure, draft canonicalization failure, quote
failure, unsafe path, target mismatch, unsupported operation, compiler failure,
proposal failure, verifier failure, runtime failure, or model failure.

Stop the remaining study without adaptation or substitution if:

- Either canonical request identity cannot be reproduced.
- The exact prompt, schema, runtime, or executable hash changes.
- The compiler or production verifier has an unresolved defect.
- Any model workspace can access protected material.
- Any canonical request, fixture, source file, repository trust anchor, or
  already frozen artifact is mutated.
- Independence of a later run is contaminated.
- The selected model becomes unavailable.

Otherwise finish exactly six invocations and publish all failures as observed.

## Frozen measures and scoring

For every run record condition, ordinal, order, runtime and executable identity,
explicit model, directly observable session ID, UTC start and end time,
launcher elapsed time, exit status, output count and hash, JSON and schema
validity, semantic outcome, canonical-draft hash when available, compiler
result, proposal hash, verification result and receipt hash, candidate IDs,
patch result, replay identity, all four mutation counts, and directly emitted
usage and timing fields. Do not estimate unavailable monetary cost or tokens.

Positive runs are scored separately at the semantic and mechanical stages:

- Expected-decision recovery.
- Exact evidence correctness.
- Target correctness.
- Operation and replacement correctness.
- Compiler result.
- Verifier result.
- Candidate ID and patch outcome.
- Unsupported claims and fabrication.
- Direct usage and timing.
- Timed trusted-review burden and disposition: `usable unchanged`,
  `usable after edits`, `rejected`, or `not supplied`.

Negative runs record correct semantic abstention, false-positive drafts,
false-positive candidates, compiler and verifier results, direct usage and
timing, and mutations.

Replay every retained schema-valid draft at least twice. Success requires
byte-identical proposal bytes and identities on every replay.

## Frozen productization threshold and disposition

The threshold passes only if all conditions hold:

1. At least 2 of 3 positive runs produce the verifier-valid expected candidate.
2. All 3 negative runs provide canonical semantic abstentions that compile into
   verifier-valid Handoff `no_decision` proposals.
3. Fabricated candidates total zero.
4. Request-copy, canonical-request, source-repository, and trust-boundary
   mutations each total zero.
5. Every retained valid draft replays to byte-identical proposal bytes,
   candidate IDs, and patch identities.
6. There is no material cost or review-burden regression.

For the final condition, the directly comparable prior-study all-run totals are
150,489 input tokens, 8,764 output tokens, 27 model requests,
41,781,200,000 nano-AIU, and 191,778 session milliseconds. A cost regression is
material if any new all-run total exceeds its prior value by more than 25
percent. Directly reported monetary cost is compared only if both studies
report it. The prior timed-review median was 1.5 milliseconds. Review burden is
materially worse if the new timed-review median exceeds 10 milliseconds, or if
any verifier-valid positive candidate needs manual repair to become usable.
Unavailable measures are reported unavailable and do not become estimates.

Disposition rules:

- `threshold_passed`: every condition above passes. The only authorized next
  step is a later experimental product-design proposal.
- `threshold_failed`: any condition fails. No productization authorization is
  made.
- `study_stopped`: a frozen stop condition prevents all six uncontaminated
  invocations. No threshold pass is possible.

## Frozen publication assessment

After results are frozen, assign exactly one:

- `publishable_result` if the productization threshold passes with inspectable
  semantic, replay, verification, usage, and mutation evidence.
- `useful_engineering_note` if the threshold fails but the intervention yields
  at least one verifier-valid expected positive candidate, or preserves all
  abstention and trust guarantees while exposing a specific new draft/compiler
  failure with inspectable evidence.
- `insufficient_new_evidence` otherwise, including an uncontaminated repetition
  that neither recovers a positive candidate nor isolates a new failure stage.

Only after results freeze may the report state the strongest supported thesis,
3 to 5 technical lessons, inspectable artifacts and measurements, what failed,
a concrete blog outline, and three factual title options. It must not contain a
polished public blog post.

## Forbidden claims and fixed limitations

The report must preserve these limitations unless execution differs: two small
public fixtures, one hosted model and runtime, three runs per condition, and one
scorer/reviewer.

It must not claim statistical significance, superiority, adoption readiness,
safety generalization, general model quality, production readiness, or that a
passing threshold validates a product change.
