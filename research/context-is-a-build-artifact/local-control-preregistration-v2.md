# Local context-control study v2 preregistration

Status: prospective, frozen before any v2 model output; two private
pre-authorization preparations are classified `NO_RUN`.

V2 is a new provider-independent local protocol after the outcome-free v1
startup failure documented in
`docs/local-context-control-v1-status-and-v2-amendment.md`. It is not a v1
retry. The v1 protocol, records, identities, consumed attempt, invalid
classification, and rerun prohibition remain unchanged.

The study retains 14 independent bases, 118 conditions, two arms, 236 primary
observations, a 24-condition repeat subset with 96 repeat observations, and 332
scheduled observations for the pinned local Qwen3-4B artifact. V2 separately
identifies its protocol, schedule, package, authorization, ledger, run,
receipts, and result. The corpus/context content is independently regenerated
and validated under v2 schemas and identities.

All substantive analysis, malformed-output, isolation, custody, cleanup,
resource, raw-response, and result-verification rules remain fail closed. V2
adds a normative cross-language JSON framing contract, known-answer
conformance, a zero-model-load adapter-check receipt, and a preload handshake
before single-attempt consumption. The sole attempt is consumed immediately
before the explicit load frame. No outcome-dependent retry is permitted.

Frozen identities are:

- corpus `222d1a4f021022fdb048f705bf01fd3001833f441fc2f800ad960c41a174ffdc`
- protocol `d1551d992e55c112b4e6e18fea5a018bdb360969947f2f4a45148a3ac50a1b27`
- contexts `d97859a33500b10453bef8869e43327ed1e33f981865e5e35dceaaaff1e0f707`
- schedule `46ab21daf6d64eda431364cdf2c8e81cd5fce0d2dd38321a6dd5863e991edbf0`
- package manifest `c0ad25e2c16bb38bf80b1164696580367836248bcd4a9697d7e5e91b7a943f85`
- framing conformance `d864b23a23ff10402bc44a9261b31903077f6f7915a2374db7c4e122caaedae3`

V1 supplied no model outcomes: four pre-authorization wrapper failures and the
consumed adapter-startup abort all had zero model loads and zero observations.
Consequently no observed model performance could select v2. Absolute
performance claims remain conditioned on the preregistered quiescent-host
requirements.

This implementation preregistration performs no real model inference. Execution
is blocked until the exact implementation is independently reviewed, merged to
main, and separately authorized from a clean-main execution session.

The first private preparation stopped at readiness because Colima exceeded the
frozen 1 GiB unrelated-process RSS limit. A later preparation passed three
readiness samples but stopped during adapter-check because an eligible
Go-produced host snapshot encoded empty `heavy_processes` and `failures` as
`null`, which failed the strict Python `failures == []` binding. No
adapter-check directory survived; neither preparation created authorization or
attempt-ledger records, loaded the model, ran inference, or produced model
output.

The producer correction is therefore an outcome-independent implementation
amendment under the existing v2 protocol. It does not change the frozen
scientific identities above and does not justify a v3 namespace. Any future
private preparation must use the corrected reviewed merged commit/tree, rebuilt
runner, corrected adapter identity, and newly generated owner-only runtime and
adapter-check records; the old private binary is execution-ineligible.
