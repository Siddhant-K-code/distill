# Local context-control v1 status and prospective v2 amendment

## Frozen v1 status

V1 remains frozen. Its implementation, preregistration, corpus, contexts,
schedule, identities, consumed authorization, attempt ledger, private execution
marker, durable recovery lock, and sealed aborted record must not be deleted,
rewritten, reinterpreted, or rerun.

The one authorized v1 attempt aborted during adapter startup before observation
1. The Python adapter parsed the Go-written `runtime-manifest.json` with a
payload-only strict decoder, rejected the file's normative single terminal LF
as trailing JSON bytes, and exited before model verification, `import mlx`,
`mlx_lm.load`, or a ready envelope. There were zero call directories, receipts,
completion records, result summaries, observations, model loads, network
requests, provider calls, cloud calls, or held-out accesses.

The v1 classification is
`invalid_pre_observation_adapter_startup_failure`; rerun is prohibited. The
sealed private aborted-record digest is
`821ba7bc9b845ef32e199df396f0a17f3c05ea49d3acd8f98f9d31b18ccca8a7`.
Independent security and methodology reviews approved this classification.
Before authorization, four wrapper attempts also failed with zero model loads
and zero outcomes. The consumed startup abort is the fifth startup event and
also produced zero model loads and zero outcomes.

## Prospective v2 boundary

V2 is not a retry of v1. It is a separate protocol, schedule, authorization,
attempt-ledger, run, result, CLI, adapter, schema, domain-separation, and output
namespace universe. It was designed after an outcome-free startup failure and
before any local-model output existed. Outcome selection is therefore
impossible: there was no v1 model outcome available to inspect, tune against,
accept, reject, or suppress.

The independently validated corpus and context content may remain substantively
the same, but v2 uses new seeds, transform/compiler versions, protocol identity,
schedule ordering and identity, observation IDs, schemas, package identity, and
authorization bindings. V2 remains a provider-independent local
development/calibration study, excludes TypeSafe/AgentTrace held-out evidence
and cloud access, and authorizes no spend.

## Normative JSON framing

Distill-native structured identities use RFC 8785/JCS. The inherited frozen
model aggregate identity alone retains its separately documented legacy
sorted/indented serializer.

- On-disk canonical JSON is one UTF-8 JCS value followed by exactly one LF.
  Missing LF, multiple LF, CRLF, trailing whitespace, prose, a second value,
  leading whitespace, duplicate or nested duplicate keys, nonfinite numbers,
  out-of-range exponents, and a wrong top-level type fail closed.
- Stdin/stdout transport frames are one UTF-8 JCS object followed by exactly one
  transport LF. Only the framing layer removes that LF; the payload parser
  receives LF-free bytes and rejects all other leading or trailing bytes.
- JSONL is one canonical object per line with one LF after every record,
  including the final record.

Go and Python implement separate file and frame readers. A cross-language
known-answer conformance receipt covers valid vectors and every adversarial
class before adapter-check or authorization can succeed.

## Adapter-check and attempt boundary

`adapter-check` is mandatory before authorization. It runs the exact committed
v2 adapter under `env -i` semantics, `python -I -B -S`, offline cache flags, and
the deny-network sandbox. It validates source commit/tree, binary build
provenance, adapter, interpreter, runtime, model, package, protocol, schedule,
host preflight, all input file hashes, and an absent path-bound run namespace.
It executes Go/Python framing conformance, emits a content-addressed owner-only
receipt with `model_loaded=false` and `network_allowed=false`, and exits without
importing MLX, calling `load()`, creating the run namespace, or writing model
caches or bytecode.

Authorization reconstructs every binding and rejects a copied check directory,
stale receipt, namespace reuse, or any source, binary, adapter, interpreter,
runtime, model, package, protocol, schedule, or host-identity drift. A failed
adapter-check creates neither an authorization directory nor an attempt ledger.
The successful check directory contains its own locked, path-bound use ledger;
authorization consumes it exactly once to prevent receipt replay.

Run startup repeats all fail-closed validation and starts the adapter in a
preload handshake. The preload process verifies the copied canonical package,
authorization, adapter-check, model, runtime, source, and output bindings and
returns `model_loaded=false`. Only after that handshake succeeds does the Go
runner create the private adapter workspace, append and sync the sole attempt
entry, and issue the explicit load frame. Any failure after ledger consumption
is a terminal preserved abort; no outcome-dependent retry is allowed. Any
failure before consumption may be corrected only while the receipt and all
bound inputs remain valid and no attempt exists.

Filesystem snapshot rollback by a malicious owner remains outside the
executable threat model. Independent custody must preserve the adapter-check,
authorization ledger, and run namespace through review.

## Result contract

Any public v2 result must disclose the v1 invalid boundary, four
pre-authorization wrapper failures, one consumed startup abort, zero v1
observations, and zero v1 model loads. It must state that v2 was prospective
before model output and that outcome selection was impossible. Absolute
performance remains conditioned on the frozen quiescent host, AC power,
memory, swap, disk, unrelated-process, sleep-inhibition, and deny-network
requirements.
