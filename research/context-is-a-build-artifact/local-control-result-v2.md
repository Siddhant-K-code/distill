# Local context-control v2 result

**Status:** completed valid negative result. One attempt was consumed.

Compiled context made the pinned local Qwen3-4B more reliably answerable, not
reliably safe.

## Question

The study asked whether byte-stable context compilation improved local model
decision quality and selective risk under equivalent source perturbations.
This development and calibration study compared raw Arm A with compiled Arm C.

## Design

The run covered 14 independent bases, split evenly between Distill and
LLMTraceFX. It included 118 conditions and 332 observations. Of these, 236 were
primary observations and 96 were repeat observations across 48 arm pairs.

The corrected implementation merged in [#109](https://github.com/Siddhant-K-code/distill/pull/109)
at commit `670a48a993e7db129d6b75b84516d90977b1c460`, tree
`81b4a875473325c0d09894e1a1c175f8b6a3da6b`, after 12 CI checks passed.
That merge preceded and is distinct from study execution.

## Integrity chain

| Artifact | SHA-256 |
|---|---|
| Public aggregate | `452890410706cabb57b6580b598171052a22c2a27cef98c2da3ccdfe481d1f47` |
| Private result summary | `bea16fbb9fe5de39bb128be76f3874a3faecdc3c59c3cc7073743afa259130f4` |
| Private execution report | `1fcf50d28d4dadb3997de77eff88db918b9e06e52d177aa9dae1c4b034913e7b` |
| Runner | `c4bd2851fa1aa0006f949cfa4dd30ba5ee28a37c0a3741dad28c38568153486e` |
| Adapter | `9c85bd25470452c6564153756597f3636c8268d6eff3c90e66632b32e91f637e` |
| Authorization | `e2f3461d05b20327acd38672b0b9564d62b9fca22942fe9e92a29dbe8323bfa8` |
| Attempt ledger entry | `46196495ff620e5581b5f3a21c756c16740e888d7ce305ebfbe625540f65d8c1` |
| Receipt collection | `207eaea28ffc7238899e7bc037fb6b1c43cabb147f41bda1a525a79a75ea8970` |

The authorization hash is public-safe. Its content remains private. Model and
runtime custody were reverified after the run. Independent exact runner
verification passed and regenerated a byte-identical summary.

The frozen corpus, protocol, contexts, schedule, result schema, package, and
framing identities remain unchanged. The
[public checksum manifest](local-control-public-evidence-v1.SHA256SUMS) binds
this report and the aggregate record.

## Aggregate results

The run produced 271 valid observations, 61 malformed JSON observations, and
zero adapter errors.

| Measure | Arm A | Arm C | C minus A |
|---|---:|---:|---:|
| Equal-weight correctness | 0.28354978354978355 | 0.3733766233766234 | +0.08982683982683984 |
| Review burden | 0.5720779220779221 | 0.3093073593073593 | -0.26277056277056277 |
| Unsafe acceptance | 0.26774891774891774 | 0.43528138528138527 | +0.16753246753246756 |

The exact two-sided sign-flip value of 0.09375 applies only to correctness. It
does not support a conventional significance claim. No p-value was defined or
computed for unsafe acceptance.

Leave-one-base-out correctness deltas ranged from +0.0478 to +0.1096.
Unsafe-accept deltas ranged from +0.1163 to +0.1958.

## Malformed mechanism

Primary Arm A produced 75 valid and 43 malformed observations out of 118.
Primary Arm C produced 118 valid and zero malformed observations. Across all
repeats, Arm A produced 105 valid and 61 malformed observations out of 166.
Arm C produced 166 valid and zero malformed observations.

Malformed outputs scored as `correctness=false`, `review=true`, and
`unsafe_accept=false`. Compilation removed malformed-as-review abstentions.

## Preregistered decision

The +0.16753246753246756 unsafe-accept delta exceeded the frozen +0.05 harm
threshold. The overall outcome is `negative`, even though correctness and
review burden improved.

Threshold development selected `no-safe-auto-action`. Confidence was
unavailable, so this result makes no calibrated-confidence claim.

## Repeatability

The repeat subset contained 48 arm pairs. Decision agreement, route agreement,
malformed agreement, and evidence-ID Jaccard were all 1.0. These figures show
realized repeatability in this run. They do not establish model determinism.

## Descriptive efficiency

Arm C used 2.462188731814656 times as many input tokens as Arm A. Mean request
latency was 3.060386754493316 percent higher. These figures are descriptive and
quiescent-host-conditioned, not a clean causal comparison. The quality phase
did not collect a Metal trace.

## Failure chronology

1. V1 ended in an invalid pre-observation startup failure with zero model loads
   and zero observations. Rerun remains prohibited.
2. The first v2 preparation stopped as `NO_RUN` at readiness because Colima met
   or exceeded the frozen 1 GiB unrelated-process RSS bound.
3. The second v2 preparation stopped as `NO_RUN` at adapter check because Go
   nil slices encoded as JSON `null` instead of empty arrays.
4. Both v2 preparations stopped before authorization, model load, and
   inference. The implementation then received an outcome-independent
   amendment.
5. One later authorized attempt completed the study.

## Limitations and claim boundaries

This result compares only Arm A and Arm C for the pinned local Qwen3-4B
artifact. It includes no larger-model comparison and supports no scaling claim.
It does not show that better context beats a bigger model. It makes no
calibrated-confidence claim, general determinism claim, unconditional
performance claim, or correctness significance claim.

The canonical public record is
[`local-control-public-aggregate-v1.json`](local-control-public-aggregate-v1.json).
It contains aggregates and public hashes only.
