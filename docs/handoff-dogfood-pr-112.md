# Handoff v0 internal dogfood: PR #112

Status: one controlled internal retrospective completed on 2026-09-30. This is
not an external-adoption claim.

## Result

The retained request verified successfully with zero request or source
mutations. The proposal was structurally valid but returned `no_decision`, so
the reviewer rejected it after comparing it with four frozen expected
decisions.

The first attempt exposed a runtime compatibility problem. GitHub Copilot CLI
created `.agent-traces` in its current working directory, which was the
canonical request directory. Verification correctly failed on that unexpected
entry. Capturing terminal output was also ambiguous because the response
contained two concatenated JSON objects and invalid literal newlines.

The compatibility fix does not weaken verification or change CLI behavior. The
canonical request remains private and immutable. The agent runs from a
disposable private parent containing:

```text
request/                  copied agent input
output/proposal.json      sole proposal capture
```

Runtime metadata may appear beside those paths. Trusted verification still uses
the retained canonical request, the tester-held digest, and the external
proposal path. It never verifies the agent's request copy.

## Frozen corpus

- Conversation: public [PR #112](https://github.com/Siddhant-K-code/distill/pull/112)
- Conversation snapshot SHA-256:
  `6b78ca03b7272dc3e204545ec87224726174a5bcf5e28f8af4e45926e633b996`
- Base revision:
  `f2e3b2fcba7084e728bd0a9718f86d23d53b880a`
- Head revision:
  `445df3f15623396805e157a59c32469d749588ed`
- Merge revision:
  `1795f8a919aed586b5630079dca1795825ffc751`
- Target: `docs/handoff-v0.md` from the base revision
- Target SHA-256:
  `d265069feb86500a6744f2fbf2d98f6caeb2fa895f8987bcc123ee45bc80ed0e`
- Frozen expected decisions: 4
- Expected-decision control SHA-256:
  `cedf67293300b539e26ca8107f525bc0e462e53e7f2ec0697591395b4e028f0c`

The conversation snapshot records the PR URL and immutable commit revisions.
GitHub PR text can change, so the checked-in snapshot bytes are the
reproducibility boundary.

## Agent and trust boundary

- Runtime: GitHub Copilot CLI 1.0.87-0
- Model: `gpt-5.6-sol`, selected explicitly
- Fresh session: `7115204f-01ff-4eb0-84de-1cc9e01361d2`
- Available tools: `view`, `apply_patch`
- Agent input: copied request plus an empty `output/` directory
- Excluded: trusted digest file, expected decisions, source repository,
  canonical request, and verifier output
- Request ID:
  `13acba266c76aaa8c9a36603ef00bbb1a043f8c052de6d97d83b0924a0828807`
- Tester-retained request SHA-256:
  `3562acf55eb949695679e1fb7e6fd8d7809c78b342951d4563a7e7ec0f58ad35`
- Proposal SHA-256:
  `02984139c791fbbf70528eb754e7f29e7c1401c657e267d7b706c1c26d9aa9f9`
- Receipt SHA-256:
  `c69a976a822c397ed7c8463e18c52908685b988884d17229645eb8d46fdea0ed`

The tester captured and supplied the trusted digest. The agent saw only the
non-authoritative value contained in its request copy. Runtime tracing created
metadata at the disposable workspace root, not in either request tree.

## Measurements

Objective measurements:

| Measure | Result |
|---|---|
| Expected committed decisions frozen before the agent run | 4 |
| Candidates found | 0 |
| Canonical request mutations | 0 |
| Agent request-copy mutations | 0 |
| Source mutations | 0 |
| Verification | Valid `no_decision`, `require_review` |
| Review time | 37 seconds, 0.62 minutes |

Reviewer judgment:

| Pilot Kit field | Result |
|---|---|
| Missed expected decisions | 4 |
| Fabricated or unsupported candidates | 0; no candidates were proposed |
| Exact evidence correctness | `none`; no evidence was supplied |
| Patch outcome | `rejected`; no patch was supplied |
| Trust boundary understood | `yes`; the tester supplied the retained digest |
| Highest-friction manual step | Detecting that the proposal incorrectly treated the pre-merge target as already containing the merged clarifications |

The proposal reason itself was incorrect. The frozen target did not lead with
the agent-independent workflow, did not contain the trust-boundary diagram or
copy-and-paste prompt, and had not received the planned simplification.

## Reproduce verification

The checked-in artifacts contain only public source material and sanitized
outputs. They do not contain a private request bundle, runtime traces, absolute
paths, or temporary metadata.

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

./distill handoff verify \
  --request "$work/request/handoff.request.json" \
  --expected-request-sha256 "$request_sha256" \
  --proposal testdata/handoff-v0/dogfood-pr-112/proposal.json \
  --out "$work/review"
diff -ru testdata/handoff-v0/dogfood-pr-112/review "$work/review"
```

## Limitations

This was one internal retrospective against one public documentation PR and one
hosted proposal model. It measures protocol compatibility and one model result,
not general decision-extraction quality. No external tester result, adoption,
or release-readiness conclusion follows from it.
