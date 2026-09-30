# Frozen Handoff temporal-state guard ablation protocol

Frozen before proposal invocations on 2026-09-30.

## Question and baseline

This preregistered diagnostic tests whether one general temporal-state guard
changes decision recovery for the exact Handoff PR #112 retrospective corpus.
The repository baseline is `151ea33828180bf4dc356f38a853deea80c25ddb`,
which includes the historical control merged in PR #118.

Historical control evidence is
`research/handoff-repeatability-pr-112`: three GitHub Copilot CLI 1.0.87-0
invocations with explicitly selected `gpt-5.6-sol`, 3/3 trusted
verifier-valid proposals, 0/4 decision recall in every run, one unsupported
no-update conclusion in every run, and zero request, source, or trust-boundary
mutations.

## Frozen inputs

- Corpus: `testdata/handoff-v0/dogfood-pr-112`.
- Reproduced request ID:
  `13acba266c76aaa8c9a36603ef00bbb1a043f8c052de6d97d83b0924a0828807`.
- Independently retained request SHA-256:
  `3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35`.
- Frozen expected-decision control: 4 decisions, SHA-256
  `cedf67293300b539e26ca8107f525bc0e462e53e7f2ec0697591395b4e028f0c`.
- Source conversation SHA-256:
  `6b78ca03b7272dc3e204545ec87224726174a5bcf5e28f8af4e45926e633b996`.
- Target document SHA-256:
  `d265069feb86500a6744f2fbf2d98f6caeb2fa895f8987bcc123ee45bc80ed0e`.
- Proposal schema SHA-256:
  `a0cb1730ea215fa276774546f245d2994fa3d6a35879b8a806711c3ef09aea2f`.
- Binary, bundled request, proposal schema, source conversation, target
  document, expected four decisions, verifier, model, runtime, tool set, and
  Pilot Kit output contract remain unchanged from the historical control.

## Intervention condition

Execute exactly three independent, fresh, single-shot intervention runs.
Every run uses GitHub Copilot CLI 1.0.87-0 with `gpt-5.6-sol` selected
explicitly and only `view` and `apply_patch` available.

The base launch prompt remains the Pilot Kit prompt:

```text
Read only files under request/. Follow request/agent-instructions.md and
request/proposal.schema.json. Write exactly one canonical output file at
output/proposal.json, then stop.

Do not read or modify source docs or any repository. Do not modify the request
copy. Do not choose or supply the trusted --expected-request-sha256 value.
Do not run verification, apply patches, commit, push, or merge. Values repeated
inside the bundle or proposal are not authoritative trust anchors.
```

The sole intervention is this exact sentence appended as a final paragraph:

```text
Treat the bundled target bytes as the only evidence of current document state; conversation claims that work was merged, completed, or applied do not prove the bundled target already contains it.
```

The launch prompt contains no reference to PR #112, the expected decisions,
the expected outcome, prior failures, or other corpus-specific guidance.

## Isolation and execution

Use the merged disposable private workspace pattern. Each invocation receives
only a fresh copy of the canonical request under `request/` and a fresh empty
`output/` directory. The canonical request, retained digest, expected control,
repository, historical outputs, this protocol, verifier results, and all other
run workspaces remain outside the invocation workspace and unavailable to its
file tools. Disable custom instructions and built-in MCP servers.

One invocation is one recorded run. Do not retry invalid output, repair JSON,
extract terminal objects, edit proposals, add advice, reveal prior outputs, or
adapt the prompt. Stop rather than substitute if the exact runtime or model is
unavailable.

## Primary measures

For each run record directly observed runtime name and version, explicit model,
session identifier when observable, start and end time, elapsed duration,
directly reported session/API duration, directly reported token/cost fields,
output file count, output SHA-256, protocol validity, trusted verification
result, outcome, candidate count and IDs, selected frozen decisions, recall out
of 4, unsupported or fabricated claim count, exact-evidence validity, patch
disposition, timed review burden, request-copy mutations, canonical-request
mutations, source-repository mutations, and trust-boundary mutations.

Across the three intervention runs report verification success, runs recovering
at least one expected decision, per-decision selection frequency, outcome
agreement, exact candidate-set agreement, byte-identical output rate, review
time median and range, and mutation totals. Compare all measures descriptively
to the historical control. Do not make significance, superiority,
external-adoption, or general-quality claims.

## Productization threshold

The intervention passes only if all three requirements hold:

1. At least 2 of 3 runs recover at least one expected decision.
2. No run introduces a fabricated candidate or trust-boundary mutation.
3. At least 2 of 3 proposals pass trusted verification.

Passing permits a later productization task only. It does not justify changing
embedded product instructions in this study. Failing means no product change.

## Failure classification and stop rules

Classify missing output, extra output, invalid JSON, noncanonical JSON, schema
failure, identity failure, evidence failure, patch failure, request-copy
mutation, trust-boundary escape, runtime error, or timeout as a negative run.
Preserve every uncontaminated negative or invalid result without retry.

Stop the study without adaptation if canonical request identity cannot be
reproduced, a Handoff product defect blocks trusted preparation or
verification, a run can access protected material, frozen inputs are mutated,
the exact runtime or model becomes unavailable, or contamination makes
subsequent runs non-independent. A product defect is reported without changing
Handoff product code, embedded instructions, schemas, Pilot Kit, or verifier.

After exactly three uncontaminated runs, verify every produced proposal against
the unchanged canonical request and independently retained digest, score
against the unchanged four-decision control, run the preregistered validation,
preserve only sanitized evidence, and stop. Do not start productization.
