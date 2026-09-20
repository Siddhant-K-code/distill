# TypeSafe / Jev Excluded-Pilot Provider Record v1

**Status:** implementation evidence for an excluded integration pilot only  
**Retrieved:** 2026-09-20  
**Final-study eligible:** no

The research-only `distill-typesafe-jev-pilot` command consumes the immutable
offline `requests.jsonl`; it does not change cases, labels, policy, or the
frozen request shape. It uses Go's standard `net/http` client rather than an
SDK so the exact raw response body and provider request ID can be preserved.
Redirects and automatic retries are disabled. Each scheduled call has one
transport attempt.

## Pinned provider contract

| Field | Pinned value |
|---|---|
| Provider | TypeSafe AI |
| Endpoint | `POST https://api.typesafe.ai/v1/systemone` |
| API version | `v1` |
| Model | `jev-1.13.0` |
| Moving aliases rejected | `jev-latest`, `jev-preview`, and any alias containing `latest`, `stable`, `preview`, or `newest` as a delimited token |
| Request | one text `state`; seven independent `choice` questions; each choice key maps to `null` criteria |
| Response | exact `model`, seven typed answers, complete probability maps, provider-defined confidence, input/output token usage |
| Request ID | `x-typesafe-request-id` response header |
| Price | USD 0.042 per million input tokens; output tokens free |
| Context limit | 64,000 total tokens; 32,000 for state plus the longest question |
| Rate limits | 250,000 tokens/second and 1,200 requests/minute; documented as dynamically adjustable |
| Execution client | Go standard library `net/http`; no TypeSafe SDK dependency |
| Reference SDK | Python `typesafe-sdk` 0.7.0 at `2ce5c65f13646cab6e6f782328194c9d85f3300a` |

TypeSafe Choice `confidence` is a provider-defined statistic derived from the
probability distribution, not the selected label's probability. The adapter
preserves that field in the hash-bound raw response and verifies that it is a
finite value in `[0,1]`. The semantic receipt maps `confidence` to the selected
label's probability with
`confidence_semantics=selected_label_probability`, as required by the frozen
request contract. The full probability vector remains authoritative for the
preregistered acceptance score. Provider-reported cost is absent from the
documented response and is therefore `null`. The append-only ledger separately
derives cost from `input_tokens` and the pinned public input-token price.

The authenticated `GET /v1/models` account request returned only the moving
aliases `jev-latest` and `jev-preview`. The immutable identifier
`jev-1.13.0` is pinned from the official models document and must be echoed
exactly by every paid response. The local authorization directory preserves
the model-list body and request ID with their digests; neither is committed.

## Known Jev 1.13 jagged edges

The provider documents literal reading, weak numeric/date reasoning,
indirection, context degradation from irrelevant detail, adversarial content,
contradictory instructions and criteria, non-guaranteed structural
invariants, and lack of generation support. The pilot keeps calculations and
policy in code and treats all provider outputs as excluded measurements.

## Privacy and unresolved external gates

The submitted fixtures are synthetic/public-domain-style and contain no
personal, employer, user, or private-repository data. The public MCA permits
customer inputs and assigns any provider output to the customer. The MCA and
privacy policy state that inputs are not used to train or fine-tune models
without prior consent.

Publication independence and disclosure terms, held-out non-tuning,
account-specific retention/ZDR, and a provider-side numeric spend-cap API are
not established by the public API. They remain unresolved external
collaboration gates. This excluded integration pilot does not submit held-out
or final-study data. Its local authorization reserves the documented
64,000-input-token worst case before every call and stops fail-closed under an
exact USD 5.000000 cap. Infrastructure failures pause execution after the
single failed attempt; rerunning the same command validates the existing
prefix and continues only at the next never-attempted schedule entry.

## Retrieved evidence

| Source | SHA-256 |
|---|---|
| `https://raw.githubusercontent.com/typesafe-ai/skills/main/skills/typesafe-ai/SKILL.md` | `71ea90d7906c6554c4f4c460ef7361b2d26f59116ccdae986dc6d997b9389f52` |
| `https://docs.typesafe.ai/llms.txt` | `9bb713532b8b5b836d48bf19733a6649525aff8353e22c5cbebf5524a498ff24` |
| `https://docs.typesafe.ai/api.md` | `7ea1c82d9d16bd06e3d38885a5292548293f91e6d1ade4682c27398ced51a9e5` |
| `https://docs.typesafe.ai/models.md` | `9d20bb3c90a0147532d0b20ddc4c64579391be7842e965543b27bad684eeb4d6` |
| `https://docs.typesafe.ai/confidence.md` | `97dafe98b77979906a2ad456013dd85a70dcecd109d0644b096817d910997ea5` |
| `https://docs.typesafe.ai/primitives/choice.md` | `4c55085131f8f2d7ae1faefdec52b1c7b379d40657d701cd2c786021cfe9dac9` |
| `https://docs.typesafe.ai/model-jaggedness/jev-1.13.md` | `e69329bd32e91ac08f0bd2681ceb4aacc34923b9190e29c15c8f75d8e22950e2` |
| `https://typesafe.ai/legal/mca` | `e8eda228c6da44f61aa09e2563765926393295b1ddefd88cd5a8f83c4f130e82` |
| `https://typesafe.ai/legal/privacy-policy` | `9819f3e11b213ce3e7e96cdaaf2b9d68fe171e436128ed21bd12bf48eefd2dc1` |
| `https://typesafe.ai/legal/data-processing` | `548d326ed3287aa35c4f60d2b03153feeb39704363a5ba1ee0a75bc0dddda3e0` |
| `https://docs.typesafe.ai/sdk/python/changelog.md` | `c49eb12a55807efbbde7dc1779efd555a34d91cd70bde070242ce9f2371094e1` |
| `https://docs.typesafe.ai/sdk/python/api/retries.md` | `bdf94b25ae1d23a0784c9e1affcf0363e80deb56089a0e48d3e3860bb5181cc9` |
| `https://docs.typesafe.ai/sdk/python/api/types/responses.md` | `7b0167d010dc165b6584939a550c8ec35d31c003f0e8095638a46a037b6f6f8a` |
