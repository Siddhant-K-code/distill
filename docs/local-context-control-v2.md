# Local context-control v2 operator contract

V2 is the separate prospective successor described in
`local-context-control-v1-status-and-v2-amendment.md`. Do not run v1 again and
do not execute v2 observations from an implementation or review checkout.

## Portable offline package

```bash
make distill-local-context-control-v2
```

This prepares, validates, and summarizes the 14-base, 118-condition,
332-observation-per-model v2 package without network access or model execution.

Frozen v2 identities:

- corpus: `222d1a4f021022fdb048f705bf01fd3001833f441fc2f800ad960c41a174ffdc`
- protocol: `d1551d992e55c112b4e6e18fea5a018bdb360969947f2f4a45148a3ac50a1b27`
- contexts JSONL: `d97859a33500b10453bef8869e43327ed1e33f981865e5e35dceaaaff1e0f707`
- schedule JSONL: `46ab21daf6d64eda431364cdf2c8e81cd5fce0d2dd38321a6dd5863e991edbf0`
- result schema: `3f98d5c5427f46192fbf4413d90119e32fc618bd00679a19a32a0d742196ac11`
- package manifest: `c0ad25e2c16bb38bf80b1164696580367836248bcd4a9697d7e5e91b7a943f85`
- framing conformance receipt: `d864b23a23ff10402bc44a9261b31903077f6f7915a2374db7c4e122caaedae3`

## Clean-main execution sequence

Use a separate, explicitly authorized session from the exact reviewed and
merged `origin/main` commit.

1. Build `distill-local-context-control-v2` from clean merged main.
2. Prepare and validate the v2 offline package from that checkout and retain
   its path. The package is an input and may be under the repository build
   directory; only the private output namespaces below must be external.
3. Recreate and independently retain the pinned CPython/runtime tree and wheel
   verification records.
4. Write owner-only v2 model and runtime manifests.
5. Run `preflight`; any frozen host eligibility failure blocks execution.
6. Choose absent, absolute, owner-private adapter-check, authorization, and run
   namespaces. They must be distinct and outside the repository, model, and
   runtime trees.
7. Run `adapter-check` with the reviewed commit/tree and an expiry no more than
   two hours after check start. It must finish with zero model loads.
8. Run `authorize` with the successful adapter-check directory and this exact
   acknowledgement:

```text
I authorize only the pinned local 4B v2 development/calibration schedule with zero network access, zero cloud spend, and no scaling control.
```

9. Run the exact `run` command once. The runner repeats pre-load validation,
   consumes the ledger only after the adapter preload handshake, and then
   issues the explicit load command.
10. Preserve all private records under independent custody. Verify and summarize
   in a separate result-review session.

There is no retry flag. Any consumed failure is terminal and must remain
preserved. No TypeSafe/AgentTrace held-out, provider, cloud, network, or scaling
access is authorized.
