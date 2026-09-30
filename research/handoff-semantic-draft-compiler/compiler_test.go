package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Siddhant-K-code/distill/internal/artifact"
	"github.com/Siddhant-K-code/distill/pkg/handoff"
)

func TestCompileSuccess(t *testing.T) {
	fixture := prepareFixture(t, "retry-policy")
	draftPath := writeDraft(t, fixture.work, positiveDraft())

	result, err := compileSemanticDraft(fixture.requestPath, fixture.digest, draftPath)
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join(fixture.repoRoot, "testdata/handoff-v0/retry-policy/proposal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Proposal, expected) {
		t.Fatalf("compiled proposal differs from expected fixture:\n%s", result.Proposal)
	}
}

func TestCompileAbstention(t *testing.T) {
	fixture := prepareFixture(t, "brainstorming")
	draft := semanticDraft{
		SchemaVersion: semanticDraftSchemaVersion,
		Outcome:       "abstain",
		Reason:        "The conversation explicitly deferred the decision.",
		Candidate:     nil,
	}
	result, err := compileSemanticDraft(fixture.requestPath, fixture.digest, writeDraft(t, fixture.work, draft))
	if err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile(filepath.Join(fixture.repoRoot, "testdata/handoff-v0/brainstorming/proposal.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(result.Proposal, expected) {
		t.Fatalf("compiled abstention differs from expected fixture:\n%s", result.Proposal)
	}
}

func TestCompileRejectsMissingQuote(t *testing.T) {
	fixture := prepareFixture(t, "retry-policy")
	draft := positiveDraft()
	draft.Candidate.EvidenceQuote = "Decision: this quote does not exist."
	_, err := compileSemanticDraft(fixture.requestPath, fixture.digest, writeDraft(t, fixture.work, draft))
	assertErrorContains(t, err, "evidence quote is missing")
}

func TestCompileRejectsAmbiguousQuote(t *testing.T) {
	fixture := prepareFixture(t, "retry-policy")
	draft := positiveDraft()
	draft.Candidate.EvidenceQuote = "Operator:"
	_, err := compileSemanticDraft(fixture.requestPath, fixture.digest, writeDraft(t, fixture.work, draft))
	assertErrorContains(t, err, "evidence quote is ambiguous")
}

func TestCompileRejectsTargetMismatch(t *testing.T) {
	fixture := prepareFixture(t, "retry-policy")
	draft := positiveDraft()
	draft.Candidate.TargetAnchorQuote = "Production requests use five retries."
	_, err := compileSemanticDraft(fixture.requestPath, fixture.digest, writeDraft(t, fixture.work, draft))
	assertErrorContains(t, err, "target anchor quote is missing")
}

func TestCompileRejectsUnsafePathAndOperation(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*semanticDraft)
		want   string
	}{
		{
			name: "unsafe path",
			mutate: func(draft *semanticDraft) {
				draft.Candidate.TargetPath = "../retries.md"
			},
			want: "target_path",
		},
		{
			name: "unsupported operation",
			mutate: func(draft *semanticDraft) {
				draft.Candidate.Operation = "delete"
			},
			want: "semantic draft schema validation",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			fixture := prepareFixture(t, "retry-policy")
			draft := positiveDraft()
			test.mutate(&draft)
			_, err := compileSemanticDraft(
				fixture.requestPath, fixture.digest, writeDraft(t, fixture.work, draft),
			)
			assertErrorContains(t, err, test.want)
		})
	}
}

func TestCompileRejectsMalformedDraft(t *testing.T) {
	fixture := prepareFixture(t, "retry-policy")
	draftPath := filepath.Join(fixture.work, "malformed.json")
	if err := os.WriteFile(draftPath, []byte("{"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := compileSemanticDraft(fixture.requestPath, fixture.digest, draftPath)
	assertErrorContains(t, err, "semantic draft JSON")
}

func TestCompileReplayIsByteIdentical(t *testing.T) {
	fixture := prepareFixture(t, "retry-policy")
	draftPath := writeDraft(t, fixture.work, positiveDraft())
	first, err := compileSemanticDraft(fixture.requestPath, fixture.digest, draftPath)
	if err != nil {
		t.Fatal(err)
	}
	for replay := 0; replay < 3; replay++ {
		next, replayErr := compileSemanticDraft(fixture.requestPath, fixture.digest, draftPath)
		if replayErr != nil {
			t.Fatal(replayErr)
		}
		if !bytes.Equal(first.CanonicalDraft, next.CanonicalDraft) {
			t.Fatal("canonical draft bytes changed during replay")
		}
		if !bytes.Equal(first.Proposal, next.Proposal) {
			t.Fatal("proposal bytes changed during replay")
		}
	}
}

func TestCompiledProposalPassesTrustedVerifier(t *testing.T) {
	fixture := prepareFixture(t, "retry-policy")
	result, err := compileSemanticDraft(
		fixture.requestPath, fixture.digest, writeDraft(t, fixture.work, positiveDraft()),
	)
	if err != nil {
		t.Fatal(err)
	}
	proposalPath := filepath.Join(fixture.work, "proposal.json")
	if err := os.WriteFile(proposalPath, result.Proposal, 0o600); err != nil {
		t.Fatal(err)
	}
	summary, err := handoff.VerifyWithExpectedRequest(
		fixture.requestPath, proposalPath, filepath.Join(fixture.work, "review"), fixture.digest,
	)
	if err != nil {
		t.Fatal(err)
	}
	if summary.CandidateCount != 1 || summary.Outcome != handoff.OutcomeCandidates {
		t.Fatalf("unexpected verification summary: %+v", summary)
	}
}

type preparedFixture struct {
	work        string
	repoRoot    string
	requestPath string
	digest      string
}

func prepareFixture(t *testing.T, name string) preparedFixture {
	t.Helper()
	repoRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	work, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	fixtureRoot := filepath.Join(repoRoot, "testdata", "handoff-v0", name)
	summary, err := handoff.Prepare(
		filepath.Join(fixtureRoot, "conversation.md"),
		filepath.Join(fixtureRoot, "docs"),
		filepath.Join(work, "request"),
	)
	if err != nil {
		t.Fatal(err)
	}
	return preparedFixture{
		work:        work,
		repoRoot:    repoRoot,
		requestPath: filepath.Join(work, "request", handoff.RequestFileName),
		digest:      summary.RequestSHA256,
	}
}

func positiveDraft() semanticDraft {
	return semanticDraft{
		SchemaVersion: semanticDraftSchemaVersion,
		Outcome:       "candidate_intent",
		Reason:        "One committed retry-policy decision is ready for review.",
		Candidate: &candidateIntent{
			DecisionText:           "Decision: production requests will use three attempts with exponential backoff starting at 200ms.",
			DecisionType:           "policy",
			EvidenceQuote:          "Decision: production requests will use three attempts with exponential backoff starting at 200ms.",
			EvidenceOccurrence:     1,
			TargetPath:             "runbook/retries.md",
			TargetAnchorQuote:      "Production requests use two immediate retries.",
			TargetAnchorOccurrence: 1,
			Operation:              "replace",
			ReplacementText:        "Production requests use three attempts with exponential backoff starting at 200ms.",
			Rationale:              "The patch is only a candidate and requires a human reviewer.",
		},
	}
}

func writeDraft(t *testing.T, directory string, draft semanticDraft) string {
	t.Helper()
	data, err := artifact.CanonicalJSON(draft)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, strings.ReplaceAll(t.Name(), "/", "-")+".json")
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func assertErrorContains(t *testing.T, err error, want string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected error containing %q", want)
	}
	if !strings.Contains(err.Error(), want) {
		t.Fatalf("error %q does not contain %q", err, want)
	}
}
