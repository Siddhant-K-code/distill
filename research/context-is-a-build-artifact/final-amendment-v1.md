# Final-Study Prospective Amendment v1

**Protocol:** `context-build-artifact/final-amendment/v1`

**Status:** prospective; no final corpus outcomes or provider responses accessed

**Date:** 2026-09-20

**Applies after:** the excluded offline and provider-integration pilots

**Execution effect:** freezes the final offline implementation and dataset
allocation. It does not authorize provider calls.

This amendment resolves ambiguities identified after the excluded pilots without
rewriting the preregistration. Pilot cases, source text, identifiers, labels,
request states, provider responses, thresholds, and receipts are excluded from
the final corpus. The authors had access to the merged protocol and excluded
pilot feasibility evidence, but no final-study labels, model outputs, aggregate
outcomes, or held-out receipts.

The machine-readable allocation and analysis parameters are in
[`final-amendment-v1.json`](final-amendment-v1.json). Its raw SHA-256, the final
implementation commit, source registry, corpus, split, schedule, question
schema, policy, validator, and analysis digests must be recorded in an
exact-head review receipt before any final provider call.

## 1. Unit, allocation, and confirmatory consequence

The final corpus contains exactly **21 independent base cases** and **150
clustered condition records** before context arms and replicates:

| Repository | Role | Split | Independent bases | Condition records |
|---|---:|---:|---:|---:|
| Distill | `threshold_development` | `calibration` | 7 | 77 |
| LLMTraceFX | `safety_calibration` | `calibration` | 7 | 41 |
| AgentTrace | `held_out` | `held_out_repository` | 7 | 32 |

The decision case remains the independent statistical unit. A condition record
is a base or registered within-base variant, not an independent case. Context
arms and replicates are repeated measures.

This allocation cannot meet the preregistered minimum of 25 accepted independent
base cases in threshold development, safety calibration, or held out. Section
8 therefore requires qualified autonomous coverage to be zero and makes H1-H5
non-evaluable. The study may still report the preregistered descriptive and
exploratory condition-level results with case-clustered uncertainty. It must not
substitute condition counts for independent accepted-case counts, relax the
minimum, or present a descriptive result as confirmatory.

The frozen risk ceiling is `0.15`; accepted-count minima remain `25/25/25`.
The threshold grid is
`0.50, 0.60, 0.70, 0.80, 0.90, 0.95`. The automatic selector therefore
returns no qualified candidate for this allocation. This expected
non-evaluability is a design consequence, not an outcome-dependent stop.

## 2. Repository roles and contamination

Distill is threshold-development only because the protocol, Lock implementation,
and excluded pilots informed implementation. LLMTraceFX is safety-calibration
only because its public evidence families informed protocol and pilot design.
Neither repository may enter held-out analysis.

AgentTrace is the declared repository-level threshold holdout at pinned commit
`b109ec5b3714b842746e97ee8e975329d8582667` only if the frozen contamination
ledger contains no exact AgentTrace source, fixture, test, case, label, output,
or derivative used in either pilot, prompt/rubric construction, policy tuning,
or adjudicator calibration. The analyst knows the seven case families, so the
claim is repository-level threshold holdout, not analyst blindness. Public
model-pretraining exposure is unknowable and must be disclosed. If any ledger
entry cannot establish absence, AgentTrace is prospectively downgraded to
confirmatory secondary data and must not be described as held out.

## 3. Decision and evidence precedence

The deterministic policy applies the strongest matching route:

1. Cryptographic, schema, privacy, closed-registry, or evidence-integrity
   contradiction overrides a benign wrapper and routes to `reject`.
2. An affirmative claim unsupported by the registered evidence routes to
   `reject`.
3. Honest incomplete, unsupported, unavailable, refused, or not-applicable
   evidence routes to `review` unless a separately registered task explicitly
   defines evidence-record correctness and the answer is correct for that
   record.
4. An explicit identity mismatch with an honest noncomparability conclusion
   routes to `review`; pooling, ranking, or claiming comparability despite the
   mismatch routes to `reject`.
5. Provider teardown is accepted only with independent provider-side deletion
   evidence. SSH failure, local process shutdown, or an absent local resource
   cannot establish provider deletion.
6. TTFT is the interval to first nonempty content before completion. Headers,
   an empty event, completion time, and buffered post-completion output are not
   TTFT; presenting buffered timing as TTFT routes to `reject`.
7. A checksum-valid artifact resealed against an invalid closed registry always
   routes to `reject`.

Honest refusal, `unsupported`, and `not_applicable` are distinguished from
successful task execution. They are never silently promoted to success.

## 4. Missing, null, zero, and envelopes

For every typed field:

- missing means the field or required evidence is absent;
- JSON `null` means measured or provider-reported data is unavailable where the
  schema explicitly permits it;
- numeric zero means a present measurement whose value is exactly zero; and
- `unknown` and `not_applicable` remain decision labels with their registered
  meanings.

No loader or analysis may coerce among these states.

An evidence envelope is closed. Every primary file, allowed supplement, and
generated transformation receipt must be declared before generation with a
media type, byte length, role, and digest. An undeclared extra file invalidates
the envelope. A supplement cannot override a primary artifact unless the
registry explicitly declares precedence before execution.

## 5. Transform applicability and duplicate families

The unit before arms is the condition record. Each Distill base receives the
unperturbed condition plus exactly one instance of all ten preregistered
transformation families, yielding 77 records. LLMTraceFX contributes 41
distinct family-specific records and AgentTrace contributes exactly 32
variants under their frozen family registries. No equivalent semantic condition
may be duplicated under a different family name to reach the count.

Applicability is frozen before context generation. An inapplicable transform
has a registry entry and rationale but creates no condition. If independent
validation changes the total from 150 before any Jev response exists, execution
must stop for a separately reviewed prospective amendment. Counts never change
after an outcome is visible.

The independent semantic adjudicator may share canonical types and frozen
constants but must not import or call the generator or verifier under test.

## 6. Repository-specific definitions

For AgentTrace, the evidence unit is a complete assignment bundle containing
the raw canonical session plus its declared attachments and outcomes. The
canonical loader establishes syntax and references; an independent semantic
validator establishes meaning. A tool outcome must follow and uniquely link to
its call within the registered observation window. Recovery requires a
successful linked retry and a clean terminal state. `completed` means
termination only. Missing terminal state, duplicate identifiers,
non-monotonic offsets, invalid references, partial usage, and missing usage
remain distinct. Integrity does not establish authenticity. Privacy canaries
apply to every bundle member; locally redacted operational paths are
review-only and any unredacted canary rejects.

For Distill, no final case may copy pilot source text, identifier, label,
request state, transformation receipt, or provider artifact. Each base has a
frozen per-transform applicability record. Token accounting covers the complete
rendered Arm A and Arm C contexts plus the longest atomic question under the
same bound. OS-specific ACL/capability checks are pinned in the registry;
unsupported capability is an explicit skip and never a pass. Hidden checks
remain with the label custodian. Final receipts use the separately versioned
final semantic registry while the pilot validator remains unchanged.

## 7. Arms, systems, schedule, and seeds

Approved context arms are:

- **Arm A:** frozen raw deterministic concatenation; and
- **Arm C:** Distill Lock v0 compiled context at the registered commit and
  configuration.

Arm B is prospectively omitted because no independently reviewed implementation
and full golden identity existed at this amendment freeze. Arm D remains out of
scope.

The 150 conditions produce 300 condition-arm pairs. A seeded, stratified 20%
subset of exactly 60 pairs receives replicates 2 and 3, producing exactly 420
scheduled observations per decision system. Both the deterministic baseline and
pinned Jev system consume the same schedule; only the Jev observations are
provider calls. Replicates are measurements, never retries. The runner performs
zero semantic retries and no adaptive extension.

For final-study records only, this amendment prospectively overrides the
generic schedule details in `receipt-validation/v1`. `schedule_index` is
one-based, with the first entry equal to `1`. The final `run_schedule_digest`
is the lowercase SHA-256 of
`ASCII("context-build-artifact/run-schedule/v1\n")` followed by the RFC 8785
JCS serialization of the complete schedule after NFC normalization. It is not
the raw SHA-256 of the pretty-printed schedule artifact. Pilot records retain
their existing frozen contract.

Seeds are frozen in the machine-readable registry for transformation,
condition ordering, repeatability selection, arm order, bootstrap, and
randomization tests. The final schedule and all contexts are generated and
digested before labels are available to the runner.

## 8. Thresholds and analysis

Threshold development executes before automatic selection. Safety calibration
executes only after the selected-threshold artifact is sealed. Held-out
execution is allowed exactly once only after the safety-qualification artifact
is sealed. The runner fails closed on early held-out access, label access, a
changed registry, or an absent phase receipt.

The exact one-sided Clopper-Pearson checks, ten-bin ECE, multiclass Brier score,
log loss clipped at `1e-15` with extended-real zero-probability reporting,
selective risk/coverage, consistency, held-out degradation, case-clustered
10,000-resample bootstrap, and 100,000 seeded sign-flip/randomization procedures
remain as preregistered. H1 is the sole primary family; H2-H5 retain their Holm
family, although all are non-evaluable under the independent-base minima above.

## 9. Custody, validation, and execution gates

The custodian seals each ground-truth artifact with an independently generated,
unreused 256-bit nonce. Runner packages contain only the commitment. Labels and
nonces remain unavailable until the schedule is terminal and the complete
runner receipt manifest is sealed. Any early access invalidates held-out use.

Before a provider call, exact-head code, security, privacy, methodology,
statistics, reproducibility, and operations reviews must approve the same
implementation commit and all content-addressed inputs. The implementation PR
must be merged and execution must use a clean checkout of the resulting current
`main`.

The six review roles are `methodology`, `operations`, `privacy`,
`reproducibility`, `security`, and `statistics`. Each uses a distinct Ed25519
key from a versioned reviewer-role trust root compiled into the execution
binary. The gate attestation binds all review approvals and the exact names in
the machine-readable `required_external_gates` list. The binary verifies that
its embedded VCS revision equals the reviewed commit and that the build is
clean. Missing build metadata, a dirty build, substituted/non-distinct keys, or
a mismatched trust-root version or digest fails closed.

Trust-root installation or rotation requires a prospective reviewed amendment,
new role keys, a new trust-root version and digest, and a new exact-head review.
The initial offline implementation intentionally embeds
`no-production-trust-root-v1`, an empty key map. It therefore cannot authorize
provider execution while this external-gate record is NO-GO.

The final authorization binds the exact immutable model, public pricing,
corpus, split, schedule, context, schema, policy, validator, analysis, call
count, worst-case token bound, cumulative cap, prior pilot spend, remaining
cap, expiry, reviewed AgentTrace contamination artifact, collector key, and one
canonical execution-ledger identity and path. It is local, ignored, owner-only,
and contains no credential.

The execution ledger is created atomically before authorization in an
owner-only directory. Its random identity, canonical path digest, genesis,
dataset, schedule, analysis, token bound, prior spend, and cap are bound into
the signed attestation and authorization. An independent append-only witness
binds every hash-chain advance. Missing, copied, renamed, replaced, truncated,
or reset ledgers fail closed; a scheduled call ID may be attempted once across
processes and restarts. Worst-case cost is reserved before transport and an
ambiguous post-submission failure consumes the full reservation unless
authoritative provider usage establishes a lower cost.

Labels can be released only after a collector-signed receipt manifest validates
a terminal bijection over the complete 420-entry schedule and matches the
authorized ledger. If the reviewed contamination record is absent or invalid,
AgentTrace is downgraded before generation to `confirmatory_secondary`; it is
never called held out by default.

The key remains exclusively in macOS Keychain service `ai.typesafe.api` and is
made available in memory only through `$HOME/.local/bin/typesafe-run`.

The cumulative hard cap is USD `5.000000`; prior inferred excluded-pilot spend
is USD `0.000760242`; the maximum remaining inferred-cost authorization is USD
`4.999239758`. Provider-reported spend remains a separate nullable field.
Worst-case cost is reserved before every call. There are no calls merely to
consume credits.

Public terms alone do not satisfy a mandatory provider-side numeric spend cap,
publication-independence confirmation, immutable-model guarantee, or any
preregistered account-specific retention requirement. If any mandatory item
remains unresolved after exact-current review, final authorization is not
created and the final provider call count is zero.
