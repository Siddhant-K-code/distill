# Handoff positive-negative capability control

Status: completed uncontaminated study with one invalid run on 2026-09-30.
This is a diagnostic on two checked-in fixtures, not a significance,
superiority, adoption, or general-quality claim.

## Result

The diagnostic classification is `construction_bottleneck_observed`.

No positive run constructed a candidate covering the expected retry decision.
Positive runs 1 and 3 nevertheless stated in their direct CLI responses that a
retry decision was present before each wrote a verifier-valid `no_decision`.
Positive run 2 exited successfully without writing any output. It remains an
invalid unretried run.

All three negative runs wrote verifier-valid `no_decision` proposals with no
false-positive candidate. Across all six runs there were zero fabricated
positive candidates and zero request-copy, canonical-request, source,
repository, or trust-boundary mutations.

The full capability threshold did not pass because it required at least 2 of 3
positive runs to produce a verifier-valid candidate covering the expected
retry decision. The observed result was 0 of 3.

## Frozen design

The exact [protocol](protocol.md) was written outside every model workspace
before the first invocation. Its SHA-256 is
`adf2730663c1c6ecd3c0fe7a7567c886b923efa29dc8c2c3536f31089e2bd94b`.
The exact unchanged [Pilot Kit prompt](prompt.txt) has SHA-256
`ac2d97ead6d5e97e8134477ad5e4a358a87fb12479c85846325639ea7901e25c`.

- Baseline:
  `167212e42e360fadc8b17e8f079a2b4f0f7e3411`, including compatible
  dependency merge `b5b246a`.
- Runtime: GitHub Copilot CLI 1.0.90-5.
- Runtime executable SHA-256:
  `a21a8624194aabc709cdd8ec564b653efb0d771057607b040047e9a1c569a25b`.
- Model: `gpt-5.6-sol`, selected explicitly before every run.
- Available model tools: exactly `view` and `apply_patch`.
- Custom instructions and built-in MCP servers: disabled.
- Conditions: three positive `retry-policy` runs and three negative
  `brainstorming` runs.
- Fixed order: `positive-1`, `negative-1`, `positive-2`, `negative-2`,
  `positive-3`, `negative-3`.
- Retries, repairs, terminal-object extraction, and proposal edits: zero.

Each invocation started in a fresh mode-0700 disposable parent containing only
a copied request and an empty output directory. Canonical requests,
tester-retained digests, expected proposals, the repository, protocol, prior
outputs, logs, and verifier results remained outside model file access.

The positive request SHA-256 was
`cd32864f9e42b44e0341bb9199dabb28a032103b8e2d9e9b4f4c19d61d3aea38`.
The negative request SHA-256 was
`45d333595c146fcd8ade578f6015258dda8c57275b75352d63ca08f8c89fb4d2`.
Both were captured independently from Handoff preparation and reproduced
during validation.

## Per-run results

All candidate ID sets were empty. `Valid` means JSON, schema, canonical
encoding, request identity, and trusted Handoff verification all passed.

| Order | Run | Session | CLI ms | Files | Proposal SHA-256 | Valid | Outcome | Control score | Mutations |
|---:|---|---|---:|---:|---|---|---|---|---|
| 1 | `positive-1` | `a810cde0-274e-49c2-9438-46cbdcfe9f8f` | 56,760 | 1 | `a91a516a86bec61fd49b67f621b6e6f8f6b155f52b4afe03582181d1d9b4725e` | yes | `no_decision` | missed; extraction text present | 0/0/0/0 |
| 2 | `negative-1` | `4706dc43-978f-4c05-aad9-28e9adc1a340` | 20,969 | 1 | `8d798a659e9ddd8a9918546c8bfddfb594e568034b7eb4bd66d78f62a51fea38` | yes | `no_decision` | correct abstention | 0/0/0/0 |
| 3 | `positive-2` | `ee7e0fb9-8583-4810-8c37-0825d16903bb` | 10,867 | 0 | none | no | `missing_output` | missed; invalid | 0/0/0/0 |
| 4 | `negative-2` | `04478d9f-f6cb-44d8-a9d4-6e9f100e7b91` | 22,316 | 1 | `99c7095d0704cbdd5d30129bf2144c3da5b495ab4656838c5d47ab5ef783b5aa` | yes | `no_decision` | correct abstention | 0/0/0/0 |
| 5 | `positive-3` | `ad53a13a-308c-4321-b942-f49dba9d8d86` | 45,895 | 1 | `ba21081523bebfecab3ece02c725733acbff24778fc1129c77659f2c7deae770` | yes | `no_decision` | missed; extraction text present | 0/0/0/0 |
| 6 | `negative-3` | `9f580a4f-b256-476e-a08b-298767ec366e` | 44,775 | 1 | `2064d418cf6d13dc125d39843cc5fcdecaa2e43f1d5728ec6155fcf0b50e1fc0` | yes | `no_decision` | correct abstention | 0/0/0/0 |

Mutation columns are request copy, canonical request, source repository, and
trust boundary. The runtime created its normal disposable trace directory.
That runtime-owned directory contained no protected study material, was not a
model output or trust-boundary mutation, and was deleted with each workspace.

The runtime directly reported these usage fields. `Input` includes cached
input. `Uncached` and `cache read` are reported separately. Premium cost and
nano-AIU are provider accounting fields, not inferred monetary cost.

| Run | Session ms | API ms | Calls | Input | Uncached | Cache read | Output | Reasoning | Premium cost | Nano-AIU | Review ms |
|---|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|---:|
| `positive-1` | 55,322 | 47,122 | 5 | 28,666 | 10,746 | 17,920 | 2,991 | 2,168 | 1 | 10,997,200,000 | 2 |
| `negative-1` | 18,812 | 13,631 | 5 | 26,673 | 8,753 | 17,920 | 924 | 152 | 1 | 6,066,000,000 | 2 |
| `positive-2` | 8,845 | 5,244 | 1 | 3,527 | 3,527 | 0 | 56 | 0 | 1 | 1,522,800,000 | 1 |
| `negative-2` | 20,825 | 15,581 | 6 | 32,857 | 8,793 | 24,064 | 1,061 | 280 | 1 | 6,601,760,000 | 1 |
| `positive-3` | 44,589 | 39,180 | 5 | 30,183 | 10,215 | 19,968 | 2,624 | 1,713 | 1 | 10,132,720,000 | 2 |
| `negative-3` | 43,385 | 38,048 | 5 | 28,583 | 8,615 | 19,968 | 1,108 | 246 | 1 | 6,460,720,000 | 1 |

No monetary cost was directly reported, so none is estimated. Review timing
covers deterministic trusted scoring against the checked-in expected
proposal. No positive patch was supplied for a qualitative patch disposition.

## Positive-control scoring

| Measure | Result |
|---|---:|
| Verifier-valid proposals | 2/3 |
| Expected retry decisions recovered | 0/3 |
| Runs with positive extraction text | 2/3 |
| Invalid runs | 1/3 |
| Fabricated candidates | 0 |
| Unsupported proposal claims | 1 |
| Exact evidence supplied | 0/3 |
| Correct target supplied | 0/3 |
| Valid candidate ID supplied | 0/3 |
| Valid patch supplied | 0/3 |
| Semantically equivalent patch supplied | 0/3 |
| Byte-identical output agreement | 0/3 pairs |
| Outcome agreement | 1/3 pairs |
| Candidate-set agreement | 1/3 pairs |
| Review time | 2 ms, 1 ms, 2 ms |
| Median and range | 2 ms; 1-2 ms |

Positive run 1 made one unsupported claim that the required derived digest
values could not be computed from the bundle. Positive runs 1 and 3 supplied
no evidence, target, candidate ID, or patch, so each corresponding disposition
is `not_supplied`. Positive run 2 supplied no proposal at all.

## Negative-control scoring

| Measure | Result |
|---|---:|
| Verifier-valid `no_decision` | 3/3 |
| Correct abstention | 3/3 |
| False-positive candidates | 0 |
| Invalid runs | 0/3 |
| Byte-identical output agreement | 0/3 pairs |
| Outcome agreement | 3/3 pairs |
| Candidate-set agreement | 3/3 pairs |
| Review time | 2 ms, 1 ms, 1 ms |
| Median and range | 1 ms; 1-2 ms |

## Threshold and classification

| Frozen requirement | Required | Observed | Passed |
|---|---:|---:|---|
| Positive verifier-valid expected candidate | 2/3 | 0/3 | no |
| Negative verifier-valid `no_decision` | 2/3 | 3/3 | yes |
| Fabricated positive candidates | 0 | 0 | yes |
| Negative false-positive candidates | 0 | 0 | yes |
| Trust-boundary mutations | 0 | 0 | yes |

The full capability threshold failed only its positive-candidate requirement.
The next frozen classification rule applies because positive extraction text
appeared but no positive run completed candidate construction. The diagnostic
classification is therefore `construction_bottleneck_observed`.

## Retained evidence and reproduction

The compact aggregate is [results.json](results.json). Five exact raw proposal
files are retained under [proposals/](proposals/). No sixth proposal exists:
`positive-2` wrote zero output files, and the study forbade reconstruction from
terminal text.

To reproduce both request identities and independently verify every retained
proposal:

```bash
make build
work="$(mktemp -d)"

positive_prepare="$(./distill handoff prepare \
  --conversation testdata/handoff-v0/retry-policy/conversation.md \
  --docs testdata/handoff-v0/retry-policy/docs \
  --out "$work/positive-request")"
negative_prepare="$(./distill handoff prepare \
  --conversation testdata/handoff-v0/brainstorming/conversation.md \
  --docs testdata/handoff-v0/brainstorming/docs \
  --out "$work/negative-request")"

positive_digest="$(printf '%s\n' "$positive_prepare" |
  sed -n 's/.* request_sha256=\([0-9a-f]*\) .*/\1/p')"
negative_digest="$(printf '%s\n' "$negative_prepare" |
  sed -n 's/.* request_sha256=\([0-9a-f]*\) .*/\1/p')"

test "$positive_digest" = \
  "cd32864f9e42b44e0341bb9199dabb28a032103b8e2d9e9b4f4c19d61d3aea38"
test "$negative_digest" = \
  "45d333595c146fcd8ade578f6015258dda8c57275b75352d63ca08f8c89fb4d2"

for proposal in research/handoff-capability-control/proposals/positive-*.json; do
  name="$(basename "$proposal" .json)"
  ./distill handoff verify \
    --request "$work/positive-request/handoff.request.json" \
    --expected-request-sha256 "$positive_digest" \
    --proposal "$proposal" \
    --out "$work/review-$name"
done

for proposal in research/handoff-capability-control/proposals/negative-*.json; do
  name="$(basename "$proposal" .json)"
  ./distill handoff verify \
    --request "$work/negative-request/handoff.request.json" \
    --expected-request-sha256 "$negative_digest" \
    --proposal "$proposal" \
    --out "$work/review-$name"
done
```

## Validation

The final evidence package passed:

- Fresh preparation reproduced both retained request digests exactly.
- A second independent pass verified all five retained proposals against the
  canonical request for their condition and the independently retained digest.
- `go test ./pkg/handoff ./cmd -count=1`.
- `make handoff-v0-demo`.
- Protocol, prompt, proposal, run-order, classification, aggregate, mutation,
  and missing-output assertions.
- `git diff --check`.
- Added-line scans for private paths, credentials, em dashes, and machine
  identifiers.
- Artifact-name scans for request bundles, runtime logs, and traces.

The first reproduction command used macOS's logical `/var` spelling and
Handoff rejected that symlinked path as designed. The successful validation
used the physical `pwd -P` path required by the Pilot Kit. This was not a model
invocation and did not alter the study.

## Limitations

This study uses two small public fixtures, one hosted model, one CLI build,
three runs per condition, and one trusted scorer. The positive control observed
two extraction-to-construction gaps and one missing-output failure, but it
cannot establish how often either occurs elsewhere. The review timings measure
deterministic scorer execution, not human review burden. The runtime initialized
configured plugin MCP servers, but the model tool allowlist excluded every tool
except `view` and `apply_patch`; both built-in MCP servers were disabled.

No Handoff product code, embedded instruction, schema, fixture, Pilot Kit, or
verifier was changed.
