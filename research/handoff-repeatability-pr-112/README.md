# Handoff PR #112 repeatability study

Status: completed valid negative result on 2026-09-30. This is a controlled
internal retrospective, not an external-adoption or general-quality claim.

## Result

Three fresh GitHub Copilot CLI invocations with explicitly selected
`gpt-5.6-sol` each produced one canonical, verifier-valid `no_decision`
proposal. All three missed all four frozen expected decisions. The outputs
agreed on outcome and empty candidate set, but no pair was byte-identical
because each reason differed.

Trusted verification establishes protocol validity and binding to the frozen
request. It does not establish substantive correctness. Each run was therefore
classified as a valid-protocol false negative. The study contains three
negative runs and zero protocol-invalid runs.

## Frozen design

The [protocol](protocol.md) was written outside every agent workspace before
the first proposal invocation. Its SHA-256 is
`d806f360fb4d844cf0257bc8d6a23398d45dbdcc7ed30d42706c3e368ac35611`.

- Baseline includes merge commit
  `a15a8462395cde8ea3b55dd2d367a7d89baf0497`.
- Corpus: `testdata/handoff-v0/dogfood-pr-112`.
- Request ID:
  `13acba266c76aaa8c9a36603ef00bbb1a043f8c052de6d97d83b0924a0828807`.
- Tester-retained request SHA-256:
  `3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35`.
- Frozen expected decisions: 4.
- Condition C1: GitHub Copilot CLI 1.0.87-0, `gpt-5.6-sol`, three runs.
- Tools exposed to the model: `view` and `apply_patch`.
- Prompt: the unchanged strict file-output prompt in
  `docs/handoff-pilot-kit.md`.

Claude Code 2.1.222 was installed but unauthenticated. No Ollama or LM Studio
CLI/runtime was installed. No alternative condition met the frozen constraints,
so none was executed.

Each run began in a fresh private disposable parent containing only `request/`
copied from the canonical bundle and an empty `output/`. The canonical request,
retained digest, expected control, repository, prior outputs, and verifier
results were outside the model workspace and unavailable to its file tools.
Runtime-created `.agent-traces` stayed in the disposable parent. They were not
retained. Each parent was removed after the proposal and mutation evidence were
retained privately.

## Per-run results

| Run | Session | CLI elapsed | Runtime session | API time | Reported output tokens | Proposal SHA-256 | Verification | Outcome | Candidates | Recall | Review |
|---|---|---:|---:|---:|---:|---|---|---|---:|---:|---:|
| 1 | `a2fa56e2-d689-45d3-a9c6-b7ccf5c55445` | 36.236 s | 32.446 s | 18.460 s | 1,220 | `05f0240e716bfb47d46129d8091c2f72d951f2a6b7b31cbbff761b2c5b5965fb` | valid | `no_decision` | 0 | 0/4 | 19 s |
| 2 | `9ad26896-6832-4b13-8f9a-c3df13747c4c` | 35.598 s | 32.593 s | 24.567 s | 1,280 | `a3049889e1412dcf77c395ee6186e4c2ab886e110482240bb86cd40db1c551d1` | valid | `no_decision` | 0 | 0/4 | 20 s |
| 3 | `9263883f-4fb6-4aea-9222-04acde3a36bb` | 44.003 s | 40.786 s | 28.171 s | 1,800 | `c30d7777cd8462c48c875acfb483359f1f081b8081290f4ef5263ee88bcb61e8` | valid | `no_decision` | 0 | 0/4 | 21 s |

The runtime directly reported per-message output tokens, API duration, session
duration, and zero premium requests. It did not report input-token counts or
monetary cost, so neither is estimated here.

All candidate ID sets were empty. Each proposal supplied no evidence and no
patch. Exact-evidence validity was therefore `none supplied`, and patch
disposition was `not supplied`. Each reason made one unsupported conclusion
that no committed decision required a target update. That conclusion conflicts
with all four frozen expected decisions.

## Repeatability and burden

| Measure | Result |
|---|---:|
| Byte-identical output pairs | 0/3, 0% |
| Outcome agreement | 3/3 pairs, 100% |
| Candidate-set agreement | 3/3 pairs, 100% |
| Verification success | 3/3 runs, 100% |
| Decision 1 selection frequency | 0/3, 0% |
| Decision 2 selection frequency | 0/3, 0% |
| Decision 3 selection frequency | 0/3, 0% |
| Decision 4 selection frequency | 0/3, 0% |
| Review burden | 19 s, 20 s, 21 s |
| Median review burden | 20 s |
| Review burden range | 19-21 s |

The stable empty candidate set is repeatable abstention, not repeatable
decision recovery.

## Integrity and mutations

Every run created exactly one file under `output/`. All three request copies
remained byte-identical to the canonical request tree. After the three runs,
the canonical request still had SHA-256
`3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35`.

| Boundary | Run 1 | Run 2 | Run 3 | Aggregate |
|---|---:|---:|---:|---:|
| Request-copy mutations | 0 | 0 | 0 | 0 |
| Canonical-request mutations | 0 | 0 | 0 | 0 |
| Agent-caused source-repository mutations | 0 | 0 | 0 | 0 |
| Trust-boundary mutations | 0 | 0 | 0 | 0 |

The retained public artifacts contain no request bundles, traces, credentials,
absolute paths, machine identifiers, or estimated costs.

## Reproduce verification

The three exact sanitized proposals are under
[`proposals/`](proposals/). Recreate the canonical request and verify each
proposal using the tester-held digest:

```bash
make build
work="$(cd "$(mktemp -d)" && pwd -P)"

prepare_output="$(./distill handoff prepare \
  --conversation testdata/handoff-v0/dogfood-pr-112/conversation.md \
  --docs testdata/handoff-v0/dogfood-pr-112/docs \
  --out "$work/request")"
request_sha256="$(printf '%s\n' "$prepare_output" |
  sed -n 's/.* request_sha256=\([0-9a-f]*\) .*/\1/p')"
test "$request_sha256" = \
  "3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35"

for proposal in research/handoff-repeatability-pr-112/proposals/*.json; do
  name="$(basename "$proposal" .json)"
  ./distill handoff verify \
    --request "$work/request/handoff.request.json" \
    --expected-request-sha256 "$request_sha256" \
    --proposal "$proposal" \
    --out "$work/review-$name"
done
```

The compact aggregate is in [`results.json`](results.json).

## Limitations

This study uses one public documentation PR, one hosted model, three
invocations, one runtime version, and one reviewer. It measures observed
repeatability and review burden for this frozen task only. With n=3 it supports
no significance, superiority, adoption, or general-quality claim.
