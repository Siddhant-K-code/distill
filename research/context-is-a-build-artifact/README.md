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
`63f96c8eb1cacee9a072d43ae508f8628acc6ccd86bcf5926ca6954975c06238`.

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
