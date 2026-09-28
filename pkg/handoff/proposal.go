package handoff

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sort"
	"strings"
	"sync"

	"github.com/Siddhant-K-code/distill/internal/artifact"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

var (
	proposalSchemaOnce sync.Once
	proposalSchema     *jsonschema.Schema
	proposalSchemaErr  error
)

func loadProposal(proposalPath string) (Proposal, []byte, error) {
	data, err := readSafeFileLimited(proposalPath, 8<<20)
	if err != nil {
		return Proposal{}, nil, fmt.Errorf("read proposal: %w", err)
	}
	if err := validateProposalJSONSchema(data); err != nil {
		return Proposal{}, nil, err
	}
	var proposal Proposal
	if err := artifact.DecodeCanonicalJSON(data, &proposal); err != nil {
		return Proposal{}, nil, fmt.Errorf("proposal: %w", err)
	}
	return proposal, data, nil
}

func validateProposalJSONSchema(data []byte) error {
	proposalSchemaOnce.Do(func() {
		var document any
		if err := json.Unmarshal(proposalSchemaBytes, &document); err != nil {
			proposalSchemaErr = fmt.Errorf("decode embedded proposal schema: %w", err)
			return
		}
		compiler := jsonschema.NewCompiler()
		compiler.DefaultDraft(jsonschema.Draft2020)
		compiler.AssertFormat()
		if err := compiler.AddResource("urn:distill:handoff:proposal:v0alpha1", document); err != nil {
			proposalSchemaErr = fmt.Errorf("load embedded proposal schema: %w", err)
			return
		}
		proposalSchema, proposalSchemaErr = compiler.Compile("urn:distill:handoff:proposal:v0alpha1")
	})
	if proposalSchemaErr != nil {
		return proposalSchemaErr
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("proposal JSON: %w", err)
	}
	if err := proposalSchema.Validate(value); err != nil {
		return fmt.Errorf("proposal schema validation: %w", err)
	}
	return nil
}

func CandidateID(candidate Candidate) (string, error) {
	candidate.ID = ""
	data, err := artifact.CanonicalJSON(candidate)
	if err != nil {
		return "", err
	}
	return "decision_" + artifact.DigestBytes(append([]byte("distill-handoff/candidate/v0alpha1\n"), data...)), nil
}

func validateProposalEnvelope(bundle *requestBundle, proposal Proposal) error {
	if proposal.SchemaVersion != ProposalSchemaVersion {
		return fmt.Errorf("proposal schema_version mismatch")
	}
	if proposal.RequestID != bundle.request.RequestID {
		return fmt.Errorf("proposal request_id does not match request")
	}
	if proposal.RequestSHA256 != artifact.DigestBytes(bundle.requestBytes) {
		return fmt.Errorf("proposal request_sha256 does not match request bytes")
	}
	if proposal.Route != RequireReview {
		return fmt.Errorf("proposal route must be %q", RequireReview)
	}
	if !cleanReason(proposal.Reason) {
		return fmt.Errorf("proposal reason must not be blank")
	}
	if err := validateDisplayText("proposal reason", proposal.Reason, true, true); err != nil {
		return err
	}
	switch proposal.Outcome {
	case OutcomeNoDecision:
		if len(proposal.Candidates) != 0 {
			return fmt.Errorf("no_decision proposal must have an empty candidates array")
		}
	case OutcomeCandidates:
		if len(proposal.Candidates) == 0 {
			return fmt.Errorf("candidate_decisions proposal must include candidates")
		}
		if len(proposal.Candidates) > 100 {
			return fmt.Errorf("proposal exceeds the 100-candidate v0 alpha limit")
		}
	default:
		return fmt.Errorf("unsupported proposal outcome %q", proposal.Outcome)
	}
	if !sort.SliceIsSorted(proposal.Candidates, func(i, j int) bool {
		return proposal.Candidates[i].ID < proposal.Candidates[j].ID
	}) {
		return fmt.Errorf("proposal candidates must be sorted by id")
	}
	return nil
}

func validateCandidateShape(candidate Candidate) error {
	expectedID, err := CandidateID(candidate)
	if err != nil {
		return err
	}
	if candidate.ID != expectedID {
		return fmt.Errorf("candidate %q stable id mismatch; want %q", candidate.ID, expectedID)
	}
	if !cleanReason(candidate.DecisionText) || !cleanReason(candidate.Reason) {
		return fmt.Errorf("candidate %q decision_text and reason must not be blank", candidate.ID)
	}
	for _, field := range []struct {
		label string
		value string
	}{
		{label: "candidate decision_text", value: candidate.DecisionText},
		{label: "candidate reason", value: candidate.Reason},
		{label: "candidate evidence quote", value: candidate.Evidence.Quote},
	} {
		if err := validateDisplayText(field.label, field.value, true, true); err != nil {
			return fmt.Errorf("candidate %q: %w", candidate.ID, err)
		}
	}
	types := map[string]bool{
		"api": true, "architecture": true, "operations": true, "policy": true,
		"process": true, "product": true, "other": true,
	}
	if !types[candidate.DecisionType] {
		return fmt.Errorf("candidate %q has unsupported decision_type %q", candidate.ID, candidate.DecisionType)
	}
	if candidate.Route != RequireReview {
		return fmt.Errorf("candidate %q route must be %q", candidate.ID, RequireReview)
	}
	if candidate.Evidence.SourcePath != ConversationPath {
		return fmt.Errorf("candidate %q evidence source path mismatch", candidate.ID)
	}
	canonical, err := artifact.CanonicalPortablePath(candidate.Target.Path)
	if err != nil {
		return fmt.Errorf("candidate %q target path: %w", candidate.ID, err)
	}
	if canonical != candidate.Target.Path || !strings.HasSuffix(candidate.Target.Path, ".md") {
		return fmt.Errorf("candidate %q target path is not canonical Markdown", candidate.ID)
	}
	if err := validateDisplayText("candidate target path", candidate.Target.Path, false, false); err != nil {
		return fmt.Errorf("candidate %q: %w", candidate.ID, err)
	}
	if !artifact.ValidDigest(candidate.Target.OriginalSHA256) ||
		!artifact.ValidDigest(candidate.Target.NormalizedSHA256) ||
		!artifact.ValidDigest(candidate.Patch.SHA256) ||
		!artifact.ValidDigest(candidate.Evidence.SourceNormalizedSHA256) {
		return fmt.Errorf("candidate %q contains an invalid digest", candidate.ID)
	}
	if reflect.ValueOf(candidate).IsZero() {
		return fmt.Errorf("candidate must not be empty")
	}
	return nil
}
