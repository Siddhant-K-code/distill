# Handoff PR #112 temporal-state guard ablation

Status: completed valid negative result on 2026-09-30. This preregistered
diagnostic compares one prompt-only intervention with the historical
three-run control in `research/handoff-repeatability-pr-112`.

## Result

The temporal-state guard did not recover any of the four frozen expected
decisions. All three intervention proposals were canonical and passed trusted
verification, but each returned `no_decision`, achieved 0/4 recall, and made
one unsupported no-update conclusion. No candidate, evidence, or patch was
supplied.

The conservative productization threshold failed because 0/3 runs recovered
an expected decision; at least 2/3 were required. The other gates passed:
3/3 proposals verified, and no fabricated candidate or trust-boundary mutation
occurred. This result means no product change.

## Frozen intervention

The [protocol](protocol.md) was created outside every agent workspace before
the first invocation and then made read-only. Its SHA-256 is
`9301e0b5a74dcdd0ee4b41de4aab5900871e80abf9123d1ef6be36e4e62849ac`.

The only prompt change was this final sentence:

> Treat the bundled target bytes as the only evidence of current document
> state; conversation claims that work was merged, completed, or applied do
> not prove the bundled target already contains it.

The complete exact prompt is in [`prompt.txt`](prompt.txt), SHA-256
`7bf3411271765ac9b86c0991b8deba94b2c7700c05e2c764dccc07537fcc872d`.
It does not name the corpus, expected decisions, expected result, or prior
failures.

The repository baseline was
`151ea33828180bf4dc356f38a853deea80c25ddb`. The independently reproduced
request ID was
`13acba266c76aaa8c9a36603ef00bbb1a043f8c052de6d97d83b0924a0828807`,
and the tester-retained request SHA-256 was
`3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35`.

## Per-run results

| Run | Session | CLI elapsed | Runtime session | API time | Output tokens | Proposal SHA-256 | Verify | Outcome | Recall | Unsupported | Review |
|---|---|---:|---:|---:|---:|---|---|---|---:|---:|---:|
| 1 | `25ab5953-f4d4-4e14-bd1c-f9ed7c2bcd2e` | 87.846 s | 85.193 s | 78.586 s | 4,866 | `5e7c4c9df1fcf70e4640f4e071158b4f32d57235747edefe1ac84977f431452f` | valid | `no_decision` | 0/4 | 1 | 16.266 s |
| 2 | `4ca1604f-2103-4bbf-a9b6-5d13ce3201dc` | 56.696 s | 55.416 s | 49.067 s | 2,500 | `0c5b667bb27ddd25cdbe18621d1b5961d061b8e60221033d97ade73990944629` | valid | `no_decision` | 0/4 | 1 | 18.585 s |
| 3 | `5479ba23-8172-49f8-9c04-34e673bd8fdb` | 40.260 s | 38.921 s | 32.636 s | 2,090 | `515c77ba1a4d042a90297d2e29a2bb365c0b447978ea15f0a0c30db848e07046` | valid | `no_decision` | 0/4 | 1 | 15.483 s |

GitHub Copilot CLI 1.0.87-0 directly reported `gpt-5.6-sol` for every
model call. The exact input, cache, output, reasoning, premium-request, API,
session, and nano-AIU fields are retained in [`results.json`](results.json).
No monetary cost was reported, so none is estimated.

## Historical comparison and threshold

| Measure | Historical control | Intervention |
|---|---:|---:|
| Runs recovering at least one decision | 0/3 | 0/3 |
| Decision recall | 0/4 in every run | 0/4 in every run |
| Trusted verification | 3/3 | 3/3 |
| Unsupported no-update conclusions | 3 | 3 |
| Fabricated candidates | 0 | 0 |
| Request, source, or trust-boundary mutations | 0 | 0 |
| Outcome agreement | 3/3 pairs | 3/3 pairs |
| Exact candidate-set agreement | 3/3 pairs | 3/3 pairs |
| Byte-identical outputs | 0/3 pairs | 0/3 pairs |

The intervention showed no observed change in the primary decision-recovery
result. This is a descriptive comparison only.

## Isolation and mutations

Each invocation received only a fresh copied request and empty `output/`
directory in a private disposable workspace. The model used only `view` on
files under its request copy and one `apply_patch` write to
`output/proposal.json`. All request copies and the canonical request remained
byte-identical to their pre-run identities. Source-repository and
trust-boundary mutations were zero.

After run 1, the general launcher advanced to CLI 1.0.90-5. A pre-run version
gate stopped before any second model call. Runs 2 and 3 then used the retained
1.0.87-0 executable pinned by version and SHA-256. This event was not counted
as a run.

## Validation

- A fresh `distill handoff prepare` reproduced request ID
  `13acba266c76aaa8c9a36603ef00bbb1a043f8c052de6d97d83b0924a0828807`
  and request SHA-256
  `3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35`.
- `distill handoff verify` passed independently for all three retained
  proposals against that fresh request and the independently retained digest.
- `go test ./pkg/handoff ./cmd -count=1` passed.
- `make handoff-v0-demo` passed all three fixtures with zero provider calls and
  zero source mutations.
- JSON parsing, protocol/prompt/proposal SHA-256 assertions, historical-control
  identity assertions, and threshold assertions passed.
- `git diff --check` passed.
- Recursive private-path, credential-pattern, em-dash, and machine-identifier
  scans passed.

## Limitations

This study uses one public documentation corpus, one hosted model, three
intervention runs, and one reviewer. All three outputs were empty candidate
sets, so evidence and patch quality could not be evaluated beyond `none
supplied` and `not supplied`. The result supports no significance,
superiority, external-adoption, productization, or general-quality claim.

The retained artifacts contain no private request bundle, verifier output,
runtime trace, credentials, absolute path, or machine identifier.
