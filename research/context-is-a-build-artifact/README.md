# Context Is a Build Artifact

**Status:** preregistered design / no model runs

Working title:

> Context Is a Build Artifact: Deterministic Context Compilation for
> Calibrated AI Decisions

Research question:

> Does byte-stable context compilation improve decision consistency,
> calibration, selective risk, and cost under equivalent source perturbations?

This directory freezes an independently reviewable study design before any Jev
integration or paid execution. It contains no model adapter, provider call,
credential, outcome, or claim that an experiment has run.

## Documents

- [Preregistration](preregistration.md): hypotheses, cases, arms, outcomes,
  analysis, stopping rules, reproducibility, ethics, and execution gates.
- [Related work](related-work.md): source matrix separating first-party claims
  from independent findings.
- [Pilot receipt schema](pilot-schema.json): JSON Schema for case, context,
  decision, usage, error, and evidence records. It defines records only and
  executes nothing.
- [Receipt validation](receipt-validation.md): canonical digest projections
  and cross-record checks required in addition to JSON Schema validation.
- [Offline pilot clarification](pilot-protocol-v1.md): prospective frozen
  seven-question and untuned routing details for synthetic excluded fixtures.
- [Offline pilot harness](pilot/README.md): zero-call generation, validation,
  and future-adapter gates.
- [Final-study prospective amendment](final-amendment-v1.md): exact repository
  allocation, condition count, arm/schedule freeze, policy precedence, custody,
  and the non-evaluable confirmatory consequence of the 21-base allocation.
- [Final-study amendment registry](final-amendment-v1.json): machine-readable
  counts, seeds, thresholds, analysis constants, budget, and external gates.
- [Final-study external gate review v2](final-external-gates-v2.md): current
  prospective TypeSafe provider-gate status and no-go blockers.
- [Final-study external gate review v1](final-external-gates-v1.md): historical
  frozen public-evidence review and exact 17-question clarification record.
- [Final-study source registry](final-source-registry-v1.json): immutable
  cross-repository commits, licenses, paths, and raw artifact digests.
- [Final-study receipt schema](final-receipt-schema-v1.json): exact Draft
  2020-12 schema packaged with the final offline corpus and applied before
  semantic receipt validation.
- [Final-study contamination ledger](final-contamination-ledger-v1.md):
  excluded-pilot comparison and the bounded repository-level holdout claim.
- [Local context-control preregistration](local-control-preregistration-v1.md):
  separate 14-base development/calibration study for an offline local open
  model; it cannot alter or repair the frozen TypeSafe final study.
- [Local context-control result projection](local-control-result-schema-v1.json):
  strict one-field categorical generation schema.

## Distill Lock v0 anchors

The study is anchored to the reviewed Distill Lock v0 implementation merged by
PR [#97](https://github.com/Siddhant-K-code/distill/pull/97).

| Anchor | Value |
|---|---|
| Merge commit | `a9b14667024c27c32c9c9dfb9c8dc35865990979` |
| Reviewed head | `4a492ccc75bb69904de40f84dfd6063432f99275` |
| Reviewed base | `48a816c5edee15fc6d18edd7872a4d16d6f310cc` |
| `context.bundle.md` SHA-256 | `3096d7b4492124df4e893d27b15a9d59ffe8a9fad80d24cd8265262c2251866a` |
| `context.lock.json` SHA-256 | `a4d44357e284f359c970af70aea744346703ecd37e8058bf7b8d3bd30f11842b` |
| `context.manifest.json` SHA-256 | `7f6849eb7f22a1de00e851bfd42ccef32dea1d82751bca9d7c592286ec6929fd` |
| `SHA256SUMS` SHA-256 | `ad4d593b7ded2a53023a24e767351b21570839f0b4fea9348d8e600252b8b0ee` |

The relevant commands are `distill lock`, `distill build`, `distill verify`,
and `make distill-lock-demo`. Distill Lock stabilizes context bytes and
provenance. It does not make a model or provider deterministic.

The frozen `pilot-schema.json` SHA-256 is
`1a63ab0a3f3e377e09b54c8993bc0d2bc8fb3fe3512fc401c10bc0f188ee2e41`.
The frozen `final-receipt-schema-v1.json` SHA-256 is
`074277e7b6f9ea30b6659ff7ea8d2d72433219b086c0fc3ef731a5679852c5bf`.
The frozen `final-source-registry-v1.json` SHA-256 is
`b5329538427ee98c6aff4027506622fd1b4c4b5920d2acd9bc470f2da9e2059c`.

## Independent review

External reviewers can review this directory without provider access.
Recommended review order:

1. Check the frozen claims, hypotheses, split, outcomes, and analysis in the
   preregistration.
2. Validate `pilot-schema.json` with a JSON Schema Draft 2020-12 validator and
   confirm its SHA-256 above before inspecting its identity exclusions.
3. Verify every related-work URL and the distinction between first-party
   claims and independent evidence.
4. Recompute all four Distill Lock artifact hashes with
   `go test ./pkg/lock -run '^TestGoldenFixture$' -count=1`, then run
   `make distill-lock-demo` for repeatability and mutation evidence.
5. File review comments before any dated execution amendment is accepted.

Material protocol changes require a versioned, dated amendment with rationale
before final execution. Pilot observations cannot silently change this file.

## Non-goals

- implementing or integrating Jev;
- selecting `jev-latest` or any moving model alias;
- calling a model or provider;
- creating, requesting, or storing an API key;
- spending credits or running a paid experiment;
- changing Distill Lock v0 behavior;
- treating model agreement as ground truth;
- presenting illustrative numbers as observed results.
