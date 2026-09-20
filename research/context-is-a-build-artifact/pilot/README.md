# Offline Excluded Pilot

This directory documents the research-only `distill-jev-pilot` harness. The
harness is not part of the public `distill` CLI. It makes **zero provider
calls**, reads no credentials or environment variables, implements no
transport, and produces only deterministic synthetic/public-domain-style data.
All outputs are permanently excluded pilot material and are never final-study
eligible.

From the repository root:

```bash
go run ./cmd/distill-jev-pilot prepare --output build/pilot
go run ./cmd/distill-jev-pilot validate build/pilot
go run ./cmd/distill-jev-pilot summarize build/pilot
# or run all three:
make distill-jev-pilot
```

`prepare` requires a nonexistent destination and publishes atomically without
replacement. `validate` fails closed on file-set, canonicalization, digest,
schema, identity, bijection, policy, privacy, or exclusion errors.
`summarize` validates first and reports fixture counts only. The ignored
`build/pilot` snapshot must not be committed. The Make target intentionally
fails if `build/pilot` already exists; preservation and no-replace publication
take precedence over silently deleting a previously validated package.

## Remaining gates

A future, separately authorized provider adapter may be designed only after
all preregistration gates exist as content-addressed records:

1. exact immutable model identifier and version;
2. versioned probability and confidence semantics;
3. stable API and SDK versions and changelog behavior;
4. provider pricing, credits, usage fields, and deterministic rate-limit
   failure handling;
5. documented jagged edges, unsupported cases, and provider variability;
6. publication-independence agreement and disclosure terms;
7. successful offline pilot and finalized prospective amendments;
8. least-privilege credential lifecycle and provider-side spend cap; and
9. the creator's explicit numeric excluded-pilot budget authorization.

Only then may a separate adapter consume `requests.jsonl` and write provider
answers into distinct receipt artifacts. It must never overwrite requests,
labels, or deterministic policy output. A final run additionally requires a
successful excluded provider-integration pilot and separate explicit numeric
final-execution budget authorization.

Each request contains one immutable text `state` plus seven independent
`choice` questions. The shape deliberately mirrors the documented
state-plus-typed-question workflow while remaining provider-neutral: it
contains no endpoint, model, API, SDK, credential, or transport configuration.
A future adapter must map this frozen shape to a separately pinned provider
contract rather than changing the request artifacts in place.

The request state is the preregistered raw deterministic-concatenation arm,
not a Distill Lock v0 bundle. Every case preserves its paired base and
transformed source sets, while the frozen Distill Lock identities remain
separate anchors. The harness does not attribute custom-rendered bytes to
Distill Lock.

Provider receipt validation also requires the separately pinned semantic
validator to parse the preserved raw provider artifacts and reproduce the
structured outputs, usage, cost, and provider request ID exactly. A receipt
cannot validate merely by presenting internally consistent structured fields.

This harness publishes no provider outcome, study result, selected threshold,
qualification decision, or final-study metric.
