package handoff

import (
	"bytes"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Siddhant-K-code/distill/internal/artifact"
)

func TestPrepareAndVerifyReviewPackage(t *testing.T) {
	root := copyPublicFixture(t, "retry-policy")
	requestDirectory := filepath.Join(root, "request")
	prepared, err := Prepare(
		filepath.Join(root, "conversation.md"),
		filepath.Join(root, "docs"),
		requestDirectory,
	)
	if err != nil {
		t.Fatal(err)
	}
	if prepared.DocumentCount != 1 || !artifact.ValidDigest(prepared.RequestID) {
		t.Fatalf("unexpected prepare summary: %+v", prepared)
	}

	proposalPath := filepath.Join(root, "proposal.json")
	proposal := retryProposal(t, requestDirectory)
	writeCanonical(t, proposalPath, proposal)
	sourceBefore := mustRead(t, filepath.Join(root, "docs", "runbook", "retries.md"))

	reviewDirectory := filepath.Join(root, "review")
	verified, err := Verify(
		filepath.Join(requestDirectory, RequestFileName),
		proposalPath,
		reviewDirectory,
	)
	if err != nil {
		t.Fatal(err)
	}
	if verified.CandidateCount != 1 || verified.Outcome != OutcomeCandidates ||
		!artifact.ValidDigest(verified.ReceiptSHA256) {
		t.Fatalf("unexpected verify summary: %+v", verified)
	}
	if after := mustRead(t, filepath.Join(root, "docs", "runbook", "retries.md")); !bytes.Equal(after, sourceBefore) {
		t.Fatal("verify mutated the source docs")
	}

	candidate := proposal.Candidates[0]
	expectedFiles := []string{
		ChecksumsFileName,
		ReceiptFileName,
		"patches/" + candidate.ID + ".patch",
		ReviewFileName,
	}
	if actual := fileNames(t, reviewDirectory); !reflect.DeepEqual(actual, expectedFiles) {
		t.Fatalf("review files = %#v, want %#v", actual, expectedFiles)
	}
	if patch := mustRead(t, filepath.Join(reviewDirectory, "patches", candidate.ID+".patch")); string(patch) != candidate.Patch.UnifiedDiff {
		t.Fatal("published patch bytes differ from the proposal")
	}
	review := string(mustRead(t, filepath.Join(reviewDirectory, ReviewFileName)))
	for _, required := range []string{
		"Review required", candidate.ID, candidate.DecisionText,
		candidate.Evidence.Quote, candidate.Patch.SHA256, RequireReview,
	} {
		if !strings.Contains(review, required) {
			t.Fatalf("review does not contain %q", required)
		}
	}
}

func TestNoDecisionProposal(t *testing.T) {
	root := copyPublicFixture(t, "brainstorming")
	requestDirectory := filepath.Join(root, "request")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	bundle, requestBytes, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName))
	if err != nil {
		t.Fatal(err)
	}
	proposal := Proposal{
		SchemaVersion: ProposalSchemaVersion,
		RequestID:     bundle.request.RequestID,
		RequestSHA256: artifact.DigestBytes(requestBytes),
		Outcome:       OutcomeNoDecision,
		Route:         RequireReview,
		Reason:        "The conversation explicitly deferred the decision.",
		Candidates:    []Candidate{},
	}
	proposalPath := filepath.Join(root, "proposal.json")
	writeCanonical(t, proposalPath, proposal)
	output := filepath.Join(root, "review")
	if _, err := Verify(filepath.Join(requestDirectory, RequestFileName), proposalPath, output); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(strings.Join(fileNames(t, output), "\n"), "patches/") {
		t.Fatal("no-decision package unexpectedly contains a patch")
	}
	if review := string(mustRead(t, filepath.Join(output, ReviewFileName))); !strings.Contains(review, "No committed decision found") {
		t.Fatal("no-decision review is not explicit")
	}
}

func TestRequestAndReviewAreDeterministicAcrossCreationOrderAndCWD(t *testing.T) {
	first := createDeterminismInput(t, []string{"zeta.md", "alpha.md"})
	second := createDeterminismInput(t, []string{"alpha.md", "zeta.md"})
	firstRequest := filepath.Join(first, "request")
	secondRequest := filepath.Join(second, "request")

	originalCWD, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(originalCWD) })
	if err := os.Chdir(first); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare("conversation.md", "docs", firstRequest); err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(second); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare("conversation.md", "docs", secondRequest); err != nil {
		t.Fatal(err)
	}
	assertTreesEqual(t, firstRequest, secondRequest)

	firstProposal := determinismProposal(t, firstRequest)
	secondProposal := determinismProposal(t, secondRequest)
	firstProposalPath := filepath.Join(first, "proposal.json")
	secondProposalPath := filepath.Join(second, "proposal.json")
	writeCanonical(t, firstProposalPath, firstProposal)
	writeCanonical(t, secondProposalPath, secondProposal)
	if !bytes.Equal(mustRead(t, firstProposalPath), mustRead(t, secondProposalPath)) {
		t.Fatal("equivalent proposals differ")
	}
	if _, err := Verify(filepath.Join(firstRequest, RequestFileName), firstProposalPath, filepath.Join(first, "review")); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filepath.Join(secondRequest, RequestFileName), secondProposalPath, filepath.Join(second, "review")); err != nil {
		t.Fatal(err)
	}
	assertTreesEqual(t, filepath.Join(first, "review"), filepath.Join(second, "review"))
}

func TestVerifyUsesFrozenBundleNotChangedSourceTree(t *testing.T) {
	root := copyPublicFixture(t, "retry-policy")
	requestDirectory := filepath.Join(root, "request")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	proposal := retryProposal(t, requestDirectory)
	proposalPath := filepath.Join(root, "proposal.json")
	writeCanonical(t, proposalPath, proposal)
	if err := os.WriteFile(
		filepath.Join(root, "docs", "runbook", "retries.md"),
		[]byte("# Changed after prepare\n"),
		0o644,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filepath.Join(requestDirectory, RequestFileName), proposalPath, filepath.Join(root, "review")); err != nil {
		t.Fatalf("verify must judge the frozen portable bundle: %v", err)
	}
}

func TestVerifyRejectsFabricatedEvidenceAndStaleTarget(t *testing.T) {
	tests := []struct {
		name   string
		mutate func(*Candidate)
		want   string
	}{
		{
			name: "fabricated quote",
			mutate: func(candidate *Candidate) {
				candidate.Evidence.Quote = "Decision: use unlimited retries."
			},
			want: "evidence quote",
		},
		{
			name: "stale target",
			mutate: func(candidate *Candidate) {
				candidate.Target.OriginalSHA256 = strings.Repeat("0", 64)
			},
			want: "target digest",
		},
		{
			name: "unsafe diff header",
			mutate: func(candidate *Candidate) {
				candidate.Patch.UnifiedDiff = strings.Replace(
					candidate.Patch.UnifiedDiff,
					"+++ b/runbook/retries.md",
					"+++ b/../outside.md",
					1,
				)
				candidate.Patch.SHA256 = artifact.DigestBytes([]byte(candidate.Patch.UnifiedDiff))
			},
			want: "patch headers",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			root := copyPublicFixture(t, "retry-policy")
			requestDirectory := filepath.Join(root, "request")
			if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
				t.Fatal(err)
			}
			proposal := retryProposal(t, requestDirectory)
			test.mutate(&proposal.Candidates[0])
			proposal.Candidates[0].ID = mustCandidateID(t, proposal.Candidates[0])
			proposalPath := filepath.Join(root, "proposal.json")
			writeCanonical(t, proposalPath, proposal)
			_, err := Verify(filepath.Join(requestDirectory, RequestFileName), proposalPath, filepath.Join(root, "review"))
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("got error %v, want containing %q", err, test.want)
			}
			if _, statErr := os.Lstat(filepath.Join(root, "review")); !os.IsNotExist(statErr) {
				t.Fatal("failed verify published a partial output")
			}
		})
	}
}

func TestVerifyRejectsAllowRouteAndNonCanonicalJSON(t *testing.T) {
	root := copyPublicFixture(t, "brainstorming")
	requestDirectory := filepath.Join(root, "request")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	bundle, requestBytes, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName))
	if err != nil {
		t.Fatal(err)
	}
	proposal := fmt.Sprintf(
		`{"schema_version":%q,"request_id":%q,"request_sha256":%q,"outcome":"no_decision","route":"allow","reason":"none","candidates":[]}`+"\n",
		ProposalSchemaVersion, bundle.request.RequestID, artifact.DigestBytes(requestBytes),
	)
	proposalPath := filepath.Join(root, "proposal.json")
	if err := os.WriteFile(proposalPath, []byte(proposal), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filepath.Join(requestDirectory, RequestFileName), proposalPath, filepath.Join(root, "review")); err == nil {
		t.Fatal("direct allow route was accepted")
	}

	canonical := Proposal{
		SchemaVersion: ProposalSchemaVersion, RequestID: bundle.request.RequestID,
		RequestSHA256: artifact.DigestBytes(requestBytes), Outcome: OutcomeNoDecision,
		Route: RequireReview, Reason: "No decision.", Candidates: []Candidate{},
	}
	data, err := artifact.CanonicalJSON(canonical)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(proposalPath, bytes.Replace(data, []byte("  \""), []byte("\t\""), 1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(filepath.Join(requestDirectory, RequestFileName), proposalPath, filepath.Join(root, "review-2")); err == nil ||
		!strings.Contains(err.Error(), "not canonical") {
		t.Fatalf("non-canonical proposal error = %v", err)
	}
}

func TestPrepareRejectsSymlinksUnsupportedFilesAndCollisions(t *testing.T) {
	t.Run("conversation symlink", func(t *testing.T) {
		root := realTempDir(t)
		mustMkdir(t, filepath.Join(root, "docs"))
		mustWrite(t, filepath.Join(root, "real.md"), "# Conversation\n")
		mustWrite(t, filepath.Join(root, "docs", "doc.md"), "# Doc\n")
		if err := os.Symlink(filepath.Join(root, "real.md"), filepath.Join(root, "conversation.md")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), filepath.Join(root, "out")); err == nil {
			t.Fatal("conversation symlink was accepted")
		}
	})
	t.Run("docs symlink", func(t *testing.T) {
		root := realTempDir(t)
		mustMkdir(t, filepath.Join(root, "docs"))
		mustWrite(t, filepath.Join(root, "conversation.md"), "# Conversation\n")
		mustWrite(t, filepath.Join(root, "outside.md"), "# Outside\n")
		if err := os.Symlink(filepath.Join(root, "outside.md"), filepath.Join(root, "docs", "doc.md")); err != nil {
			t.Skipf("symlink unavailable: %v", err)
		}
		if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), filepath.Join(root, "out")); err == nil {
			t.Fatal("docs symlink was accepted")
		}
	})
	t.Run("unsupported docs file", func(t *testing.T) {
		root := realTempDir(t)
		mustMkdir(t, filepath.Join(root, "docs"))
		mustWrite(t, filepath.Join(root, "conversation.md"), "# Conversation\n")
		mustWrite(t, filepath.Join(root, "docs", "doc.txt"), "text")
		if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), filepath.Join(root, "out")); err == nil {
			t.Fatal("unsupported docs file was accepted")
		}
	})
	t.Run("unsupported conversation file", func(t *testing.T) {
		root := realTempDir(t)
		mustMkdir(t, filepath.Join(root, "docs"))
		mustWrite(t, filepath.Join(root, "conversation.txt"), "# Conversation\n")
		mustWrite(t, filepath.Join(root, "docs", "doc.md"), "# Doc\n")
		if _, err := Prepare(filepath.Join(root, "conversation.txt"), filepath.Join(root, "docs"), filepath.Join(root, "out")); err == nil {
			t.Fatal("unsupported conversation file was accepted")
		}
	})
	t.Run("output collision", func(t *testing.T) {
		root := copyPublicFixture(t, "brainstorming")
		output := filepath.Join(root, "request")
		mustMkdir(t, output)
		if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), output); err == nil {
			t.Fatal("existing output was accepted")
		}
	})
}

func TestVerifyRejectsOutputInsideRequestBundle(t *testing.T) {
	root := copyPublicFixture(t, "brainstorming")
	requestDirectory := filepath.Join(root, "request")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	proposalPath := filepath.Join(root, "proposal.json")
	if _, err := Verify(
		filepath.Join(requestDirectory, RequestFileName),
		proposalPath,
		filepath.Join(requestDirectory, "review"),
	); err == nil || !strings.Contains(err.Error(), "outside the immutable request bundle") {
		t.Fatalf("inside-request output error = %v", err)
	}
	if _, _, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName)); err != nil {
		t.Fatalf("failed verify invalidated request bundle: %v", err)
	}
}

func TestVerifyRejectsOversizedProposalBeforeJSONParsing(t *testing.T) {
	root := copyPublicFixture(t, "brainstorming")
	requestDirectory := filepath.Join(root, "request")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	proposalPath := filepath.Join(root, "oversized.json")
	if err := os.WriteFile(proposalPath, bytes.Repeat([]byte{'x'}, (8<<20)+1), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(
		filepath.Join(requestDirectory, RequestFileName),
		proposalPath,
		filepath.Join(root, "review"),
	); err == nil || !strings.Contains(err.Error(), "8388608-byte limit") {
		t.Fatalf("oversized proposal error = %v", err)
	}
}

func TestTamperedRequestBundleFailsClosed(t *testing.T) {
	for _, path := range []string{ConversationPath, "inputs/docs/architecture/cache.md"} {
		t.Run(path, func(t *testing.T) {
			root := copyPublicFixture(t, "brainstorming")
			requestDirectory := filepath.Join(root, "request")
			if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(requestDirectory, filepath.FromSlash(path)), []byte("tampered\n"), 0o644); err != nil {
				t.Fatal(err)
			}
			if _, _, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName)); err == nil {
				t.Fatal("tampered request bundle was accepted")
			}
		})
	}
}

func TestUnexpectedEmptyRequestDirectoryFailsClosed(t *testing.T) {
	root := copyPublicFixture(t, "brainstorming")
	requestDirectory := filepath.Join(root, "request")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(requestDirectory, "unexpected"), 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName)); err == nil {
		t.Fatal("unexpected empty request directory was accepted")
	}
}

func TestOverlappingCandidatePatchesAreRejected(t *testing.T) {
	root := copyPublicFixture(t, "retry-policy")
	requestDirectory := filepath.Join(root, "request")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	proposal := retryProposal(t, requestDirectory)
	second := proposal.Candidates[0]
	second.DecisionText = "Use four attempts while reviewing the retry policy."
	second.Reason = "Conflicts with the first candidate and must be rejected."
	second.Patch.UnifiedDiff = strings.Replace(second.Patch.UnifiedDiff, "three attempts", "four attempts", 1)
	second.Patch.SHA256 = artifact.DigestBytes([]byte(second.Patch.UnifiedDiff))
	second.ID = mustCandidateID(t, second)
	proposal.Candidates = append(proposal.Candidates, second)
	sort.Slice(proposal.Candidates, func(i, j int) bool { return proposal.Candidates[i].ID < proposal.Candidates[j].ID })
	proposalPath := filepath.Join(root, "proposal.json")
	writeCanonical(t, proposalPath, proposal)
	if _, err := Verify(filepath.Join(requestDirectory, RequestFileName), proposalPath, filepath.Join(root, "review")); err == nil ||
		!strings.Contains(err.Error(), "overlaps") {
		t.Fatalf("overlap error = %v", err)
	}
}

func TestHandoffPackageHasNoNetworkOrProviderImports(t *testing.T) {
	entries, err := os.ReadDir(".")
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data := string(mustRead(t, entry.Name()))
		for _, forbidden := range []string{`"net"`, `"net/http"`, `pkg/embedding`, `pkg/retriever`, `pkg/pinecone`} {
			if strings.Contains(data, forbidden) {
				t.Fatalf("%s imports forbidden network/provider dependency %s", entry.Name(), forbidden)
			}
		}
	}
}

func TestPublicFixtureProposals(t *testing.T) {
	update := os.Getenv("UPDATE_HANDOFF_FIXTURES") == "1"
	fixtures := []struct {
		name     string
		proposal func(*testing.T, string) Proposal
	}{
		{name: "retry-policy", proposal: retryProposal},
		{name: "api-deprecation", proposal: apiDeprecationProposal},
		{name: "brainstorming", proposal: noDecisionProposal},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			root := copyPublicFixture(t, fixture.name)
			requestDirectory := filepath.Join(root, "request")
			if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
				t.Fatal(err)
			}
			proposal := fixture.proposal(t, requestDirectory)
			data, err := artifact.CanonicalJSON(proposal)
			if err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(fixtureSourcePath(t, fixture.name), "proposal.json")
			if update {
				if err := os.WriteFile(sourcePath, data, 0o644); err != nil {
					t.Fatal(err)
				}
			} else if actual := mustRead(t, sourcePath); !bytes.Equal(actual, data) {
				t.Fatalf("public proposal fixture is stale; run UPDATE_HANDOFF_FIXTURES=1 go test ./pkg/handoff -run TestPublicFixtureProposals")
			}
			proposalPath := filepath.Join(root, "proposal.json")
			if err := os.WriteFile(proposalPath, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(
				filepath.Join(requestDirectory, RequestFileName),
				proposalPath,
				filepath.Join(root, "review"),
			); err != nil {
				t.Fatal(err)
			}
		})
	}

	t.Run("negative fixtures", func(t *testing.T) {
		root := copyPublicFixture(t, "retry-policy")
		requestDirectory := filepath.Join(root, "request")
		if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
			t.Fatal(err)
		}
		base := retryProposal(t, requestDirectory)
		cases := []struct {
			name   string
			mutate func(*Candidate)
		}{
			{
				name: "unsupported-evidence",
				mutate: func(candidate *Candidate) {
					candidate.Evidence.Quote = "Decision: use unlimited retries."
				},
			},
			{
				name: "stale-target",
				mutate: func(candidate *Candidate) {
					candidate.Target.OriginalSHA256 = strings.Repeat("0", 64)
				},
			},
		}
		for _, test := range cases {
			proposal := base
			proposal.Candidates = append([]Candidate(nil), base.Candidates...)
			test.mutate(&proposal.Candidates[0])
			proposal.Candidates[0].ID = mustCandidateID(t, proposal.Candidates[0])
			data, err := artifact.CanonicalJSON(proposal)
			if err != nil {
				t.Fatal(err)
			}
			sourceDirectory, err := filepath.Abs(filepath.Join("..", "..", "testdata", "handoff-v0", "negative", test.name))
			if err != nil {
				t.Fatal(err)
			}
			sourcePath := filepath.Join(sourceDirectory, "proposal.json")
			if update {
				if err := os.MkdirAll(sourceDirectory, 0o755); err != nil {
					t.Fatal(err)
				}
				if err := os.WriteFile(sourcePath, data, 0o644); err != nil {
					t.Fatal(err)
				}
			} else if actual := mustRead(t, sourcePath); !bytes.Equal(actual, data) {
				t.Fatalf("negative proposal fixture %q is stale", test.name)
			}
			proposalPath := filepath.Join(root, test.name+".json")
			if err := os.WriteFile(proposalPath, data, 0o644); err != nil {
				t.Fatal(err)
			}
			if _, err := Verify(
				filepath.Join(requestDirectory, RequestFileName),
				proposalPath,
				filepath.Join(root, "review-"+test.name),
			); err == nil {
				t.Fatalf("negative proposal fixture %q was accepted", test.name)
			}
		}
	})
}

func TestHandoffDemo(t *testing.T) {
	for _, fixture := range []string{"retry-policy", "api-deprecation", "brainstorming"} {
		t.Run(fixture, func(t *testing.T) {
			root := copyPublicFixture(t, fixture)
			requestDirectory := filepath.Join(root, "request")
			if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
				t.Fatal(err)
			}
			proposalPath := filepath.Join(root, "proposal.json")
			if _, err := Verify(
				filepath.Join(requestDirectory, RequestFileName),
				proposalPath,
				filepath.Join(root, "review"),
			); err != nil {
				t.Fatal(err)
			}
			t.Logf("fixture=%s request=%s proposal=%s review=verified route=require_review provider_calls=0 source_mutations=0",
				fixture,
				artifact.DigestBytes(mustRead(t, filepath.Join(requestDirectory, RequestFileName))),
				artifact.DigestBytes(mustRead(t, proposalPath)),
			)
		})
	}
}

func TestGoldenReviewFixture(t *testing.T) {
	root := copyPublicFixture(t, "retry-policy")
	requestDirectory := filepath.Join(root, "request")
	reviewDirectory := filepath.Join(root, "review")
	if _, err := Prepare(filepath.Join(root, "conversation.md"), filepath.Join(root, "docs"), requestDirectory); err != nil {
		t.Fatal(err)
	}
	if _, err := Verify(
		filepath.Join(requestDirectory, RequestFileName),
		filepath.Join(root, "proposal.json"),
		reviewDirectory,
	); err != nil {
		t.Fatal(err)
	}
	actual := make(map[string]string)
	for _, item := range []struct {
		prefix string
		root   string
	}{
		{prefix: "request/", root: requestDirectory},
		{prefix: "review/", root: reviewDirectory},
	} {
		for _, name := range fileNames(t, item.root) {
			actual[item.prefix+name] = artifact.DigestBytes(mustRead(t, filepath.Join(item.root, filepath.FromSlash(name))))
		}
	}
	expected := map[string]string{
		"request/SHA256SUMS":                     "3fa8eb3141c79b7b74a65b49af0b9c271234bbcd645f3d5eaa9623c13ad07a5e",
		"request/agent-instructions.md":          "3fa0521ae38e1ef4693689000f5090e289c7089c999f02d7f6b6252ec3f75768",
		"request/handoff.request.json":           "84fa1bd3a4d48e5583cccd6e01d57f7a4061142c74e010e30be826403afabf89",
		"request/inputs/conversation.md":         "94a8d1951427b388d37919fecee4d68ae2f0d4398199b29214ce5230ef74077a",
		"request/inputs/docs/runbook/retries.md": "21f128d20f43ebeaa8a37a8519363103ea8cf4d57b126c7643879bfc07fbea70",
		"request/proposal.schema.json":           "a0cb1730ea215fa276774546f245d2994fa3d6a35879b8a806711c3ef09aea2f",
		"review/SHA256SUMS":                      "22f5bb13c9c8cd3e08ecf16a3ef47d90f3ed81ec12bfea816c1e2d565111ddbc",
		"review/handoff.receipt.json":            "5d138eef331973f0430be0d1bb77c30f6d925f9d07686f4125381a196f0066f3",
		"review/patches/decision_31aa07641f4dca8447a579d6a42acc80ca3657d3ccae0fa43a715667ee1861cd.patch": "645e3cd036647c78073739e00b11e09e9efb34bea701dd248b6a5522e4a97f23",
		"review/review.md": "c0aeae554a1d419dc8592f40bdbfb1cd7c1d3c6d842ffc2edabb749e09c7b628",
	}
	if !reflect.DeepEqual(actual, expected) {
		t.Fatalf("golden hashes differ\nactual: %#v", actual)
	}
}

func retryProposal(t *testing.T, requestDirectory string) Proposal {
	t.Helper()
	bundle, requestBytes, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName))
	if err != nil {
		t.Fatal(err)
	}
	quote := "Decision: production requests will use three attempts with exponential backoff starting at 200ms."
	document := bundle.request.Documents[0]
	diff := "--- a/runbook/retries.md\n" +
		"+++ b/runbook/retries.md\n" +
		"@@ -1,3 +1,3 @@\n" +
		" # Retry policy\n" +
		" \n" +
		"-Production requests use two immediate retries.\n" +
		"+Production requests use three attempts with exponential backoff starting at 200ms.\n"
	candidate := candidateFor(t, bundle, quote, document, diff, "policy")
	return Proposal{
		SchemaVersion: ProposalSchemaVersion, RequestID: bundle.request.RequestID,
		RequestSHA256: artifact.DigestBytes(requestBytes), Outcome: OutcomeCandidates,
		Route: RequireReview, Reason: "One committed retry-policy decision is ready for review.",
		Candidates: []Candidate{candidate},
	}
}

func apiDeprecationProposal(t *testing.T, requestDirectory string) Proposal {
	t.Helper()
	bundle, requestBytes, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName))
	if err != nil {
		t.Fatal(err)
	}
	quote := "Decision: deprecate API v1 on 2026-12-01 and remove it on 2027-03-01."
	document := bundle.request.Documents[0]
	diff := "--- a/reference/api.md\n" +
		"+++ b/reference/api.md\n" +
		"@@ -1,3 +1,3 @@\n" +
		" # API lifecycle\n" +
		" \n" +
		"-API v1 remains supported indefinitely.\n" +
		"+API v1 is deprecated on 2026-12-01 and will be removed on 2027-03-01.\n"
	candidate := candidateFor(t, bundle, quote, document, diff, "api")
	return Proposal{
		SchemaVersion: ProposalSchemaVersion, RequestID: bundle.request.RequestID,
		RequestSHA256: artifact.DigestBytes(requestBytes), Outcome: OutcomeCandidates,
		Route: RequireReview, Reason: "One committed API lifecycle decision is ready for review.",
		Candidates: []Candidate{candidate},
	}
}

func noDecisionProposal(t *testing.T, requestDirectory string) Proposal {
	t.Helper()
	bundle, requestBytes, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName))
	if err != nil {
		t.Fatal(err)
	}
	return Proposal{
		SchemaVersion: ProposalSchemaVersion, RequestID: bundle.request.RequestID,
		RequestSHA256: artifact.DigestBytes(requestBytes), Outcome: OutcomeNoDecision,
		Route: RequireReview, Reason: "The conversation explicitly deferred the decision.",
		Candidates: []Candidate{},
	}
}

func determinismProposal(t *testing.T, requestDirectory string) Proposal {
	t.Helper()
	bundle, requestBytes, err := loadRequestBundle(filepath.Join(requestDirectory, RequestFileName))
	if err != nil {
		t.Fatal(err)
	}
	quote := "Decision: update alpha."
	document := bundle.request.Documents[0]
	diff := "--- a/alpha.md\n+++ b/alpha.md\n@@ -1 +1 @@\n-alpha\n+alpha updated\n"
	candidate := candidateFor(t, bundle, quote, document, diff, "process")
	return Proposal{
		SchemaVersion: ProposalSchemaVersion, RequestID: bundle.request.RequestID,
		RequestSHA256: artifact.DigestBytes(requestBytes), Outcome: OutcomeCandidates,
		Route: RequireReview, Reason: "One deterministic candidate.", Candidates: []Candidate{candidate},
	}
}

func candidateFor(t *testing.T, bundle *requestBundle, quote string, document InputIdentity, diff, decisionType string) Candidate {
	t.Helper()
	start := bytes.Index(bundle.conversation, []byte(quote))
	if start < 0 {
		t.Fatalf("quote %q is absent", quote)
	}
	end := start + len(quote)
	candidate := Candidate{
		DecisionText: quote, DecisionType: decisionType,
		Evidence: Evidence{
			SourcePath: ConversationPath, SourceNormalizedSHA256: bundle.request.Conversation.NormalizedSHA256,
			Quote: quote, StartByte: int64(start), EndByte: int64(end),
			StartLine: 1 + bytes.Count(bundle.conversation[:start], []byte("\n")),
			EndLine:   1 + bytes.Count(bundle.conversation[:end-1], []byte("\n")),
		},
		Target: Target{
			Path: document.Path, OriginalSHA256: document.OriginalSHA256,
			NormalizedSHA256: document.NormalizedSHA256,
		},
		Patch:  Patch{UnifiedDiff: diff, SHA256: artifact.DigestBytes([]byte(diff))},
		Route:  RequireReview,
		Reason: "The patch is only a candidate and requires a human reviewer.",
	}
	candidate.ID = mustCandidateID(t, candidate)
	return candidate
}

func createDeterminismInput(t *testing.T, order []string) string {
	t.Helper()
	root := realTempDir(t)
	mustMkdir(t, filepath.Join(root, "docs"))
	mustWrite(t, filepath.Join(root, "conversation.md"), "# Review\n\nDecision: update alpha.\n")
	for _, name := range order {
		content := strings.TrimSuffix(name, ".md") + "\n"
		mustWrite(t, filepath.Join(root, "docs", name), content)
	}
	return root
}

func copyPublicFixture(t *testing.T, name string) string {
	t.Helper()
	source := fixtureSourcePath(t, name)
	destination := filepath.Join(realTempDir(t), name)
	err := filepath.WalkDir(source, func(filePath string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		relative, err := filepath.Rel(source, filePath)
		if err != nil {
			return err
		}
		target := filepath.Join(destination, relative)
		if entry.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := os.ReadFile(filePath)
		if err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil {
		t.Fatal(err)
	}
	return destination
}

func fixtureSourcePath(t *testing.T, name string) string {
	t.Helper()
	source, err := filepath.Abs(filepath.Join("..", "..", "testdata", "handoff-v0", name))
	if err != nil {
		t.Fatal(err)
	}
	return source
}

func writeCanonical(t *testing.T, path string, value any) {
	t.Helper()
	data, err := artifact.CanonicalJSON(value)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func realTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func mustCandidateID(t *testing.T, candidate Candidate) string {
	t.Helper()
	id, err := CandidateID(candidate)
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func mustRead(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return data
}

func mustWrite(t *testing.T, path, content string) {
	t.Helper()
	mustMkdir(t, filepath.Dir(path))
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func mustMkdir(t *testing.T, path string) {
	t.Helper()
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

func fileNames(t *testing.T, root string) []string {
	t.Helper()
	var names []string
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if !entry.IsDir() {
			relative, err := filepath.Rel(root, path)
			if err != nil {
				return err
			}
			names = append(names, filepath.ToSlash(relative))
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	sort.Strings(names)
	return names
}

func assertTreesEqual(t *testing.T, left, right string) {
	t.Helper()
	leftNames := fileNames(t, left)
	rightNames := fileNames(t, right)
	if !reflect.DeepEqual(leftNames, rightNames) {
		t.Fatalf("tree names differ: %#v != %#v", leftNames, rightNames)
	}
	for _, name := range leftNames {
		if !bytes.Equal(
			mustRead(t, filepath.Join(left, filepath.FromSlash(name))),
			mustRead(t, filepath.Join(right, filepath.FromSlash(name))),
		) {
			t.Fatalf("tree file %q differs", name)
		}
	}
}
