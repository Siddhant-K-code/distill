# Receipt Validation and Digest Construction

**Specification:** `context-build-artifact/receipt-validation/v1`

This document freezes the validation contract for
[`pilot-schema.json`](pilot-schema.json). It defines records and a future
offline validator; it does not implement a runner, model adapter, provider
call, replay cache, or credential flow.

JSON Schema validation is necessary but not sufficient because Draft 2020-12
cannot express equality, bijection, artifact existence, or exact
cross-collection membership. Every pilot and final record must pass both the
schema and a validator implementing this specification. The validator's source
commit, executable digest, dependency lock, and configuration digest must be
published before it is used.

## 1. Canonical structured bytes

Before serialization, every JSON string must be valid Unicode normalized to
NFC. JSON values may contain only finite numbers accepted by RFC 8785; NaN and
infinities are invalid. Serialize projected JSON with the JSON Canonicalization
Scheme in [RFC 8785](https://doi.org/10.17487/RFC8785), encoded as UTF-8 with
no BOM or trailing newline.

For a structured digest with domain `D` and projected value `V`, compute:

```text
sha256(lowercase-hex) over ASCII("context-build-artifact/" + D + "/v1\n")
followed by UTF8(JCS(V))
```

The ASCII prefix is part of the preimage and provides domain separation. JSON
Pointers below use RFC 6901. Array order is significant unless this document
requires a prior sort.

Raw artifact digests are lowercase SHA-256 over exact artifact bytes with no
prefix. `receipt_schema_digest`, `*_artifact_digest`, source content hashes,
compiled-context hashes, and entries in `evidence_hashes` are raw artifact
digests unless a field description explicitly names a structured domain.

## 2. Condition identity

`identity_digest` uses domain `condition` over an object with exactly these
members in this logical projection:

```text
/schema_version
/receipt_schema_digest
/study_phase
/case
/source_set
/perturbation
/ground_truth_commitment_digest
/custody/labels_available_to_runner
/custody/commitment_published_digest
/compiled_context
/question_schema
/decision_system
/execution_authorization
/policy/policy_id
/policy/policy_version
/policy/policy_digest
/policy/threshold_set_digest
/validator
/evidence_hashes
```

Before hashing, sort `/evidence_hashes` by `artifact_id`, then `sha256`.
Condition identity intentionally excludes:

- `/record_stage`;
- `/ground_truth`;
- `/custody/access_log_digest` and `/custody/unsealed_at`;
- `/decision`;
- `/policy/result` and `/policy/reason_codes`;
- `/run_validity`;
- `/measurement`; and
- all digest fields being computed.

Thus runner and analysis records for the same pre-execution condition retain
the same identity even if model output, ground truth, timing, usage, or cost
differs. A changed source, perturbation, context, question schema, model,
policy configuration, execution authorization, or evidence artifact produces a
different identity.

## 3. Outcome and receipt integrity

`outcome_digest` uses domain `outcome` over an object containing exactly:

```text
/identity_digest
/ground_truth_commitment_digest
/ground_truth
/decision
/policy/result
/policy/reason_codes
/run_validity
```

It excludes measurement metadata. Runner and analysis records normally have
different outcome digests because held-out ground truth is sealed in runner
records.

`receipt_digest` uses domain `receipt` over the complete receipt object,
including `identity_digest`, `outcome_digest`, custody audit fields,
measurement, timestamps, usage, cost, waits, provider request ID, and errors,
but excluding `/receipt_digest` itself. It protects receipt integrity but is
not a deterministic condition identity.

## 4. Required semantic checks

The validator fails closed with a machine-readable code when any check fails.
It never repairs, coerces, reorders, retries, or silently drops data.

### 4.1 Schema and validator

1. The exact schema bytes hash to `/receipt_schema_digest`.
2. The JSON Schema engine and version equal `/validator` and run with Draft
   2020-12 format assertion enabled. URI and RFC 3339 date-time negative
   fixtures must fail.
3. The semantic-validator executable/source hashes and configuration match
   `/validator`.
4. All three structured digests recompute under Sections 1-3.

### 4.2 Case, split, sources, and perturbation

1. `manifest_order` contains every `sources[].source_id` exactly once and no
   other value. Source IDs and normalized relative paths are individually
   unique.
2. Every raw source artifact exists, has the recorded byte length and SHA-256,
   and matches the case manifest.
3. `source_set.source_set_digest`,
   `perturbation.result_source_set_digest`, and
   `compiled_context.input_source_set_digest` are equal. For perturbation
   `none`, base and result source-set digests are equal.
4. Pilot cases have role `pilot_development`; final
   `threshold_development`/`safety_calibration` roles have split `calibration`;
   `held_out` has split `held_out_repository`.
5. Repository roles, dataset snapshot, split assignment, and case manifest
   match the content-addressed pre-execution registry.
6. Perturbation type and equivalence match the frozen mapping in the schema.

### 4.3 Questions and probabilities

The exact question set is:

```text
evidence_complete
observed_tests_support
verifier_support
cleanup_complete
external_effects_resolved
patch_risk
disposition
```

`question_schema.question_ids`, valid decision outputs, and unsealed
ground-truth answers contain each ID exactly once and no unknown ID.

Allowed labels are:

| Question | Labels |
|---|---|
| `evidence_complete` | `yes`, `no`, `unknown` |
| `observed_tests_support` | `yes`, `no`, `unknown`, `not_applicable` |
| `verifier_support` | `yes`, `no`, `unknown`, `not_applicable` |
| `cleanup_complete` | `yes`, `no`, `unknown`, `not_applicable` |
| `external_effects_resolved` | `yes`, `no`, `unknown`, `not_applicable` |
| `patch_risk` | `low`, `medium`, `high`, `unknown` |
| `disposition` | `accept`, `review`, `reject` |

Ground truth may additionally use `indeterminate` under the adjudication rule;
model output may not.

For a probabilistic system, every valid output has exactly the allowed labels
as probability keys. Values are finite in `[0,1]` and sum within `1e-9` of
one. `selected_label` is a probability key. When confidence semantics are
`selected_label_probability`, confidence equals that probability within
`1e-12`. Null probability or confidence is invalid for a probabilistic system.
A deterministic policy has both values null and semantics `not_available`
unless its separately preregistered version emits genuine probabilities.

### 4.4 Decisions, policy, validity, and authorization

1. A failed decision has no outputs, has a sanitized error artifact, and
   produces policy result `review` with reason `decision_failed`.
2. Every error's `sanitized_error_artifact_id` resolves to exactly one
   `evidence_hashes` entry whose digest equals `error_detail_digest`.
3. Invalidity is orthogonal to the observed decision. An invalid run retains
   the decision/error, has at least one registered invalidity reason, and is
   excluded only under the preregistered rules.
4. A deterministic-policy record has no provider authorization, model, API, or
   SDK. A provider record has all four, and the model ID and version contain no
   moving alias. Its model snapshot digest, parameters, API, SDK package,
   confidence semantics, publication agreement, credential policy, numeric
   authorization, and provider spend cap match the preflight registry.
5. `excluded_provider_pilot` authorization is valid only for pilot data.
   `final` authorization is valid only after a successful excluded provider
   pilot and for final data.
6. Measurement has `attempt_count = 1` and `retry_count = 0`. Documented
   rate-limit waits may delay that attempt but cannot trigger resubmission.

### 4.5 Custody, artifacts, and time

1. `ground_truth_commitment_digest` equals
   `custody.commitment_published_digest`. A runner record has null ground truth
   and unseal time. An analysis record has ground truth whose digest matches
   that prepublished commitment and a non-null unseal time.
2. The access log proves no held-out label access before compiler, schema,
   policy, threshold, schedule, analysis, and receipt-manifest commitments.
3. Every referenced artifact exists in the allowlisted artifact manifest and
   matches media type, byte length, and SHA-256. No unexpected artifact is
   accepted.
4. Attempt finish is not earlier than start. Rate-limit waits fall within the
   attempt interval. Timestamps are audit data only and never enter condition
   identity.
5. Privacy scan approvals and redaction/deletion receipts match the artifact
   lifecycle policy before provider egress or publication.

## 5. Replay exclusion

`distill_lock_content_addressed_replay` is deliberately absent from receipt
schema v1. A replay experiment requires a new protocol amendment and schema
version that bind an authoritative freshness snapshot, amendment digest,
validity interval, revalidation receipt, cache key, and failure-closed stale
outcome. Matching historical hashes alone is never proof that mutable
lifecycle evidence is current.
