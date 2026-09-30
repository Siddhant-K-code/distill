# Frozen expected decisions for the PR #112 retrospective

This trusted control was written before the proposal-agent invocation. It must
not enter the Handoff request bundle or the proposal agent's working directory.

1. Lead the Handoff document with an agent-independent prepare, propose, verify,
   and human-review workflow plus a trust-boundary diagram.
   Rationale: the first PR summary bullet explicitly commits to this framing.
2. State that only the private request bundle enters the agent-controlled
   boundary. Retain the trusted prepare digest independently outside that
   boundary, and do not treat any matching value in the bundle or proposal as
   authoritative.
   Rationale: the second PR summary bullet and the immutable digest-wording
   correction commit establish this boundary.
3. Add a copy/paste proposal-agent prompt and tighten the offline fixture
   example around the tester-retained digest and local verification.
   Rationale: the third PR summary bullet and validation record commit to both.
4. Remove redundant protocol detail without losing the alpha guarantees,
   review-only behavior, private modes, failure boundary, or fixture guidance.
   Rationale: the fourth PR summary bullet explicitly defines the allowed
   simplification and the content that must remain.

Expected committed decision count: 4
