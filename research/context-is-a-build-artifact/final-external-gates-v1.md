# Final-Study External Gate Review v1

**Record:** `context-build-artifact/final-external-gates/v1`

**Review date:** 2026-09-20

**Retrieval window:** 2026-09-20T16:15:59Z–16:17:36Z

**Decision:** **NO-GO for final provider execution**

This record evaluates the public TypeSafe terms, privacy documents, data
processing terms, API/model documentation, and Jev 1.13 documentation against
the merged preregistration. It contains no credential, authorization header,
account response, provider call, or final-study outcome.

The public baseline establishes customer ownership of inputs and assignment of
outputs, no customer-data model-weight training without consent, a documented
`jev-1.13.0` identifier, public pricing of USD 0.042 per million input tokens,
provider-reported token usage, rate limits, and documented jagged edges.

It does not satisfy the mandatory final gates for bounded retention or enabled
ZDR, independent publication and disclosure, immutable model/system semantics,
stable API behavior, a provider-enforced numeric spend cap, provider-reported
monetary cost, or protection against study-responsive service changes. The MCA
also incorporates an Acceptable Use Policy URL that returned HTTP 404. No final
execution authorization may be created from this record.

## Retrieved official sources

Hashes are SHA-256 over the HTTP response-body bytes after redirects, excluding
response headers.

| Source | HTTP status | Bytes | SHA-256 |
|---|---:|---:|---|
| `https://typesafe.ai/legal/mca` | 200 | 249482 | `e8eda228c6da44f61aa09e2563765926393295b1ddefd88cd5a8f83c4f130e82` |
| `https://typesafe.ai/legal/privacy-policy` | 200 | 173226 | `9819f3e11b213ce3e7e96cdaaf2b9d68fe171e436128ed21bd12bf48eefd2dc1` |
| `https://typesafe.ai/legal/data-processing` | 200 | 178468 | `548d326ed3287aa35c4f60d2b03153feeb39704363a5ba1ee0a75bc0dddda3e0` |
| `https://docs.typesafe.ai/api.md` | 200 | 11772 | `7ea1c82d9d16bd06e3d38885a5292548293f91e6d1ade4682c27398ced51a9e5` |
| `https://docs.typesafe.ai/models.md` | 200 | 7245 | `9d20bb3c90a0147532d0b20ddc4c64579391be7842e965543b27bad684eeb4d6` |
| `https://docs.typesafe.ai/confidence.md` | 200 | 11396 | `97dafe98b77979906a2ad456013dd85a70dcecd109d0644b096817d910997ea5` |
| `https://docs.typesafe.ai/model-jaggedness/jev-1.13.md` | 200 | 11588 | `e69329bd32e91ac08f0bd2681ceb4aacc34923b9190e29c15c8f75d8e22950e2` |
| `https://docs.typesafe.ai/sdk/python/changelog.md` | 200 | 1565 | `c49eb12a55807efbbde7dc1779efd555a34d91cd70bde070242ce9f2371094e1` |
| `https://docs.typesafe.ai/sdk/python/api/types/responses.md` | 200 | 38625 | `7b0167d010dc165b6584939a550c8ec35d31c003f0e8095638a46a037b6f6f8a` |
| `https://docs.typesafe.ai/legal.md` | 200 | 1059 | `4141ec2e7efad5a2965e56273216300ee3ece1e8d85024f0d0692efa08afcf5c` |
| `https://typesafe.ai/legal/aup` | 404 | 7384 | `51550177001c1c6ddc369e8a4ad1b4e045774ac658d7c268611a0428c2da579e` |

The final AUP digest is the 404 response body and is not evidence of the
incorporated policy text.

## Gate classification

| Gate | Status | Basis |
|---|---|---|
| Customer input ownership and output assignment | passed | MCA Sections 4.2 and 11 |
| No customer-request/response weight training without consent | passed with condition | MCA Section 4.1, Privacy Policy, Models documentation; no consent may be granted |
| Exact current model identifier | passed as current fact | Models documentation names `jev-1.13.0` |
| Complete choice probability maps | passed as current fact | API and confidence documentation |
| Current public token price | passed as current fact | USD 0.042/M input; output free |
| Provider-reported token usage | passed as current fact | documented input/output token fields |
| Known Jev 1.13 limitations | passed as current fact | versioned jaggedness document |
| Immutable model and complete serving-system identity | unresolved | no promise binds weights, routing, prompts, preprocessing, postprocessing, probabilities, or calibration to the identifier |
| Stable API/SDK semantics | failed | MCA permits incompatible API updates; SDK changelog alone is insufficient |
| Provider-reported monetary cost | failed | no documented monetary-cost or credits-consumed response field |
| Bounded retention or enabled ZDR | failed under standard terms | necessity-based retention; ZDR is enterprise-only and unconfirmed |
| Independent publication of all outcomes | unresolved | ownership is favorable, but no affirmative publication/no-veto promise exists |
| Disclosure of credits/support/collaboration | failed as-is | account terms and relationship announcements are restricted or confidential |
| Complete applicable use restrictions | unresolved | incorporated AUP URL returned 404 |
| Provider-side hard numeric spend cap | unresolved | no documented cap control, fail-closed guarantee, or activation receipt |
| Post-submission non-tuning beyond weights | unresolved | telemetry/service improvement and serving changes remain permitted |
| Account-specific controlling terms | unresolved | no account Order or separate agreement was inspected |

## Exact clarification required from TypeSafe

Before any final call, TypeSafe would need to answer in a controlling written
record:

1. May the researchers publish the complete methodology and all favorable,
   unfavorable, null, negative, invalid-run, cost, and limitation findings
   without TypeSafe approval, suppression, veto, editorial control, or delay
   beyond a predeclared security embargo?
2. May the publication identify TypeSafe and Jev, accurately state that the
   service was used, and make nominative references to `jev-1.13.0` despite MCA
   Section 16.4?
3. May all credits, free/promotional usage, pricing needed for reproducibility,
   support, collaboration, review, funding, and relationships be disclosed?
4. What operative AUP is incorporated by the MCA, and does it permit
   independent evaluation, benchmarking, aggregate statistical analysis, and
   publication without classifying the work as prohibited distillation,
   imitation training, or competing-product development?
5. Does `jev-1.13.0` permanently bind immutable weights, tokenizer, inference
   code, routing, prompts, preprocessing, postprocessing, probability and
   confidence calculations, calibration, and safety behavior, with every
   change requiring a new identifier?
6. Will every study request be served only by that immutable system, echo the
   exact identifier, and avoid silent fallback, alias resolution, canaries,
   A/B tests, routing substitutions, or replacements?
7. Will the exact version remain callable for the complete execution window,
   with a specified withdrawal-notice period?
8. What fixed API/version contract governs request/response schema,
   probability/confidence semantics, usage, errors, request IDs, compatibility,
   deprecation, and changelog behavior for the complete window?
9. What is the normative, version-bound confidence formula?
10. What exact account price, credit conversion, taxes/fees, promotional-credit
    treatment, token-accounting rule, output price, and price-effective period
    apply, and can TypeSafe issue a signed or versioned pricing record?
11. Can TypeSafe provide per-request monetary cost or credits consumed tied to
    request ID, or approve deterministic inference from reported input tokens
    and the fixed signed price?
12. Is there a provider-enforced hard numeric spend cap that rejects before the
    aggregate authorized amount can be exceeded under concurrency and delayed
    accounting, and what activation receipt proves the exact cap and disabled
    refill?
13. Will ZDR be enabled before submission, with exact maximum retention for
    bodies, errors, headers, IDs, logs, abuse records, telemetry, hashes,
    derived statistics, caches, backups, and subprocessors?
14. What ZDR exceptions remain, and can TypeSafe issue an account-scoped,
    time-scoped activation/deletion receipt?
15. Will no pilot/final input, output, error, telemetry, aggregate result,
    hypothesis, or communication be used before freeze/publication to train,
    select examples, change prompts/routing/calibration/thresholds,
    preprocessing/postprocessing, safety rules, or otherwise tune the service?
16. Will the account receive no study-specific human review, intervention,
    routing, tuning, or optimization?
17. Do any Order, checkout, promotional-credit, enterprise, or account-specific
    terms override the public terms, and what exact text controls?

## Account API disposition

The public API documentation exposes inference and model-list behavior, not
retention settings, publication rights, controlling account terms, ZDR
activation receipts, or a numeric spend-cap control. An authenticated model
list cannot cure the failed or unresolved mandatory gates above. To minimize
credential use, no authenticated account request is made while this no-go
decision is active. If controlling written answers later clear every mandatory
gate, the exact account model list, pricing/credit state, ZDR state, and spend
cap must be fetched from a clean merged implementation and added to a new
authorization record before execution.
