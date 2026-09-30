# Frozen Handoff repeatability protocol

Frozen before proposal invocations on 2026-09-30.

## Inputs and conditions

- Repository baseline includes merge commit
  `a15a8462395cde8ea3b55dd2d367a7d89baf0497`.
- Corpus: `testdata/handoff-v0/dogfood-pr-112`.
- Reproduced request ID:
  `13acba266c76aaa8c9a36603ef00bbb1a043f8c052de6d97d83b0924a0828807`.
- Independently retained request SHA-256:
  `3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35`.
- Frozen expected-decision control: 4 decisions, retained outside agent
  workspaces.
- Condition C1: GitHub Copilot CLI 1.0.87-0 with model
  `gpt-5.6-sol` selected explicitly.
- Three independent fresh invocations in C1.
- No alternative condition. Claude Code 2.1.222 is installed but reports
  `loggedIn: false`; no Ollama or LM Studio CLI/runtime is installed.
- Every invocation receives the unchanged prompt from
  `docs/handoff-pilot-kit.md`, an isolated copy of the request, and an empty
  `output/` directory. Available tools are restricted to `view` and
  `apply_patch`. No memory, repository, canonical request, retained digest,
  expected control, prior output, verifier result, or other run workspace is
  exposed.

## Primary measures

For each run record runtime, explicit model, CLI version, session identifier
when observable, start/end time, duration, output SHA-256, protocol validity,
trusted verification result, outcome, candidate count and IDs, selected frozen
decisions, decision recall out of 4, fabricated or unsupported claim count,
exact-evidence validity, patch disposition, timed review burden, request-copy
mutation count, canonical-request mutation count, source mutation count, and
tokens or cost only when directly reported by the runtime.

Within C1 report:

- Byte-identical output rate as identical output pairs divided by all 3
  possible run pairs.
- Outcome agreement as agreeing outcome pairs divided by all 3 pairs.
- Candidate-set agreement as pairs with exactly equal candidate ID sets divided
  by all 3 pairs.
- Per-decision selection frequency as runs selecting the frozen decision
  divided by 3.
- Verification success as valid trusted verifications divided by 3.
- Review burden as each timed review duration plus median and range.

## Failure classification and stop rule

One fresh CLI invocation is one recorded run. Missing output, extra output,
invalid JSON, noncanonical JSON, schema failure, identity failure, evidence
failure, patch failure, request-copy mutation, trust-boundary escape, runtime
error, or timeout remains a negative run. Do not retry, repair, extract a
terminal object, alter the prompt, reveal prior results, or help calculate
candidate fields.

Stop the study without adapting the protocol if the canonical identity cannot
be reproduced, a product defect blocks trusted preparation or verification, a
run can access protected material, the frozen inputs are mutated, or another
contamination event makes subsequent runs non-independent. Otherwise execute
exactly three C1 invocations, including failures.

## Scoring and review

A frozen decision is selected only when at least one verified candidate
substantively captures that decision. One candidate may select multiple frozen
decisions only when its decision text and patch cover each one. Decision recall
is selected frozen decisions divided by 4. A claim is fabricated or unsupported
when its decision text, evidence, reason, or patch asserts material not
supported by the exact cited conversation evidence and frozen target. Exact
evidence validity is `all`, `some wrong`, or `none`; a no-candidate result is
`none supplied`. Patch disposition is `usable unchanged`, `usable after edits`,
`rejected`, or `not supplied`. Review timing starts when the sanitized proposal
and trusted review output are opened and ends when scoring and patch
disposition are recorded. Results are descriptive only. With n=3, make no
significance, superiority, adoption, or general-quality claims.
