# Handoff semantic-draft compiler experiment

Status: completed on 2026-09-30 with one preregistration deviation. The frozen
productization threshold failed. Publication assessment:
`useful_engineering_note`.

This is a two-fixture, six-run research result. It is not a product change,
significance result, superiority claim, adoption recommendation, safety
generalization, or general model-quality claim.

## Result

On these fixtures, the semantic/mechanical split closed the positive
construction gap while preserving negative abstention:

- All 3 positive runs produced the expected semantic retry-policy intent.
- All 3 positive drafts compiled into verifier-valid expected candidates with
  the exact expected patch outcome.
- All 3 negative runs abstained, compiled into verifier-valid `no_decision`
  proposals, and produced no false-positive candidate.
- All 6 retained drafts replayed to byte-identical proposal bytes, candidate
  IDs, and patch identities.
- Fabricated candidates, unsupported claims, request-copy mutations,
  canonical-request mutations, source-repository mutations, and trust-boundary
  mutations were all zero.

The preregistered productization threshold nevertheless failed. Directly
reported model requests increased from 27 in the prior capability-control study
to 36 here, a 33.3 percent increase and more than the frozen 25 percent
material-regression limit. The other directly comparable totals did not
regress: input tokens fell from 150,489 to 148,524, output tokens fell from
8,764 to 3,217, and nano-AIU fell from 41,781,200,000 to 24,892,260,000.
The current runtime did not directly report session duration, so none is
estimated.

The strongest honest thesis is therefore narrow: for one positive and one
negative public fixture, exact semantic quotes and a deterministic compiler
converted a previously observed construction bottleneck into 3 of 3 valid
positive candidates without losing 3 of 3 negative abstentions, but the
intervention did not satisfy its own cost gate.

## Frozen design

The complete [protocol](protocol.md), exact [prompt](prompt.txt), semantic
[JSON Schema](semantic-draft.schema.json), and [frozen manifest](frozen-manifest.json)
were hashed before the first model invocation.

| Frozen artifact | SHA-256 |
|---|---|
| Protocol | `8d76e52cd88ee5896add3f76f9aa3afe1a9044ecf019b56a94e9fc0d35039ea5` |
| Prompt | `f348b76c98ed70a09835e17c14d9b281da113dc45ab142a1a382829899f9838d` |
| Semantic-draft schema | `7f05117827b50c7ddec7ac33fbab9b58194aa272a698588e870f93fec34ce5a6` |
| Frozen manifest | `48ed6906220b6125ba8f36bb7a582b257e411f6439cf2cba802cde1e775a2a61` |
| Compiler source | `fd5bd856f4bf9cdee49e2705b48fbdb369e80023cf720101457d59df9bad2311` |
| Frozen compiler binary | `335050613d3cf9db8848031ffcd1e06d527911a245d885fda8a9eca5ffba5b2c` |
| GitHub Copilot CLI executable | `a21a8624194aabc709cdd8ec564b653efb0d771057607b040047e9a1c569a25b` |

Every run used GitHub Copilot CLI `1.0.90-5`, explicitly selected
`gpt-5.6-sol`, and exposed exactly `view` and `apply_patch` to the model. The
fixed single-shot order was `positive-1`, `negative-1`, `positive-2`,
`negative-2`, `positive-3`, `negative-3`. There were no model retries, draft
repairs, or terminal-output reconstructions.

The model supplied only an abstention or one semantic intent: decision text and
type, exact evidence quote, target path, exact complete-line anchor, `replace`
operation, replacement line, and rationale. The research-only compiler derived
ranges, hashes, the unified diff, patch digest, candidate ID, and canonical
proposal bytes. The unchanged production verifier then checked each proposal
against the retained canonical request and external digest.

## Per-run outcomes

`Valid` means semantic draft schema validation, deterministic compilation, and
unchanged Handoff verification all passed. Mutation columns are request copy,
canonical request, source repository, and trust boundary.

| Order | Run | Semantic result | Valid | Candidate | CLI ms | Requests | Input | Output | Mutations |
|---:|---|---|---|---|---:|---:|---:|---:|---|
| 1 | `positive-1` | expected intent | yes | `decision_179c38986244e95383ae56153d464e8328b2e3b55419716a1070339ac75c429f` | 28,426 | 7 | 29,221 | 692 | 0/0/0/0 |
| 2 | `negative-1` | correct abstention | yes | none | 33,772 | 5 | 20,295 | 381 | 0/0/0/0 |
| 3 | `positive-2` | expected intent | yes | `decision_ca5c754c61dcc9a2b18a5c5e72f5e82920746f1ddbc91d6ff88bf5fe943573e6` | 41,815 | 7 | 29,225 | 706 | 0/0/0/0 |
| 4 | `negative-2` | correct abstention | yes | none | 20,105 | 5 | 20,389 | 411 | 0/0/0/0 |
| 5 | `positive-3` | expected intent | yes | `decision_6c0bb5bec4490ae8972a87407345b532683d0ba4f30bf0305c3cf83657d95ed4` | 24,975 | 7 | 29,114 | 668 | 0/0/0/0 |
| 6 | `negative-3` | correct abstention | yes | none | 21,048 | 5 | 20,280 | 359 | 0/0/0/0 |

All three positive proposals used patch SHA-256
`645e3cd036647c78073739e00b11e09e9efb34bea701dd248b6a5522e4a97f23`,
the exact checked-in expected patch. Candidate IDs differ because candidate
identity correctly binds each run's independently worded decision, evidence
span, and rationale. Replay identity remained exact within every retained
draft.

Two positive drafts quoted the complete speaker-prefixed evidence line; one
quoted the narrower fixture-expected decision text. All three quotes matched
unique exact conversation bytes, derived valid ranges, and passed verification.
The narrower fixture quote matched exactly in 1 of 3 runs; evidence correctness
was 3 of 3.

## Threshold

| Frozen requirement | Required | Observed | Passed |
|---|---:|---:|---|
| Positive verifier-valid expected candidate | at least 2/3 | 3/3 | yes |
| Negative verifier-valid abstention | 3/3 | 3/3 | yes |
| Fabricated candidates | 0 | 0 | yes |
| Four mutation classes | 0 | 0 | yes |
| Byte-identical replay and identities | 6/6 | 6/6 | yes |
| No material cost or review regression | true | false | no |

The disposition is `threshold_failed`. This result does not authorize a product
change or a product-design proposal.

## Deviation and scoring record

The CLI created a six-path runtime-owned `.agent-traces` tree beside
`request/` and `output/` in every disposable workspace. The frozen protocol
said runtime metadata would remain outside that parent. The trace tree
contained no canonical request, trusted digest, source repository, other-run
data, expected answer, compiler result, or verifier result. It did not change
the request copy or output and was removed with the disposable workspace. It
is recorded as a metadata-placement deviation, not a model output or
trust-boundary mutation. The user-specified isolation pattern explicitly
allowed runtime metadata beside the request and output, but the stricter
protocol sentence still makes this a preregistration deviation.

The first pre-publication collector also used an overly literal
case-sensitive comparison for the positive decision text. Before results were
frozen, the scorer was corrected to recognize the same expected sentence with
its initial capital. Raw drafts, compiled proposals, candidate IDs, patch
digests, replay outputs, and verifier receipts did not change. The correction
is recorded in [results.json](results.json).

An orchestration interruption occurred after the first successful trusted
compile but before its compile timing was retained. `positive-1` therefore has
no initial compiler elapsed value. Its proposal bytes were preserved, two
subsequent replays were byte-identical, and trusted verification passed. No
model call was repeated.

## Inspectable evidence

- [results.json](results.json) contains per-run usage, timing, scoring,
  identities, threshold checks, deviations, and aggregates.
- [drafts/](drafts/) preserves the exact raw model files.
- [canonical-drafts/](canonical-drafts/) preserves typed canonical forms used
  for replay.
- [proposals/](proposals/) preserves all six compiled Handoff proposals.
- [reviews/](reviews/) preserves verifier reviews, receipts, patches, and
  checksums. Exact review and patch bytes are base64-wrapped in small JSON
  records so the research evidence does not make `git diff --check` treat
  unified-diff context prefixes as trailing whitespace.
- [responses/](responses/) preserves exact `assistant.message` content only,
  with source-stream and content hashes plus explicit sanitization records.
- [compiler.go](compiler.go), [main.go](main.go), and
  [compiler_test.go](compiler_test.go) contain the research-only compiler and
  its focused success, abstention, ambiguity, mismatch, unsafe input,
  malformed input, replay, and verifier-acceptance tests.
- [run-study.py](run-study.py) and [collect-results.py](collect-results.py)
  preserve launcher and scoring logic.
- [SHA256SUMS](SHA256SUMS) binds every committed study artifact except the
  checksum file itself.

No raw runtime event stream, log, request bundle, trusted digest file, machine
path, credential, or disposable trace is committed.

## Technical lessons

1. Exact semantic quotes and complete-line anchors were sufficient to derive
   every Handoff mechanical field deterministically for the positive fixture.
2. Separating semantic extraction from proposal identity preserved abstention:
   all negative drafts became ordinary verifier-valid `no_decision` proposals.
3. Cross-run candidate IDs need not agree when rationale or evidence span
   differs. Replay determinism per retained draft is the relevant mechanical
   guarantee.
4. Token and nano-AIU totals can improve while another frozen cost proxy gets
   worse. The model-request count, not tokens, caused this threshold failure.
5. Runtime-owned trace placement needs an explicit preregistered allowlist and
   location check even when the model tool allowlist is correct.

## What failed

- The no-material-regression gate failed because 36 model requests exceeded
  the frozen maximum of 33.75.
- Runtime trace metadata appeared inside the disposable workspace rather than
  outside it as the frozen protocol stated.
- The first collector needed a documented pre-publication scoring correction.
- One initial compiler timing value was lost; proposal and replay evidence were
  not lost.

The study still has only two small public fixtures, one hosted model/runtime,
three runs per condition, and one scorer/reviewer. It cannot establish
frequency, significance, superiority, adoption readiness, safety outside these
fixtures, or general model quality.

## Publication assessment and possible outline

Frozen assessment: `useful_engineering_note`. The result meets that criterion
because at least one verifier-valid expected positive candidate was recovered,
all abstention and trust guarantees were preserved, and the cost failure is
directly inspectable. It does not meet the study's `publishable_result`
criterion because the complete productization threshold did not pass.

A factual engineering-note outline:

1. Baseline: semantic recognition without valid candidate construction.
2. Intervention: minimal semantic draft and deterministic compiler.
3. Trust boundary: retained digest, disposable workspace, and unchanged
   verifier.
4. Six frozen runs: 3 of 3 positive candidates and 3 of 3 abstentions.
5. Determinism: exact replay, stable patch outcome, and candidate identity.
6. Failure: higher model-request count and runtime metadata placement.
7. Next experiment: reduce request count without weakening the contract.

Three factual title options:

- "Semantic Drafts Produced 3 of 3 Valid Handoff Candidates"
- "A Deterministic Compiler Closed One Handoff Construction Gap"
- "Hashes Out, Semantics In: Six Frozen Handoff Trials"

These are title options and an outline only, not a polished public post.

## Reproduction

Run the offline compiler and Handoff checks:

```bash
go test ./research/handoff-semantic-draft-compiler -count=1
go test ./pkg/handoff ./cmd -count=1
make handoff-v0-demo
```

The compiler is intentionally not exposed through the public `distill` CLI.
From a clean checkout, build it directly:

```bash
go build -o /tmp/semantic-draft-compiler \
  ./research/handoff-semantic-draft-compiler
```

Then prepare the matching fixture outside the repository, pass the independently
retained request digest to the compiler, and verify the compiled proposal with
the normal `distill handoff verify` command. The exact frozen launch and
collection commands are represented by the two research scripts; executing
them makes six hosted model calls and must not be used as an offline
reproduction step.
