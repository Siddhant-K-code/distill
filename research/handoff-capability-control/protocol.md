# Frozen Handoff positive-negative capability-control protocol

Frozen before any proposal invocation on 2026-09-30.

## Question and baseline

This preregistered diagnostic distinguishes ambiguity in the historical PR #112
corpus from inability to construct a complete valid Handoff proposal. It does
not test prompt tuning or product behavior.

- Repository baseline: `167212e42e360fadc8b17e8f079a2b4f0f7e3411`.
- Compatible dependency merge in the baseline ancestry:
  `b5b246a`.
- Handoff implementation, embedded instructions, proposal schema, fixtures,
  Pilot Kit, and verifier remain unchanged.
- Historical context only: the PR #112 repeatability study and temporal-state
  guard ablation each recovered 0 of 4 expected decisions in every run.

## Frozen conditions and order

There are exactly two conditions with exactly three independent single-shot
runs per condition:

1. Positive control: `testdata/handoff-v0/retry-policy` conversation and docs.
2. Negative control: `testdata/handoff-v0/brainstorming` conversation and docs.

The fixed alternating run order is:

1. `positive-1`
2. `negative-1`
3. `positive-2`
4. `negative-2`
5. `positive-3`
6. `negative-3`

One GitHub Copilot CLI invocation is one run. There are exactly six planned
invocations. No invocation may be retried or replaced.

The checked-in expected proposal for each fixture is hidden from all model
runs. It may be opened only by the trusted scorer after all six model
invocations finish.

## Frozen request identities

Both fixtures are prepared with Handoff built from the repository baseline.
The printed request digest is retained independently outside the canonical
request and every model workspace.

Positive control:

- Request ID:
  `94730d11355f823740c1a4d9a7ff73c8841bc6c0038e8f6ac1c0d57829a75487`.
- Tester-retained request SHA-256:
  `cd32864f9e42b44e0341bb9199dabb28a032103b8e2d9e9b4f4c19d61d3aea38`.
- Source conversation SHA-256:
  `94a8d1951427b388d37919fecee4d68ae2f0d4398199b29214ce5230ef74077a`.
- Source document SHA-256:
  `21f128d20f43ebeaa8a37a8519363103ea8cf4d57b126c7643879bfc07fbea70`.

Negative control:

- Request ID:
  `eb613088a5e48b03b2f1cac9c47a6a6338f02dc859e3f3e6afd0b686cb36df47`.
- Tester-retained request SHA-256:
  `45d333595c146fcd8ade578f6015258dda8c57275b75352d63ca08f8c89fb4d2`.
- Source conversation SHA-256:
  `a706e599c3a4b29b8c7e289e21ed2b9fd8376e4e6c7ca69a7dd25ec308d05326`.
- Source document SHA-256:
  `12f10d33f8c077c0611e58bcdee398c9d6e93641fbf91e62aa6a31a5cc64a4c7`.

The proposal schema SHA-256 for both requests is
`a0cb1730ea215fa276774546f245d2994fa3d6a35879b8a806711c3ef09aea2f`.

## Frozen runtime and prompt

Every run uses:

- Runtime: GitHub Copilot CLI 1.0.90-5.
- Installed executable SHA-256:
  `a21a8624194aabc709cdd8ec564b653efb0d771057607b040047e9a1c569a25b`.
- Model: `gpt-5.6-sol`, selected explicitly.
- Available model tools: exactly `view` and `apply_patch`.
- Custom instructions disabled.
- Built-in MCP servers disabled.
- Memory disabled.
- Automatic updates disabled.
- Ask-user disabled.
- Automatic access to the surrounding system temporary directory disabled.
- The unchanged strict file-output prompt from
  `docs/handoff-pilot-kit.md`, preserved separately as `prompt.txt`.

Before every invocation, the launcher must verify that the same resolved
executable reports version 1.0.90-5 and has the frozen SHA-256. The launcher
must pass `--model gpt-5.6-sol`, `--available-tools view apply_patch`,
`--allow-all-tools`, `--no-custom-instructions`, `--disable-builtin-mcps`,
`--no-auto-update`, `--no-ask-user`, and `--disallow-temp-dir`. It must not add
a temporal guard, corpus hint, expected outcome, prior result, candidate
assistance, or reasoning instruction.

## Isolation and execution

Each run receives a fresh mode-0700 disposable parent containing only a copied
`request/` and an empty `output/` before launch. The model runs with that parent
as its working directory.

The canonical requests, tester-retained digests, expected proposals, source
repository, this protocol, prior run outputs, runtime logs, usage records, and
verifier results remain outside the model workspace and unavailable to its
file tools. Runtime metadata is written outside the model workspace.

Do not repair JSON, extract a terminal object, edit a proposal, reveal prior
outputs, add advice, or adapt later runs. After retaining the raw proposal when
present, sanitized execution evidence, output count and hash, mutation counts,
and verification evidence, remove the disposable parent safely.

## Frozen per-run measures

For every run record:

- Condition, ordinal, and fixed order position.
- Exact runtime name, version, executable SHA-256, explicit model, and session
  identifier when directly observable.
- UTC start and end time, launcher elapsed time, and directly reported session
  and API timing.
- Directly reported token, request-credit, or cost fields. Do not infer
  monetary cost.
- Model exit status.
- Output file count, raw proposal presence, and proposal SHA-256 when present.
- JSON parse validity, schema validity, canonical validity, identity validity,
  trusted verification result, and verification receipt SHA-256.
- Outcome, candidate count, and exact candidate IDs when parseable.
- Request-copy, canonical-request, source/trust-fixture, and trust-boundary
  mutation counts.
- Timed trusted-review duration.

## Frozen scoring

Trusted verification always uses the canonical request and the independently
retained digest.

Positive runs are scored against the hidden checked-in positive expected
proposal for:

- Recovery of the expected retry decision.
- Exact evidence validity.
- Target correctness.
- Candidate ID validity.
- Patch validity.
- Patch semantic equivalence to the expected patch, or reviewer disposition as
  `usable unchanged`, `usable after edits`, `rejected`, or `not supplied`.
- Unsupported claims and fabricated candidates.

Negative runs are scored against the hidden checked-in negative expected
proposal for:

- Correct abstention.
- Any false-positive candidate, including a candidate in an otherwise invalid
  proposal.

Within each condition report:

- Byte-identical output agreement over all three possible run pairs.
- Outcome agreement over all three possible run pairs.
- Exact candidate-set agreement over all three possible run pairs.
- Trusted verification success.
- Positive expected-decision recovery or negative correct abstention.
- Review duration for each run plus median and range.

## Frozen capability threshold and diagnostic classification

Apply classifications in this order.

`full_proposal_capability_observed` requires all of:

1. At least 2 of 3 positive runs produce a verifier-valid candidate covering
   the expected retry decision.
2. At least 2 of 3 negative runs produce verifier-valid `no_decision`.
3. Zero fabricated positive candidates.
4. Zero negative false-positive candidates.
5. Zero trust-boundary mutations across all six runs.

Otherwise classify `construction_bottleneck_observed` if at least one positive
run contains text that substantively recovers the expected retry decision in
its raw proposal or directly returned CLI response, but that recovery fails
proposal construction or trusted verification.

Otherwise classify `decision_recovery_bottleneck_observed` if at least 2 of 3
positive runs abstain or omit the expected retry decision while at least 2 of 3
negative runs produce verifier-valid `no_decision`.

Otherwise classify `mixed_or_inconclusive`.

The report is descriptive only. It must not make significance, superiority,
adoption, or general-quality claims.

## Frozen failure handling and stop rules

Missing output, extra output, invalid JSON, noncanonical JSON, schema failure,
identity failure, evidence failure, patch failure, request-copy mutation,
trust-boundary escape, runtime error, or model error is retained as a negative
run without retry.

Stop the study without adaptation or substitution if:

- Either canonical request identity cannot be reproduced.
- A Handoff product defect blocks preparation or trusted verification.
- Any run can access protected material.
- A frozen input, canonical request, source fixture, repository, or trust
  anchor is mutated.
- Independence of subsequent runs is contaminated.
- The exact runtime version or executable SHA-256 changes.
- `gpt-5.6-sol` becomes unavailable.

After exactly six uncontaminated invocations, independently verify every
proposal that can be submitted to the verifier, score only then against the
hidden expected proposals, assert hashes and aggregate semantics, run the
preregistered repository validation, preserve only sanitized evidence, and
stop. Do not start product changes.
