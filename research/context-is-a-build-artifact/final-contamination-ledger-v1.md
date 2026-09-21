# Final-Study Contamination Ledger v1

**Record:** `context-build-artifact/final-contamination-ledger/v1`

**Frozen:** 2026-09-20

**Evidence available:** merged preregistration, excluded pilot source and
generated package, and the public source registry. No final label, provider
response, threshold outcome, or aggregate result was available.

## Repository role decision

Distill remains threshold-development only. LLMTraceFX remains
safety-calibration only. Neither is eligible for held-out analysis because
their implementations and evidence families informed protocol or pilot work.

AgentTrace at
`b109ec5b3714b842746e97ee8e975329d8582667` is eligible for the
repository-level threshold holdout under the checks below. This is not a claim
of analyst blindness or freedom from unknown public-model pretraining exposure.
The seven AgentTrace family definitions were known before corpus generation.

## Pilot exclusion audit

The audit inspected:

- merged source under `internal/studypilot` and `internal/jevpilot`;
- the excluded pilot documentation;
- a newly generated excluded pilot package produced by the merged
  `distill-jev-pilot` command; and
- the exact AgentTrace commit, family IDs, anchor paths, anchor hashes, and
  complete public synthetic-bundle member hashes in
  `final-source-registry-v1.json`.

The generated pilot package contained eight files. Running the merged command

```text
go run ./cmd/distill-jev-pilot summarize <generated-pilot-directory>
```

reproduces dataset snapshot
`a939914f5a752272bbe7e2f9c6e807222add930927d013f298d3d1bd5a3ef261`,
pilot manifest
`e25a307506d01887947365dc5b9fd047343f73b09c8da953e76846bb57b024c7`,
and checksum-manifest
`a3d5e55e81781f6eb0b243c4cd64438a71ebedb83bbb85958fb79db998a30895`.

Results:

| Check | Result |
|---|---|
| AgentTrace repository name or commit in pilot source/generated artifacts | absent |
| AgentTrace final family IDs in pilot source/generated artifacts | absent |
| Exact AgentTrace anchor-file digest reused by a pilot artifact | absent |
| Exact AgentTrace public synthetic-bundle member digest reused by a pilot artifact | absent |
| Pilot case/request/label identifiers reused as final identifiers | prohibited and enforced by final validator |
| Pilot artifacts admitted to the final namespace | prohibited and enforced by final validator |

The audit establishes absence of exact AgentTrace source, fixture, test, case,
label, or output reuse in the excluded pilot artifacts available for review.
It cannot establish absence from unknown model pretraining and makes no such
claim.

## Prospective access rules

Before held-out execution:

1. AgentTrace source bytes may be fetched only through the immutable
   content-addressed registry.
2. The runner receives label-free assignment bundles and commitments only.
3. No AgentTrace label, receipt, aggregate outcome, or repository-specific
   threshold may enter implementation, threshold development, safety
   qualification, prompt/rubric revision, policy tuning, or adjudicator
   calibration.
4. Newly generated privacy canaries must be non-secret and must not copy
   credential-shaped upstream test literals.
5. A complete AgentTrace assignment bundle is the evidence unit. A trace alone
   cannot establish attachment completeness, outcome consistency, privacy, or
   integrity.
6. Any early access or exact derivative discovered later downgrades AgentTrace
   prospectively to confirmatory secondary data. It must not be called held
   out.
