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

A projection is reconstructed as a nested JSON object with the original member
names and hierarchy; unlisted members are dropped. Pointers never become flat
string keys. For example, selecting `/custody/labels_available_to_runner`
produces `{"custody":{"labels_available_to_runner":false}}`, not
`{"/custody/labels_available_to_runner":false}`.

Known-answer vectors for the domain construction are:

| Domain | JCS value | Expected SHA-256 |
|---|---|---|
| `condition` | `{"a":"NFC","z":1}` | `8a4cf865911e197bdd0e84893e746844c4affd7d10df25216dab7a26afc3906b` |
| `outcome` | `{"identity_digest":"0000000000000000000000000000000000000000000000000000000000000000","result":"review"}` | `4b791030da3696eed600d0dfa282dabb9a9184394ba68a5467fc0b271633dfef` |
| `receipt` | `{"attempt_count":1,"timestamp":"2026-09-20T00:00:00Z"}` | `56f80512bdbd5f9fb99d7fc55c17134752ea1c624f7ebf399885178e7dbcbf0d` |

Raw artifact digests are lowercase SHA-256 over exact artifact bytes with no
prefix. `receipt_schema_digest`, `*_artifact_digest`, source content hashes,
compiled-context hashes, and entries in `evidence_hashes` are raw artifact
digests unless a field description explicitly names a structured domain.

`source_set_digest` uses structured domain `source-set` over exactly
`source_set_id`, `manifest_order`, and `sources`. `manifest_order` remains in
declared order. Before hashing, sort `sources` by `source_id`; each source
projection contains exactly `source_id`, `relative_path`, `media_type`,
`byte_length`, `content_digest`, `normalized_content_digest`, and
`evidence_role`.

`transformation_receipt_digest` uses structured domain `transformation` over
exactly `case_id`, `perturbation_id`, `type`, `transform_version`, `seed`,
`expected_equivalence`, `base_source_set_digest`,
`result_source_set_digest`, and an ordered `operations` array. Each operation
uses a transformation-version-specific schema frozen before the pilot. The
transformation receipt is a pre-execution evidence artifact whose raw-byte
identity is resolved through `transformation_receipt_artifact_id`; the matching
`evidence_hashes` entry carries its independent raw-byte digest.

## 2. Condition identity

`identity_digest` uses domain `condition` over an object with exactly these
members in this logical projection:

```text
/schema_version
/receipt_schema_digest
/study_phase
/run_schedule_digest
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
/policy/acceptance_score_definition_digest
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
- `/policy/acceptance_score`;
- `/run_validity`;
- `/receipt_artifact_hashes`;
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

1. The exact schema bytes hash to `/receipt_schema_digest` and to the published
   constant
   `adfe846637344f202e5b73f3c082c6d485663120cefc40ca4d9c65a6a2a4c9a3`.
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
3. Recompute `source_set.source_set_digest` under Section 1.
   `perturbation.base_source_set_digest` must resolve uniquely to the
   registered unperturbed source set for the same case in the frozen dataset
   snapshot. Recompute the transformation receipt and require it to bind the
   case, perturbation ID, type, version, seed, equivalence, base digest, result
   digest, and ordered operations. Its artifact ID must resolve to exactly one
   pre-execution `evidence_hashes` entry, and that entry's raw-byte digest must
   match the preserved artifact.
4. `source_set.source_set_digest`,
   `perturbation.result_source_set_digest`, and
   `compiled_context.input_source_set_digest` are equal. For perturbation
   `none`, base and result source-set digests are equal.
5. Pilot cases have role `pilot_development`; final
   `threshold_development`/`safety_calibration` roles have split `calibration`;
   `held_out` has split `held_out_repository`.
6. Repository roles, dataset snapshot, split assignment, and case manifest
   match the content-addressed pre-execution registry.
7. Perturbation type and equivalence match the frozen mapping in the schema.

### 4.3 Questions and probabilities

The exact question set is:

```text
evidence_complete
observed_tests_support
verifier_support
cleanup_complete
external_effects_resolved
patch_risk
recommended_disposition
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
| `recommended_disposition` | `accept`, `review`, `reject` |

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

1. `recommended_disposition` is a model/policy-baseline measurement only; it
   never authorizes action. For every valid decision, execute the exact pinned
   deterministic policy over
   the validated atomic outputs and frozen threshold set. The recomputed result
   complete ordered reason-code set, and scalar acceptance score must exactly
   equal `/policy/result`, `/policy/reason_codes`, and
   `/policy/acceptance_score`; an unsafe or unknown required field cannot be
   paired with `accept`. The score function/version is bound by
   `policy_digest` and `acceptance_score_definition_digest`.
2. A failed or not-attempted decision has no outputs, has a sanitized error
   artifact, and produces policy result `review`. Failed attempts use reason
   `decision_failed`; calls stopped before execution use `not_attempted` and
   error stage `scheduler`.
3. Every error's `sanitized_error_artifact_id` resolves to exactly one
   `receipt_artifact_hashes` entry whose digest equals
   `error_detail_digest`.
4. Invalidity is orthogonal to the observed decision. An invalid run retains
   the decision/error, has at least one registered invalidity reason, and is
   excluded only under the preregistered rules.
5. A deterministic-policy record has no provider authorization, model, API, or
   SDK. A provider record has all four. A moving alias is any
   case-insensitive `latest`, `stable`, `preview`, or `newest` token delimited
   by the start/end of the value or `-`, `_`, `/`, `:`, `@`, or `.`; model IDs
   and versions containing one are invalid. Its model snapshot digest,
   parameters, API, SDK package,
   confidence semantics, publication agreement, credential policy, numeric
   authorization, and provider spend cap match the preflight registry.
6. `excluded_provider_pilot` authorization is valid only for pilot data.
   `final` authorization is valid only after a successful excluded provider
   pilot and for final data.
7. An attempted measurement has `attempt_count = 1`, non-null start/finish
   timestamps and latency; an explicit `not_attempted` terminal record has
   `attempt_count = 0`, null start/finish/latency/usage/cost/request ID, and no
   wait events. Every measurement has `retry_count = 0`. Documented rate-limit
   waits may delay an attempt but cannot trigger resubmission.

### 4.5 Custody, artifacts, and time

1. The custodian generates a cryptographically random 256-bit nonce for each
   ground-truth artifact and computes
   `SHA256(ASCII("context-build-artifact/ground-truth-commitment/v1\n") ||
   nonce || UTF8(JCS(ground_truth)))`. The nonce is never reused. The resulting
   lowercase digest is both `ground_truth_commitment_digest` and
   `custody.commitment_published_digest`. A runner record has null ground truth,
   null reveal nonce, and null unseal time. An analysis record reveals the
   32-byte nonce as 64 lowercase hexadecimal characters, includes ground truth,
   recomputes the commitment, and has a non-null unseal time. The nonce remains
   under independent custodian control until the commitments named in the
   preregistration are published.
2. The access log proves no held-out label access before compiler, schema,
   policy, threshold, schedule, analysis, and receipt-manifest commitments.
3. Every referenced artifact exists in the allowlisted artifact manifest and
   matches media type, byte length, and SHA-256. `evidence_hashes` contains only
   pre-execution evidence; `receipt_artifact_hashes` contains outcome-dependent
   sanitized responses, errors, usage, and audits. No unexpected artifact is
   accepted.
4. Attempt finish is not earlier than start. Rate-limit waits fall within the
   attempt interval. Timestamps are audit data only and never enter condition
   identity.
5. Privacy scan approvals and redaction/deletion receipts match the artifact
   lifecycle policy before provider egress or publication.

## 5. Collection and schedule validation

`run_schedule_digest` is the raw SHA-256 of the prepublished canonical schedule
artifact. Each schedule entry contains exactly the scheduled call ID,
zero-based `schedule_index`, case ID,
source-set and perturbation digests, context arm/compiler digest, question and
decision-system digests, policy and threshold-set digests, repository role,
recorded execution order, and replicate index.

For each record stage separately, the validator requires a bijection between
schedule entries and receipts:

1. exactly one terminal record exists for every scheduled call ID;
2. stopped calls have an explicit `not_attempted` record rather than no record;
3. no duplicate or extra scheduled call ID exists;
4. every scheduled field, including `schedule_index`, equals the corresponding
   receipt field;
5. runner and analysis records pair one-to-one by scheduled call ID and
   `identity_digest`; and
6. the collection manifest records and verifies every receipt and artifact
   digest.

Any collection failure invalidates confirmatory analysis. Invalid, failed, and
not-attempted records remain in all preregistered denominators.

## 6. Replay exclusion

`distill_lock_content_addressed_replay` is deliberately absent from receipt
schema v1. A replay experiment requires a new protocol amendment and schema
version that bind an authoritative freshness snapshot, amendment digest,
validity interval, revalidation receipt, cache key, and failure-closed stale
outcome. Matching historical hashes alone is never proof that mutable
lifecycle evidence is current.
