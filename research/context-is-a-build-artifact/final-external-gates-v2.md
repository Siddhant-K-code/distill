# Final-Study External Gate Review v2

**Record:** `context-build-artifact/final-external-gates/v2`

**Review date / public-source access date:** 2026-09-21

**Decision:** **NO-GO for final provider execution**

**Relationship to v1:** prospective public-evidence refresh. It supersedes only
the current provider assessment in
[`final-external-gates-v1.md`](final-external-gates-v1.md). It does not alter or
delete that historical record, its exact 17 questions, or any frozen
corpus/split/schedule/schema/question/source/receipt identity.

This review uses only public official material. Public contract text is not the
account-specific Order, checkout page, confirmation email, promotional-credit
notice, or any separate agreement that may control the account. None of those
account records was inspected. Public text is also not written, study-specific
TypeSafe consent. Only a controlling account record and affirmative written
consent can resolve the corresponding gates.

No provider call, final authorization, ledger, provider output, result, or
label unseal occurred. The final runner remains fixed at 21 bases, 150
conditions, 420 maximum calls, and Arms A/C only. Its production trust root is
empty. Final calls and final spend remain zero. The prior excluded pilot's
inferred spend is USD 0.000760242, leaving an exact final cap of USD
4.999239758.

## Freeze and package disposition

This Markdown record is not listed by the frozen final amendment, source
registry, corpus, split, schedule, question schema, receipt schema, or a
package-local checksum manifest. Existing repository wiring refers to the v1
record only from the research README. Adding v2 to a frozen evidence envelope
would change a prospective identity after freeze, so this file remains an
**unbound post-freeze evidence refresh**. The README link is navigational only.
No frozen hash or package checksum is regenerated.

## Newly available official evidence

All sources below are first-party TypeSafe publications accessed on
2026-09-21. They describe public terms and current documentation, not this
account's complete controlling agreement or a study-specific commitment.

| Source | Public evidence relevant to this review |
|---|---|
| [Legal index](https://docs.typesafe.ai/legal) | Links the MCA, DPA, and Privacy Policy; states that ZDR is available for enterprise customers. |
| [Master Customer Agreement](https://typesafe.ai/legal/mca) (updated 2026-09-19) | The preamble defines the Agreement as the MCA plus the account Order. Sections 2.5, 4.1-4.3, 8.2, 14.1, 16.4, 16.7, and 16.14 address updates, data/output/telemetry, credits, confidentiality, publicity, amendments, and Order precedence. |
| [Data Processing Addendum](https://typesafe.ai/legal/data-processing) (updated 2026-04-24) | Sections 2 and 5 impose processor-purpose and security duties for Customer Personal Data; Schedule I.8 retains it for as long as necessary, without a public numeric deletion bound. |
| [Privacy Policy](https://typesafe.ai/legal/privacy-policy) | Says TypeSafe will not train or fine-tune models on prompts or other Input and will not disclose Input except to service providers; retention is only for as long as reasonably necessary. |
| [Models](https://docs.typesafe.ai/models) | Documents `jev-1.13.0`, moving aliases, same model weights for every account, no training on customer requests/responses, USD 0.042 per million input tokens with free output, dynamic rate limits, and enterprise-only ZDR. |
| [API reference](https://docs.typesafe.ai/api) | Documents a response `model` field containing the versioned model ID and `usage.input_tokens` / `usage.output_tokens`; no monetary-cost field is documented. |
| [Confidence](https://docs.typesafe.ai/confidence) | Documents confidence as derived from probability distributions, but presents the displayed Choice formula as an approximation rather than a normative, version-bound service contract. |

An independent observation snapshot at 2026-09-21T12:16:27+05:30 recorded
SHA-256 digests `c41c8c849b4ef57831124f562161eab5808dad56cbcba3088496622e0ff05d9a`
for the MCA, `e4fbca7d1a9bc4254009b60077ed4e462cd56e67c16b320140912eeffff337e2`
for the DPA, `38ea8599245f64334601910bf9a8c8d6d20cb8d450ad41749261b637e6456f5f`
for the Privacy Policy,
`9d20bb3c90a0147532d0b20ddc4c64579391be7842e965543b27bad684eeb4d6`
for `models.md`, and
`7ea1c82d9d16bd06e3d38885a5292548293f91e6d1ade4682c27398ced51a9e5`
for `api.md`. The public URLs and access date remain the primary citations;
the downloaded response bodies are not committed.

The MCA preamble makes API access conditional on an Agreement comprising the
MCA and the account's Order (an executed order, checkout page, or confirmation
email), and MCA 16.14 makes the Order control conflicts. MCA 16.7 permits
prospective agreement updates after at least 60 days' notice. These provisions
make the public MCA important evidence, but they also make the uninspected
account record mandatory.

MCA 2.5 permits service updates that may make the API incompatible and promises
only commercially reasonable efforts to give advance notice when TypeSafe
believes an API update will materially and adversely affect integration. MCA
4.1 bars model-weight training on Customer Data without prior customer consent,
while granting perpetual Customer Data processing rights for Telemetry,
fraud/abuse monitoring, and legal compliance. MCA 4.3 permits unrestricted
Telemetry processing, including service improvement. MCA 4.2 assigns
TypeSafe's rights in Output to the customer and disclaims TypeSafe ownership,
but does not grant publication independence or override confidentiality and
publicity terms.

MCA 8.2 distinguishes purchased and promotional credits, supports optional
automatic purchased-credit refills, and allows issuance-specific promotional
terms. MCA 14.1 treats Customer fees, all pricing, Agreement terms, and
non-public service information as TypeSafe Confidential Information. MCA 16.4
bars either side from using the other's name, brand, or logo or publicly
announcing the agreement without consent, while permitting TypeSafe to identify
the customer unless asked in writing to stop. Those clauses require explicit
publication and disclosure consent before this study can name TypeSafe/Jev or
report reproducibility-relevant contract and pricing facts.

## Classification of the 17 prior gate issues

These labels classify public evidentiary coverage, not execution readiness.
`PUBLICLY EVIDENCED` means the current public source directly documents the
narrow issue. `PARTIALLY EVIDENCED` means useful public evidence exists but
does not establish the complete study requirement. `STILL BLOCKING` means the
mandatory requirement lacks a controlling account- and study-specific record.

| Prior v1 gate issue | Classification | Current basis |
|---|---|---|
| Customer input ownership and output assignment | **PUBLICLY EVIDENCED** | MCA 4.2 and 11 retain customer Input rights and assign TypeSafe's rights in Output. |
| No customer-request/response weight training without consent | **PUBLICLY EVIDENCED** | MCA 4.1, the Privacy Policy, and Models documentation cover model-weight training; the study must not consent. |
| Exact current model identifier | **PUBLICLY EVIDENCED** | Models documents the pinned `jev-1.13.0`; aliases move. |
| Complete choice probability maps | **PUBLICLY EVIDENCED** | The API reference documents complete Choice probability maps. |
| Current public token price | **PUBLICLY EVIDENCED** | Models documents USD 0.042/M input tokens and free output as the current public price. |
| Provider-reported token usage | **PUBLICLY EVIDENCED** | The API response documents input/output token counts. |
| Known Jev 1.13 limitations | **PUBLICLY EVIDENCED** | The versioned Jev 1.13 documentation remains public; this review does not convert provider claims into independent validation. |
| Immutable model and complete serving-system identity | **PARTIALLY EVIDENCED** | Same weights for every account and a versioned response ID are favorable, but do not bind tokenizer, inference code, routing, prompts, configuration, preprocessing/postprocessing, calibration, safety logic, or manual intervention. |
| Stable API/SDK semantics | **STILL BLOCKING** | MCA 2.5 expressly permits incompatible API updates and provides no study-window compatibility guarantee. |
| Provider-reported monetary cost | **PARTIALLY EVIDENCED** | Token usage and public price support inference, but the documented response has no monetary-cost or consumed-credit field. |
| Bounded retention or enabled ZDR | **STILL BLOCKING** | The DPA and Privacy Policy use necessity-based retention; public documentation limits ZDR availability to enterprise customers, with no account activation or deletion receipt. |
| Independent publication of all outcomes | **STILL BLOCKING** | Output assignment is favorable but is not a publication/no-veto promise; MCA 14.1 and 16.4 create disclosure and publicity constraints. |
| Disclosure of credits/support/collaboration | **STILL BLOCKING** | MCA 8.2 allows issuance-specific promotional terms, and MCA 14.1 covers pricing and Agreement terms. |
| Complete applicable use restrictions | **PARTIALLY EVIDENCED** | MCA 2.3 publicly states license restrictions, including model-distillation and competing-product restrictions; complete account-specific, Order, promotional-credit, and any applicable AUP terms remain uninspected. The public `/legal/aup` URL returned HTTP 404 on 2026-09-21. |
| Provider-side hard numeric spend cap | **STILL BLOCKING** | MCA 8.2 describes balances and optional auto-refill, not a fail-closed numeric cap at USD 4.999239758. |
| Post-submission non-tuning beyond weights | **PARTIALLY EVIDENCED** | No-training and same-weights statements improve this gate, but MCA 4.3 permits Telemetry use for service improvement and no source forbids study-responsive routing/configuration changes. |
| Account-specific controlling terms | **STILL BLOCKING** | The MCA identifies the Order and any effective separate agreement as controlling account records, but none was inspected. |

The no-training and same-weights claims materially improve the assessment:
they reduce the risk of customer-request/response weight updates and
account-specific weights. They do **not** establish unchanged serving code,
routing, prompts, preprocessing, postprocessing, calibration, thresholds,
safety configuration, canaries, fallbacks, or human intervention during the
study window. They also do not bound retention: the public DPA and Privacy
Policy retain data under necessity standards, while MCA 4.1 and 4.3 preserve
Telemetry processing rights.

## Disposition of the exact v1 questions

The exact wording remains frozen in v1. This crosswalk ensures that none of its
17 issues is silently dropped.

| v1 question | Group | Classification | Reason |
|---:|:---:|---|---|
| 1 | B | **STILL BLOCKING** | No all-outcomes publication/no-veto consent. |
| 2 | B | **STILL BLOCKING** | MCA 16.4 makes naming and agreement publicity consent-sensitive. |
| 3 | A/B/E | **STILL BLOCKING** | Account credit terms and permission to disclose pricing/support/relationships are absent. |
| 4 | A/F | **PARTIALLY EVIDENCED** | MCA 2.3 supplies public restrictions; complete applicable AUP/account terms and express evaluation permission are absent. |
| 5 | C | **PARTIALLY EVIDENCED** | Same weights and a version ID do not bind the complete serving system. |
| 6 | C | **PARTIALLY EVIDENCED** | Response `model` is documented; no anti-fallback/routing/canary guarantee exists. |
| 7 | C | **STILL BLOCKING** | No complete-window availability or withdrawal-notice commitment. |
| 8 | C | **STILL BLOCKING** | MCA 2.5 permits incompatible changes. |
| 9 | C | **PARTIALLY EVIDENCED** | Confidence is documented as probability-derived, but not normatively version-bound. |
| 10 | A/E | **PARTIALLY EVIDENCED** | Public price and general credit rules exist; exact account and promotional terms do not. |
| 11 | E | **PARTIALLY EVIDENCED** | Token-based inference is possible, but provider cost/reconciliation or acceptance of inference is absent. |
| 12 | E | **STILL BLOCKING** | No provider-enforced numeric cap or activation receipt. |
| 13 | D | **STILL BLOCKING** | No account ZDR confirmation or bounded retention for each data class. |
| 14 | D | **STILL BLOCKING** | No ZDR exception, activation, or deletion receipt. |
| 15 | C/D | **PARTIALLY EVIDENCED** | Weight-training limits exist; broader study-responsive service use/change remains permitted or unspecified. |
| 16 | C/D | **STILL BLOCKING** | No prohibition on study-specific human access, review, intervention, or routing. |
| 17 | A | **STILL BLOCKING** | The controlling account Order/checkout/confirmation and issuance-specific terms were not inspected. |

## Remaining mandatory blockers

All six groups are mandatory. Partial public evidence does not clear a group.

**A. Controlling account terms.** Confirm the account-specific Order,
checkout/confirmation, all applicable promotional-credit and AUP/usage terms,
and whether the 2026-09-19 MCA is the controlling agreement.

**B. Publication and disclosure consent.** Obtain written TypeSafe consent for
independent publication naming TypeSafe/Jev and reporting favorable, negative,
null, invalid, and operational results with no prepublication veto, plus
permission to disclose necessary public pricing and contract facts, credits or
free/promotional usage, support, collaboration, review, funding, and other
relationships notwithstanding MCA 14.1 and 16.4.

**C. Study-window serving stability.** Bind the frozen serving identity beyond
the response `model`; prohibit study-responsive weights, configuration,
routing, canaries/fallbacks, and manual intervention; prohibit use of study
inputs, outputs, errors, Telemetry, aggregate results, hypotheses, or
communications before freeze/publication to select examples or tune prompts,
routing, calibration, thresholds, preprocessing/postprocessing, safety rules,
or other service behavior; keep `jev-1.13.0` callable for the complete study
window with a specified withdrawal-notice period; and guarantee stable API and
confidence semantics throughout that window.

**D. Retention and access boundaries.** Establish bounded retention or enabled
ZDR for this account, the exact boundary between raw input/output and
Telemetry, deletion timing, exceptions/backups/subprocessors, and human-access
boundaries.

**E. Spend control and reconciliation.** Establish a provider-side numeric
hard cap at or below USD 4.999239758, disable auto-refill, obtain all
promotional-credit terms, establish the controlling account price, credit
conversion, taxes/fees, token-accounting rule, output price, and
price-effective period, and obtain authoritative provider monetary-cost
reconciliation or explicit acceptance of cost inferred from those controlling
terms as the study's only cost measure.

**F. Usage rules and escalation.** Obtain the complete applicable AUP/usage
restrictions and support/escalation contacts for study-window incidents.

Until A-F are satisfied in controlling written records, provider status remains
**NO-GO**. No final authorization may be created, no trust root may be
populated, no held-out label may be accessed or unsealed, and no final provider
call may be made.

## Send-ready compact question block

No email was sent. The following block is ready for human review and delivery.

> **To:** sales@typesafe.ai; support@typesafe.ai; privacy@typesafe.ai
>
> **Subject:** Written controls required for independent `jev-1.13.0` study
>
> We are considering a small independent evaluation with a strict remaining
> provider cap of USD 4.999239758. Please answer each group in a controlling
> written record:
>
> **A - Account terms (sales):** Please provide or identify the
> account-specific Order/checkout/confirmation, all promotional-credit and
> AUP/usage terms that apply, any separate agreement, and confirm whether the
> 2026-09-19 MCA controls this account.
>
> **B - Publication (sales/legal):** Does TypeSafe consent to independent
> publication naming TypeSafe and Jev and reporting favorable, negative, null,
> invalid, and operational results without approval, suppression, editorial
> control, delay, or prepublication veto? Please also permit disclosure of the
> public pricing and contract facts, credits or free/promotional usage,
> support, collaboration, review, funding, and other relationships needed for
> reproducibility notwithstanding MCA 14.1 and 16.4.
>
> **C - Serving identity (support/sales):** For the complete frozen study
> window, what identity beyond response `model=jev-1.13.0` binds weights,
> inference/configuration, routing, prompts, preprocessing/postprocessing,
> confidence/probability semantics, and safety behavior? Please confirm no
> study-responsive change, fallback/canary/A-B route, or manual intervention.
> Please also confirm that study inputs, outputs, errors, Telemetry, aggregate
> results, hypotheses, and communications will not be used before
> freeze/publication to select examples or tune prompts, routing, calibration,
> thresholds, preprocessing/postprocessing, safety rules, or other service
> behavior; that `jev-1.13.0` will remain callable for the complete window;
> and state the API/version stability, confidence-semantics stability, and
> withdrawal-notice commitments.
>
> **D - Retention/access (privacy):** Will bounded retention or ZDR be enabled
> for this account before submission? Please state exact maxima and deletion
> timing for raw inputs/outputs, errors, headers/IDs, logs, Telemetry, hashes,
> derived statistics, caches, backups, and subprocessors; remaining exceptions;
> human-access boundaries; and the activation/deletion evidence available.
>
> **E - Spend/cost (sales/support):** Can TypeSafe enforce a fail-closed
> provider-side numeric cap at or below USD 4.999239758 with auto-refill
> disabled, including under concurrency/delayed accounting? Please provide the
> activation evidence, promotional-credit terms, controlling account price,
> credit conversion, taxes/fees, token-accounting rule, output price, and
> price-effective period, plus authoritative per-request/aggregate
> monetary-cost reconciliation, or explicitly accept cost inferred from
> provider-reported input tokens and those controlling terms as the study's
> only cost measure.
>
> **F - Restrictions/escalation (sales/support):** Please provide every
> applicable AUP/usage restriction and confirm whether independent evaluation,
> aggregate analysis, and publication are permitted and not prohibited
> distillation, imitation training, or competing-product development. Please
> identify operational, billing, privacy, and urgent study-window escalation
> contacts.
