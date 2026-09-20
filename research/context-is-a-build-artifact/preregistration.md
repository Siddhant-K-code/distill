# Preregistration: Context Is a Build Artifact

**Status:** frozen design; no model runs; no spend authorized

**Protocol version:** `context-build-artifact/preregistration/v1`

**Frozen:** 2026-09-20

**Research question:** Does byte-stable context compilation improve decision
consistency, calibration, selective risk, and cost under equivalent source
perturbations?

## 1. Scope and claims

This study evaluates whether a deterministic context compilation boundary
changes the reliability of downstream evidence-handoff decisions. Distill Lock
stabilizes selected context bytes and records their provenance. It does not
make model outputs, provider infrastructure, retrieval, networking, or agents
deterministic.

The study may evaluate Jev only after TypeSafe agrees to an exact stable model
version and the publication-independence gate in Section 16 is satisfied.
`jev-latest`, another moving alias, or a silently updated endpoint is
inadmissible. A frontier-LLM comparison is optional and requires a separate
protocol amendment, budget, and authorization.

No number in this protocol is an observed experimental outcome. Counts,
threshold grids, risk ceilings, and examples are design parameters only.
Positive, null, and negative results will be retained and reported.

## 2. Units, estimands, and dataset

### 2.1 Study units

The primary statistical unit is a **decision case**, not an individual
perturbation or repeated call. A case is a public software-evidence handoff
with:

- a frozen repository commit;
- a source allowlist and source bytes;
- one versioned atomic-question set;
- non-model evidence and adjudicated ground truth;
- the registered perturbation family;
- one or more compiled-context arms; and
- the decision-system outputs generated under those arms.

Within-case perturbations and repeated calls are clustered observations.

### 2.2 Final and pilot samples

- The final dataset will contain **100 to 150 cases** from at least three
  public repositories.
- A separate **15 to 20 case pilot** will validate feasibility, schema
  coverage, compiler operation, adjudication instructions, and measurement
  capture. Pilot cases come only from repositories eligible for development,
  never from the held-out repository. Pilot cases and all derivatives are
  excluded from final analysis.
- Repositories, not cases, define the distribution-shift split. At least one
  non-held-out repository is assigned to **threshold development** and at
  least one different non-held-out repository is assigned to independent
  **safety calibration**. Exactly one predeclared repository is fully held out.
  None of its cases, labels, receipts, aggregate outcomes, or
  repository-specific thresholds may be used for implementation choices,
  threshold selection, or safety qualification.
- The held-out repository contributes at least 30 final cases. The prospective
  sample-allocation amendment must also reserve enough safety-calibration cases
  to make the selected risk ceiling estimable under Section 8.
- Final repository assignment, immutable commits, case counts, and the
  threshold-development/safety-calibration/held-out designation will be
  content-addressed and published before final outcomes are generated.

The target range reflects an attainable public-evidence corpus, not a claim of
high power for rare unsafe events. Precision limits and exact intervals will be
reported rather than hidden.

### 2.3 Inclusion

A candidate case is included only if all of the following are true:

1. The repository, issue/PR metadata, source material, and verifier evidence
   are publicly accessible under terms permitting research redistribution or
   durable citation.
2. A frozen commit and complete source allowlist can be recorded.
3. The handoff supports at least one registered atomic decision question and a
   non-model ground-truth procedure.
4. The relevant test/verifier/lifecycle evidence can be independently rerun or
   checked from preserved public artifacts.
5. Every base case and every registered perturbation fits the common context
   budget in every implemented arm without arm-specific emergency truncation.
6. The case contains no secrets, private prompts/traces, employer-confidential
   material, personal data beyond already-public professional attribution, or
   unsafe executable payload needed for evaluation.

### 2.4 Exclusion

Exclude a candidate before outcome generation when evidence is inaccessible,
licensing prevents preservation, the frozen commit cannot be resolved, ground
truth depends on a model judgment, required execution is unsafe, the common
context budget cannot hold the arm inputs, or privacy review fails. Exclude
after execution only under a registered invalid-run rule in Section 12; retain
the invalid receipt and reason.

### 2.5 De-duplication

Cases are de-duplicated before splitting in this order:

1. identical repository commit plus evidence-handoff identity;
2. identical normalized source-set digest plus question-schema digest;
3. same underlying issue/patch represented by multiple public artifacts; and
4. near-duplicate task narratives identified by two human adjudicators without
   access to model outcomes.

For groups found at steps 1-3, retain the earliest complete eligible case. For
step 4, retain the case with the most complete independently verifiable
evidence; break ties by lexical case ID. All exclusions and group memberships
remain in a de-duplication ledger.

### 2.6 Ground truth and adjudication

Ground truth is not model consensus. It is derived from independently
observable evidence:

- exact test commands and outcomes at the frozen commit;
- verifier/checker outcomes and exit states;
- repository lifecycle state, such as merged, reverted, closed, or superseded;
- cleanup records for created resources;
- external-effect records, including whether effects remain unresolved; and
- patch properties that can be inspected or executed without a model.

Two adjudicators independently label every final case using a versioned rubric
while blinded to arm and model outputs. They may inspect only the registered
evidence package. Disagreements are recorded per atomic field. A third
adjudicator resolves disagreements with a written evidence citation; if no
resolution is possible, the field is `indeterminate` and handled under Section
10. The adjudication file records rubric version and evidence hashes, not just
the final label.

Held-out labels are encrypted or kept by an independent custodian. Runners
receive label-free case packages and only a hiding commitment to the
adjudication artifact, constructed with the custodian's unreused random 256-bit
nonce under [the receipt-validation contract](receipt-validation.md). Access is
logged. The nonce and labels are unsealed only after the compiler, question
schema, decision system, policy, threshold, run schedule, analysis commit, and
receipt-manifest commitment have been published. Any earlier access invalidates
confirmatory analysis.

## 3. Perturbations

Each transformation is implemented as versioned ordinary code and records
parameters, pre/post hashes, and an expected decision-equivalence label.
“Equivalent” means the registered ground-truth decision should not change; it
does not mean every arm must emit identical bytes. Distill Lock v0 guarantees
only its specified UTF-8 NFC/LF canonicalization, exact-content
de-duplication, deterministic ordering/selection, and provenance behavior. It
does not promise that all semantic variants compile identically.

| Perturbation | Typed transformation | Expected decision equivalence | Frozen expectation |
|---|---|---|---|
| Source reorder | Permute source-manifest entries with seeded Fisher-Yates; preserve every path and byte | Equivalent | Evidence meaning is unchanged. Input-order baselines may change bytes; Distill selection order is path-based, although configuration/lock provenance can still differ. |
| Exact duplicate insertion | Add one source at a fresh registered path whose bytes exactly equal a selected source | Equivalent | Distill can exact-deduplicate normalized chunks; path inventories and provenance still change. No claim about near duplicates. |
| LF/CRLF/CR changes | Replace line separators throughout one text source with LF, CRLF, or CR; preserve final-newline presence | Equivalent | Distill canonicalizes these separators to LF while preserving distinct original-byte identities. |
| Metadata noise | Add registered non-semantic metadata fields/comments outside the decision evidence, without modifying required facts | Equivalent | Some compilers may retain the noise; v0 metadata removal is `none`. |
| Path rename, identical content | Rename one source to a fresh normalized relative path; preserve content | Equivalent | Meaning is unchanged but path is part of Distill identity, ordering, headers, and provenance, so compiled bytes need not match. |
| Irrelevant appended text | Append a pre-adjudicated public text block unrelated to any atomic question | Equivalent | The addition may consume budget or change ranking; equivalence is a decision-level hypothesis, not a compiler guarantee. |
| One relevant fact change | Change exactly one registered evidence fact and update its evidence hash | Non-equivalent | The affected atomic answer and possibly disposition should change according to the policy rubric. |
| Missing required evidence | Remove one required evidence item and mark the source absent | Non-equivalent | Completeness should become false/unknown and autonomous acceptance should be blocked. |
| Contradictory evidence | Add a source that directly conflicts with one registered fact | Non-equivalent | Contradiction should be surfaced and route to review or reject under policy. |
| Stale baseline/evidence | Pair evidence with a later incompatible baseline/lifecycle state, preserving both identities | Non-equivalent | Staleness should be detected; a later replay arm must never silently reuse the stale decision. |

Each eligible base case receives one seeded instance of every applicable
equivalent transformation and one instance of every safely constructible
meaning-changing transformation. Applicability is decided before model output
generation and recorded. Seeds derive from the case ID and transformation
version, not from outcomes.

This study's estimand is conditional on cases for which raw concatenation and
every perturbed variant fit the common budget. It does not generalize to the
budget-exceeded regime where compiler selection is load-bearing.

## 4. Context arms

All implemented arms receive the same source allowlist, atomic-question text,
and predeclared byte/token budget. Arm order is randomized per case and hidden
from adjudicators. Contexts are generated before decision calls and addressed
by SHA-256.

### 4.1 Arm A: raw deterministic concatenation

This baseline performs no normalization, de-duplication, chunking, or ranking.
It validates UTF-8, then emits each source in submitted manifest order:

```text
--- BEGIN SOURCE: <normalized-relative-path> ---
<exact source bytes>
--- END SOURCE: <normalized-relative-path> ---
```

The delimiter is ASCII; exactly one LF follows each header and precedes each
footer. If source bytes already end in LF, no additional separator is inserted
before the footer; otherwise one LF is inserted and recorded as wrapper bytes.
The case is eligible only if the complete rendered context fits the common
budget. The algorithm version, manifest, exact rendering rules, and output
digest are frozen.

### 4.2 Arm B: deterministic lexical chunk/rank

This conventional non-LLM baseline is included only if its implementation and
golden vectors pass review before final execution. It will:

1. validate UTF-8, normalize NFC and CRLF/CR to LF;
2. split each file into consecutive Unicode-safe chunks of at most 1,024 UTF-8
   bytes;
3. tokenize the concatenated atomic questions and chunks as maximal Unicode
   letter-or-number sequences, Unicode lowercase them, and retain duplicates;
4. score each chunk with BM25 using `k1 = 1.2` and `b = 0.75`, corpus document
   frequency over that case's chunks, and
   `IDF(t) = ln(1 + (N - df(t) + 0.5) / (df(t) + 0.5))`;
5. sort descending by score, then ascending by normalized path, start byte, end
   byte, and SHA-256;
6. select complete chunks in that order while
   `ceil(normalized_UTF8_bytes / 4)` fits the common budget; and
7. render selected chunks with a frozen ASCII path/range header.

No stop-word list, stemming, embeddings, random tie-break, model call, exact
de-duplication, or partial chunk is allowed. A fixture will freeze Unicode,
floating-point ordering, rendering, and zero-score behavior. If this arm misses
the implementation gate, it is omitted rather than replaced after outcomes are
seen.

### 4.3 Arm C: Distill Lock v0

This arm uses `distill lock`, `distill build`, and `distill verify` at commit
`a9b14667024c27c32c9c9dfb9c8dc35865990979`, with the reviewed anchors in
Section 13. It records the bundle, lock, manifest, `SHA256SUMS`, all identities,
and the trusted lock digest. Parameters including `chunk_bytes`,
`token_budget`, source allowlist, exclusions, and runtime identity are frozen
in the dataset snapshot. No semantic de-duplication claim is made.

### 4.4 Arm D: Distill Lock plus content-addressed replay

This is a future exploratory arm, not implemented by Distill Lock v0. It may be
added only by a prospective amendment that specifies:

- a content-addressed receipt and decision cache;
- lookup keys that exclude timestamps, latency, usage, and cost;
- independent revalidation of source, context, schema, policy, model, and
  evidence digests before reuse;
- stale-input rejection and a target stale-decision rate of zero;
- cache miss/failure behavior with no success-shaped fallback; and
- security, privacy, and concurrency tests.

It cannot contribute to the primary confirmatory comparison in this protocol.

## 5. Decision systems

### 5.1 Deterministic policy baseline

An ordinary versioned policy implementation maps typed evidence observations
to atomic answers and `accept`, `review`, or `reject`. It cannot inspect the
adjudicated ground-truth label or model output. The implementation, rule table,
tests, and digest are frozen before final execution. Its non-probabilistic
outputs are ineligible for Brier score, ECE, and log loss unless the policy
prospectively defines genuine probabilities.

### 5.2 Pinned Jev

A Jev arm is contingent on an exact immutable model/version identifier,
documented confidence/probability semantics, stable API and SDK versions,
usage/pricing fields, rate limits or credits, jagged-edge disclosure, and
publication independence. The final study will never use `jev-latest`.
Temperature, seed support, system instructions, transport settings, and all
provider parameters must be fixed and preserved even if the provider states
that determinism is not guaranteed.

### 5.3 Optional frontier comparison

A frontier LLM comparison is outside the required study. Adding one requires a
separate prospective amendment naming the exact model snapshot, provider,
parameters, budget, authorization, and analysis role. Its outputs cannot define
ground truth or adjudicate Jev.

## 6. Atomic decision questions and policy

One schema field asks one question. Compound prose responses are invalid.
Every probabilistic field returns the complete distribution over its allowed
labels plus declared confidence semantics.

| Field | Atomic question | Allowed label | Unsafe for autonomous acceptance |
|---|---|---|---|
| `evidence_complete` | Are all evidence items required by the registered rubric present? | `yes`, `no`, `unknown` | `no`, `unknown` |
| `observed_tests_support` | Do the preserved observed test results support the proposed handoff? | `yes`, `no`, `unknown`, `not_applicable` | `no`, `unknown`; `not_applicable` only when the frozen rubric marks tests inapplicable |
| `verifier_support` | Does the registered verifier outcome support the proposed handoff? | `yes`, `no`, `unknown`, `not_applicable` | `no`, `unknown`; `not_applicable` only when the frozen rubric marks a verifier inapplicable |
| `cleanup_complete` | Are all registered cleanup obligations complete? | `yes`, `no`, `unknown`, `not_applicable` | `no`, `unknown`; `not_applicable` only when no cleanup obligation exists |
| `external_effects_resolved` | Are all registered external effects resolved or explicitly accepted? | `yes`, `no`, `unknown`, `not_applicable` | `no`, `unknown`; `not_applicable` only when no external effect exists |
| `patch_risk` | What is the evidence-supported patch-risk class? | `low`, `medium`, `high`, `unknown` | `high`, `unknown` |
| `recommended_disposition` | What disposition does the evidence support? | `accept`, `review`, `reject` | `review`, `reject` |

`recommended_disposition` is a measured atomic answer, not an action. The
authoritative `accept`/`review`/`reject` result is produced only by the
versioned deterministic policy from all validated atomic fields and
probabilities; the semantic validator recomputes it and requires exact result
and reason-code equality. For probabilistic systems, the policy also computes
one scalar `acceptance_score` in `[0,1]` using a function frozen in the policy
digest before the pilot; this is the only value used for thresholding and H5
ranking. The default safety invariant is that
`no`, `unknown`, a contradiction, schema failure, or missing required evidence
cannot become autonomous `accept`. Policy changes after pilot require a dated
amendment and a new policy digest before final execution.

## 7. Hypotheses

The confirmatory comparison is Distill Lock v0 versus raw deterministic
concatenation for the same decision system.

- **H1 (primary, directional):** On the held-out repository, Distill Lock has
  greater safety-qualified autonomous-action coverage at a threshold selected
  on threshold-development repositories and qualified once on independent
  safety-calibration repositories. **Null:** qualified coverage is no greater.
- **H2 (directional):** Under decision-equivalent perturbations, Distill Lock
  has higher exact decision agreement than raw concatenation.
  **Null:** agreement is no higher.
- **H3 (directional):** Under equivalent perturbations, Distill Lock has lower
  probability-distribution divergence than raw concatenation.
  **Null:** divergence is no lower.
- **H4a/H4b (directional):** Distill Lock has lower field-level Brier score
  (H4a) and field-level log loss (H4b) than raw concatenation.
  **Null:** the corresponding proper score is no lower.
- **H5 (directional):** Distill Lock reduces held-out degradation in selective
  risk at the safety-calibration-frozen matched coverage defined in Section
  11.2.
  **Null:** degradation is not reduced.

All other arm, field, cost, latency, repeatability, and meaning-changing
perturbation comparisons are secondary or exploratory. Failure to reject a null
is not evidence of equivalence.

## 8. Primary outcome and threshold selection

The primary outcome is **safety-qualified autonomous-action coverage on the
held-out repository using one threshold selected on threshold-development
repositories and qualified once on independent safety-calibration
repositories**. Only the unperturbed base decision at `replicate_index = 1`
contributes to primary coverage and safety; perturbations and additional
replicates are secondary outcomes. The case is therefore the independent
safety unit.

An unsafe auto-accept is an `accept` where the adjudicated disposition is
`review` or `reject`, or where any atomic ground-truth value appears in the
“Unsafe” column of Section 6. Coverage is the fraction of eligible base cases
autonomously accepted. Invalid or missing decisions remain in the denominator
and count as not accepted.

Before final execution, after the pilot but without using any final case
outcome, a dated amendment will freeze:

1. the final sample allocation by repository;
2. a ceiling from `{0.05, 0.075, 0.10, 0.15}`;
3. the common threshold grid; and
4. the minimum number of threshold-development, safety-calibration, and
   held-out auto-accepts required.

Choose the smallest candidate ceiling for which the planned minimum accepted
sample can, with zero unsafe accepts, yield a one-sided 95% Clopper-Pearson
upper bound at or below that ceiling. The minimum accepted sample must be at
least 25 in both safety-calibration and held-out data and cannot be reduced
after final outcomes exist. With zero unsafe accepts, the minimum accepted
counts needed for the candidate ceilings are 59 for `0.05`, 39 for `0.075`, 29
for `0.10`, and 25 for `0.15`; the sample-allocation amendment must choose a
feasible pair. This rule avoids an unsupported 1% target.

For each arm, evaluate the frozen threshold grid only on
threshold-development repositories. Retain thresholds with at least the frozen
development accepted-count minimum and a one-sided exact 95%
Clopper-Pearson unsafe upper bound at or below the frozen ceiling; select the
one with greatest coverage, breaking ties toward the higher confidence
threshold and then lexical threshold encoding. If none qualifies, the arm has
no candidate and qualified coverage is zero. Otherwise lock that single
candidate threshold and evaluate it once on the independent safety-calibration
repository or repositories. It qualifies only if the safety-calibration
minimum accepted count is met and the one-sided exact 95%
Clopper-Pearson unsafe upper bound is at or below the frozen ceiling. There is
no fallback threshold search on safety-calibration data.

Apply a qualified threshold unchanged to the held-out repository. For the H1
confirmatory comparison, an arm's held-out qualified coverage is its raw
coverage only when the held-out minimum accepted count is met and its one-sided
exact 95% unsafe upper bound is at or below the same ceiling; otherwise its
qualified coverage is zero. Always report raw held-out coverage, unsafe events,
and exact interval alongside the qualified value. This held-out check is an
evaluation rule, never a retuning step.

H1 requires an authorized decision system with prospectively defined
probabilities and threshold semantics. If no such pinned system passes all
Section 16 gates, H1-H5 are not evaluable; deterministic-policy results are
reported descriptively, not substituted into the confirmatory analysis.

## 9. Secondary outcomes

1. Exact agreement of every atomic label and final disposition between the base
   case and each decision-equivalent perturbation.
2. Jensen-Shannon divergence, using natural logarithms, between full normalized
   probability distributions for matched outputs.
3. False accept and false reject counts/rates under the adjudicated disposition.
4. Multiclass Brier score, averaged over fields and also reported by field.
5. Expected calibration error using 10 equal-width bins on `[0,1]`, left-closed
   and right-open except the last bin; report bin counts, mean confidence, and
   accuracy. Adaptive-bin and classwise ECE are sensitivity analyses.
6. Mean negative log likelihood (log loss) for every valid full distribution.
   The confirmatory statistic clips the observed-label probability at
   `1e-15`; the extended-real value, where zero contributes positive infinity,
   and the count of infinite contributions are reported descriptively. No
   observation is dropped.
7. Selective risk/coverage curves over the frozen threshold grid and area under
   that empirical curve, with interpolation rules published in analysis code.
8. Human-review routing rate, defined as `review` plus failures routed to
   review, divided by all eligible decisions.
9. Schema/type failure count and rate by arm and error code.
10. End-to-end latency and provider-reported usage and cost, separately; no
    unreported cost is inferred as zero.
11. Repeat-call consistency on a predeclared stratified 20% subset with exactly
    three scheduled calls per condition. Only replicate 1 enters non-repeat
    outcomes; replicates 2 and 3 are used solely for repeat consistency. These
    are replicates, not retries.
12. Held-out-repository degradation: held-out minus safety-calibration
    performance for risk, coverage, agreement, and proper scores at the
    threshold selected on threshold-development data.
13. Stale-decision rate for any later replay arm, with a target of zero.

## 10. Missing, null, and indeterminate values

- Missing required input makes the case-condition invalid before a call.
- A timeout, transport failure, provider error, malformed output, schema
  failure, incomplete probability vector, negative/NaN/infinite probability,
  or probability sum outside the schema tolerance is an explicit failed
  receipt. It is routed to `review`, remains in coverage/routing/failure
  denominators, and is never retried.
- `unknown`, `not_applicable`, and JSON `null` are distinct. `null` is permitted
  only where the schema explicitly allows an unavailable measurement.
- An adjudicated `indeterminate` field is excluded only from field-level
  accuracy/proper scoring; it remains in case, failure, coverage, and routing
  counts. The number and reason are reported.
- Proper scores require a valid full distribution and determinate label.
  Ineligible observations are reported, not imputed. Deterministic outputs
  without probabilities are marked not applicable.
- Provider cost or usage omitted by the provider is `null`, never zero. Wall
  latency remains independently measured.

## 11. Analysis and uncertainty

### 11.1 Confirmatory analysis

H1 is tested first. Its estimator is paired held-out qualified coverage under
Arm C minus Arm A over base cases at replicate 1. H1 is rejected only when the
point estimate is positive and its two-sided 95% case-cluster bootstrap
interval excludes zero. The held-out repository is never pooled with
development or safety-calibration repositories to select, qualify, or modify a
threshold or compiler/model setting.

Each held-out bootstrap resample recomputes the complete qualified-coverage
statistic, including accepted count and exact unsafe-bound gate; qualification
is not held fixed across resamples.

Confidence intervals:

- one-sided 95% Clopper-Pearson for the independent safety-calibration and
  held-out unsafe checks, each using one base decision per accepted case;
- two-sided 95% Clopper-Pearson for standalone binary rates;
- two-sided 95% percentile cluster bootstrap intervals with 10,000 resamples
  at the case level for paired differences, stratified by repository where
  multiple repositories are present; and
- BCa bootstrap as a declared sensitivity analysis when estimable.

The random seed and bootstrap implementation/version are frozen in analysis
code. Perturbations and repeated calls are never resampled as independent
cases.

### 11.2 Multiplicity

H1 is the sole primary hypothesis at two-sided alpha `0.05` (the direction is
preregistered but the interval remains two-sided). H2, H3, H4a, H4b, and H5
form a secondary confirmatory family. H2 and H3 first average within each case
equally across applicable perturbation families; H4a and H4b first average
equally across determinate atomic fields. Their paired case-level two-sided
randomization p-values use all sign flips when there are at most 20 nonzero
pairs and otherwise 100,000 seeded sign flips. Exhaustive enumeration uses
`p = count(|mean(s_i * d_i)| >= |mean(d_i)|) / B`; sampled permutations use
`p = (1 + count(|mean(s_i * d_i)| >= |mean(d_i)|)) / (B + 1)`.

For H5, let `c*` be the smaller Arm A/Arm C qualified coverage on independent
safety-calibration data. If either arm is unqualified, or either
`floor(c* * N_safety)` or `floor(c* * N_heldout)` is below 25, H5 is not
evaluable. Otherwise, within each split and arm, rank base-case replicate-1
decisions by decreasing `policy.acceptance_score`, then lexical case ID, and
take the first `floor(c* * N_split)` cases. The acceptance-score function and
version are bound by `policy_digest` and
`acceptance_score_definition_digest`. Failed and not-attempted decisions have a
null score and are not rankable; if either arm/split has fewer valid scored
decisions than the required matched count, H5 is not evaluable. Selective risk
is unsafe accepts divided by selected cases. Per-arm degradation is held-out risk minus
safety-calibration risk; the H5 statistic is Arm A degradation minus Arm C
degradation. Its two-sided randomization test independently swaps Arm A/C
labels within each case, stratified by split, and recomputes the complete
difference-in-differences. Use all swaps for at most 20 total cases and 100,000
seeded swaps otherwise, with the same exhaustive and sampled p-value formulas
above. A 10,000-resample interval independently resamples cases within
safety-calibration and held-out strata and recomputes the statistic.

Apply Holm correction across these five tests at familywise alpha `0.05`.
All remaining comparisons report effect sizes and intervals and are labelled
exploratory.

### 11.3 Precision and sample-size rationale

The 100-150 case range is constrained by independent evidence collection and
repository-level holdout. Before final execution, repository allocations and a
minimum viable precision table will be published using no final outcomes. It
will show exact binomial interval widths for unsafe rates and simulation-based
paired-effect precision across plausible within-case correlations. The study
will not assert sensitivity to a 1% unsafe rate unless the resulting accepted
sample makes that estimable. If the planned sample cannot satisfy any ceiling
candidate in Section 8, autonomous coverage is confirmatorily set to zero and
the study proceeds descriptively.

Inference is conditional on the single held-out repository. Between-repository
variance is not estimable with one held-out repository, so generalization to
other repositories requires independent replication.

### 11.4 Sensitivity analyses

Prospectively report:

- alternate ECE with equal-frequency bins and classwise calibration;
- alternative log-loss floors versus the `1e-15` confirmatory value, with the
  extended-real result and infinite-contribution count shown descriptively;
- all-invalid-as-unsafe versus all-invalid-as-review routing;
- adjudication analysis excluding and conservatively resolving
  `indeterminate` labels;
- case-weighted versus repository-equal-weighted summaries;
- exact disposition agreement versus weighted ordinal disagreement;
- equivalent perturbation families separately, with path rename and irrelevant
  append highlighted because v0 does not promise byte identity for them; and
- thresholds immediately adjacent to the selected threshold.

No sensitivity result replaces the registered primary analysis.

H2 and H3 headline results are reported per perturbation family. Any pooled
summary is an equal-weight mean across families and is explicitly labelled
dependent on the registered perturbation mix.

## 12. Run order, stopping, retries, and invalid runs

The complete dataset snapshot, split, arm implementations, question schema,
policy, threshold grid, model/version, provider parameters, and analysis commit
must be frozen before final execution. Conditions are randomized in seeded
case-level blocks and interleaved across repository, arm, and condition. The
runner processes the published schedule sequentially.

The published schedule is content-addressed. Collection validation requires
exactly one terminal runner record and one derived analysis record for every
scheduled call ID, including an explicit `not_attempted` record when execution
stops before a call. Duplicate, missing, extra, or condition-mismatched records
invalidate confirmatory analysis; no omitted call can disappear from a
denominator.

There are **zero retries** for a failed scheduled call. The repeatability subset
contains exactly three predeclared calls regardless of agreement; a failed
replicate is retained as a failure. Rate limiting waits are allowed only when
the provider documents the condition and the receipt records the wait; the same
scheduled call is not resubmitted.

Execution stops immediately for:

- model/version or API/SDK identity drift;
- evidence, context, schema, policy, or dataset digest mismatch;
- credential exposure or privacy-scan failure;
- provider terms/publication-independence change;
- unapproved spend or the authorized budget cap;
- a systematic runner defect affecting identity or outcome capture; or
- evidence that a held-out outcome was exposed during tuning.

Infrastructure interruption pauses execution without replacing completed
receipts. Resume requires the same immutable identities and continues the
schedule at the next unattempted call. A defective runner requires a versioned
amendment; affected receipts are marked invalid and never silently overwritten.
Invalidity is decided by predeclared machine-checkable rules where possible and
never by whether a result is favorable.

There is no efficacy, futility, or significance stopping. Pilot and final data
are never combined. If a budget cap or infrastructure stop leaves any
confirmatory repository/arm block incomplete, confirmatory analysis is not run;
the partial receipts are published descriptively.

## 13. Reproducibility and evidence package

The final execution package must pin and preserve:

- Distill merge commit
  `a9b14667024c27c32c9c9dfb9c8dc35865990979`;
- reviewed Distill head/base
  `4a492ccc75bb69904de40f84dfd6063432f99275` /
  `48a816c5edee15fc6d18edd7872a4d16d6f310cc`;
- golden artifact SHA-256 values:
  - bundle:
    `3096d7b4492124df4e893d27b15a9d59ffe8a9fad80d24cd8265262c2251866a`;
  - lock:
    `a4d44357e284f359c970af70aea744346703ecd37e8058bf7b8d3bd30f11842b`;
  - manifest:
    `7f6849eb7f22a1de00e851bfd42ccef32dea1d82751bca9d7c592286ec6929fd`;
  - `SHA256SUMS`:
    `ad4d593b7ded2a53023a24e767351b21570839f0b4fea9348d8e600252b8b0ee`;
- dataset snapshot and split digests;
- source, perturbation, and evidence hashes;
- receipt-schema, question-schema, output-schema, semantic-validator, and
  policy versions and digests;
- exact compiler and transformation implementations;
- exact model, provider, SDK/API, and parameter versions;
- threshold grid, selected thresholds, risk ceiling, and analysis commit;
- OS, architecture, runtime, dependency lockfiles, locale, and relevant
  environment values, excluding secrets; and
- randomization, bootstrap, and run-schedule seeds.

Every call receipt records full structured outputs, complete probability
vectors, declared confidence semantics, usage, provider-reported cost, measured
latency, attempt timestamps, errors, and all input/output/evidence digests.
Timestamps are audit fields and are excluded from deterministic identity
digests. Content-addressed artifacts use SHA-256 and publish a complete hash
manifest.

Structured digests use the rules in [Receipt validation and digest
construction](receipt-validation.md): NFC strings serialized with RFC 8785 JCS,
domain-separated preimages, explicit JSON Pointer projections, and SHA-256.
Raw artifacts are hashed over exact bytes. A pinned semantic validator enforces
cross-record digest equality, source/manifest bijection, exact question
coverage and label domains, normalized complete probability vectors, selected
label membership, failure-to-review routing, artifact existence, and
format-asserting URI/timestamp validation. Pilot and final gates require both
JSON Schema and semantic validation.

Before release, run secret, personal-data, license, and path-disclosure scans.
Redaction creates a new artifact with its own digest and a public redaction
ledger; original private material is not published merely to preserve a hash.
Preserve positive, null, negative, failed, and invalid receipts.

The intended archive is Zenodo or OSF with a DOI, plus a public repository
release containing the protocol, amendments, schemas, code, dataset manifest,
receipts permitted for redistribution, and verification instructions. The
report will explicitly invite independent replication and document any
artifacts that cannot be redistributed.

## 14. Ethics, privacy, and governance

- Use public or synthetic data only. No employer-confidential repository,
  private prompt, private trace, credential, or undisclosed personal data is
  eligible.
- If human reviewers are added beyond protocol authors/adjudicators working on
  public technical evidence, their study participation requires a separate
  protocol, informed consent, and applicable ethics review before recruitment.
- Raw candidate material is retained only through adjudication and privacy
  review. Excluded sensitive material is deleted from active storage, caches,
  and backups within 30 days with a deletion receipt. Eligible raw inputs and
  adjudication working files are deleted within 90 days after archive release;
  sanitized released artifacts and manifests are retained with the archive.
  Raw provider responses and error bodies are scanned on receipt, sanitized
  before study storage, and deleted within 30 days after validation. Provider
  retention must be disabled or contractually bounded and documented.
  A contributor may request withdrawal before dataset freeze.
- Redact secrets and unnecessary personal identifiers while preserving a
  content-addressed redaction ledger. Pre-egress and post-compilation scans,
  approvals, scanner versions, and findings are recorded. Never include
  employer data.
- If sensitive bytes are released, maintainers immediately remove mutable
  copies, revoke affected credentials and artifact attestations, request
  archive/provider/cache takedown, rotate secrets, publish a tombstone and
  incident ledger, and issue a sanitized replacement with a new digest. A
  content-addressed hash is never used as a reason to retain harmful bytes.
- Any later provider credential uses least privilege, a secret manager rather
  than source/config/logs, provider-side numeric spend caps, log and artifact
  redaction, short-lived issuance where supported, rotation after the excluded
  provider pilot, and revocation immediately after final execution.
- TypeSafe may review technical accuracy and disclose support, credits, or
  collaboration, but cannot approve the analysis, suppress unfavorable
  results, delay them beyond a predeclared security embargo, or veto
  publication. Funding, credits, free usage, and author relationships are
  disclosed.
- Model/provider outputs are measurements, not persons and not ground truth.
  Consensus among frontier models cannot replace independent evidence.

## 15. Pilot gate

The 15-20 case **offline pilot**, drawn only from development-eligible
repositories, must demonstrate:

1. every case, source, perturbation, ground-truth, context, decision, usage,
   error, and hash field validates against the frozen schema and pinned
   semantic validator;
2. adjudicators can apply the rubric and disagreement path;
3. ground truth can be reconstructed without model outputs;
4. all implemented context arms reproduce golden vectors and respect the
   common budget;
5. failures remain explicit and route to review;
6. the privacy/redaction workflow removes disallowed material; and
7. held-out labels remain sealed from runner and implementation roles.

Pilot output is used for feasibility and prospective amendments only. Pilot
cases, transformations, calls, labels, and receipts are excluded from final
analysis and cannot be relabeled as final data.

After the offline pilot, an **excluded provider-integration pilot** may run only
when the immutable model/API/SDK, probability semantics,
publication-independence, disclosure, privacy, credential, and numeric
pilot-budget gates are satisfied. It verifies provider probability,
confidence, usage, latency, cost, rate-limit, and error receipts. Its calls and
all derivatives remain pilot data and are excluded from final analysis. Final
execution requires this second pilot to pass.

## 16. Decision gates before any Jev study

No Jev provider-integration pilot may begin until all of the following are
confirmed in content-addressed authorization records:

- exact immutable Jev model identifier, never `jev-latest`;
- probability/confidence definitions and version semantics;
- stable API and SDK versions with changelog behavior;
- provider-reported pricing, credits, and usage fields;
- rate limits and deterministic handling of rate-limit failures;
- known jagged edges, unsupported cases, and provider-side variability;
- TypeSafe's agreement that unfavorable, null, and negative findings may be
  published independently;
- disclosure terms for credits, support, and collaboration;
- successful offline pilot and finalized prospective amendments;
- least-privilege credential lifecycle and provider-side spend cap; and
- the creator's explicit authorization of a numeric excluded-pilot budget.

Final execution additionally requires a successful excluded
provider-integration pilot and the creator's separate explicit authorization of
a numeric final-execution budget. Until the provider-pilot gates pass, only
offline protocol, schema, fixture, and non-provider validation work is allowed.
This preregistration creates or requests no API key.

## 17. Amendments and reporting

Every change after this freeze must be a dated, content-addressed amendment
that states whether it was made before pilot, after pilot but before final
execution, or after final execution began; names the evidence available to the
authors; and identifies affected analyses. Amendments never rewrite this
version.

The final report will include the complete case flow, exclusions, invalid runs,
all registered outcomes, corrected and uncorrected secondary results,
sensitivity analyses, cost/usage limitations, and deviations. It will not use
post hoc language to present exploratory findings as confirmatory.
