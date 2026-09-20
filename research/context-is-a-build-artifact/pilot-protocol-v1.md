# Offline Excluded-Pilot Protocol Clarification v1

**Status:** prospective, versioned clarification

**Date:** 2026-09-20

**Applies to:** synthetic offline excluded-pilot fixtures only

This clarification defines the offline implementation without inspecting any
model/provider response or study outcome. No model, provider, API, SDK,
credential, network, threshold metric, or final-study outcome was used. It
does not rewrite the frozen preregistration and cannot alter final-study
questions, policy, thresholds, sample allocation, or decisions.

## Seven provider questions

The provider-facing request has exactly these seven independent Choice fields:

1. `evidence_complete`: **Are all evidence items required by the registered rubric present?**
2. `observed_tests_support`: **Do the preserved observed test results support the proposed handoff?**
3. `verifier_support`: **Does the registered verifier outcome support the proposed handoff?**
4. `cleanup_complete`: **Are all registered cleanup obligations complete?**
5. `external_effects_resolved`: **Are all registered external effects resolved or explicitly accepted?**
6. `patch_risk`: **What is the evidence-supported patch-risk class?**
7. `recommended_disposition`: **What disposition does the evidence support?**

Their choices are exactly those in preregistration Section 6. Every
probabilistic response must return the selected label, the complete
distribution over that field's choices, and confidence equal to the selected
label's probability. Contradiction and staleness are independently derived
policy evidence. They are not provider questions and do not expand the frozen
seven-field output.

The four `not_applicable`-capable fields also carry a pre-execution
applicability bit in the case registry. `not_applicable` is acceptance-safe
only when that bit is false; it is invalid when the registered obligation
applies. An applicable field is safe only when its answer is `yes`.

## Untuned deterministic rule table

Rules are fixed without provider output or synthetic provider responses and use
the strongest matching route (`reject` before `review` before `accept`):

| Route | Frozen condition |
|---|---|
| `accept` | Evidence is complete; applicable observed tests, verifier, cleanup, and external effects are safe; risk is low; the recommended disposition is `accept`; and independently derived contradiction and staleness are both false. |
| `reject` | Observed tests are `no`, verifier support is `no`, external effects are unresolved (`no`), evidence is contradictory, recommended disposition is `reject`, or patch risk is high. |
| `review` | Required evidence is incomplete or unknown, a required field is unknown, cleanup is missing, evidence is stale, risk is medium or unknown, or recommended disposition is `review`. |
| `review` | A decision failed or was not attempted. |

For future probabilistic receipt validation, the versioned acceptance score is
the minimum probability assigned to the acceptance-safe label set across the
seven fields (`yes`, registered-inapplicable `not_applicable`, `low`, and
`accept`).
Independent contradiction or staleness sets it to zero. This fixture score is
not a threshold, qualification, result, or final-study metric.

## Fixture conditions

Each synthetic fixture records both its unperturbed base source set and its
category-specific transformed source set. Source reorder uses the recorded
seed with Fisher-Yates; irrelevant append changes an existing source rather
than adding a new source. The request schedule contains one excluded condition
per fixture so case/request/schedule membership is bijective. It exercises
schema, transformation, custody, and receipt mechanics; it is not the final
study schedule and does not replace the preregistered requirement to apply
every applicable transformation to final eligible base cases.

Request state uses the frozen raw deterministic concatenation arm. It is not
identified as a Distill Lock v0 bundle. The package separately binds the
reviewed Distill Lock v0 identities and validates its unchanged demo/goldens.

Pilot labels are public synthetic fixtures stored in a separate file with a
deterministic content seal, and requests contain no label fields. This is
separation for harness testing, not the preregistered random-nonce hiding
commitment used for held-out final-study custody.

## Scope

The 18 cases are deterministic, synthetic, public-domain-style fixtures. They
contain no user, employer, or private repository content. Every artifact is
permanently marked `phase=pilot`, `excluded=true`, and
`final_study_eligible=false`. Pilot records cannot be relabeled as final data.
