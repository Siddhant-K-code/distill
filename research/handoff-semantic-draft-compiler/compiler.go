package main

import (
	"bytes"
	_ "embed"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/Siddhant-K-code/distill/internal/artifact"
	"github.com/Siddhant-K-code/distill/pkg/handoff"
	"github.com/santhosh-tekuri/jsonschema/v6"
)

const semanticDraftSchemaVersion = "distill-handoff/semantic-draft-research/v1"

//go:embed semantic-draft.schema.json
var semanticDraftSchemaBytes []byte

type semanticDraft struct {
	SchemaVersion string           `json:"schema_version"`
	Outcome       string           `json:"outcome"`
	Reason        string           `json:"reason"`
	Candidate     *candidateIntent `json:"candidate"`
}

type candidateIntent struct {
	DecisionText           string `json:"decision_text"`
	DecisionType           string `json:"decision_type"`
	EvidenceQuote          string `json:"evidence_quote"`
	EvidenceOccurrence     int    `json:"evidence_occurrence"`
	TargetPath             string `json:"target_path"`
	TargetAnchorQuote      string `json:"target_anchor_quote"`
	TargetAnchorOccurrence int    `json:"target_anchor_occurrence"`
	Operation              string `json:"operation"`
	ReplacementText        string `json:"replacement_text"`
	Rationale              string `json:"rationale"`
}

type compileResult struct {
	CanonicalDraft []byte
	Proposal       []byte
}

func compileSemanticDraft(requestPath, expectedRequestSHA256, draftPath string) (compileResult, error) {
	request, requestBytes, root, err := loadTrustedRequest(requestPath, expectedRequestSHA256)
	if err != nil {
		return compileResult{}, err
	}
	draft, canonicalDraft, err := loadSemanticDraft(draftPath)
	if err != nil {
		return compileResult{}, err
	}

	proposal := handoff.Proposal{
		SchemaVersion: handoff.ProposalSchemaVersion,
		RequestID:     request.RequestID,
		RequestSHA256: artifact.DigestBytes(requestBytes),
		Route:         handoff.RequireReview,
		Reason:        draft.Reason,
		Candidates:    []handoff.Candidate{},
	}
	if draft.Outcome == "abstain" {
		proposal.Outcome = handoff.OutcomeNoDecision
	} else {
		candidate, compileErr := compileCandidate(root, request, *draft.Candidate)
		if compileErr != nil {
			return compileResult{}, compileErr
		}
		proposal.Outcome = handoff.OutcomeCandidates
		proposal.Candidates = append(proposal.Candidates, candidate)
		sort.Slice(proposal.Candidates, func(i, j int) bool {
			return proposal.Candidates[i].ID < proposal.Candidates[j].ID
		})
	}

	proposalBytes, err := artifact.CanonicalJSON(proposal)
	if err != nil {
		return compileResult{}, fmt.Errorf("encode proposal: %w", err)
	}
	return compileResult{CanonicalDraft: canonicalDraft, Proposal: proposalBytes}, nil
}

func loadTrustedRequest(requestPath, expectedSHA256 string) (handoff.Request, []byte, string, error) {
	if !artifact.ValidDigest(expectedSHA256) {
		return handoff.Request{}, nil, "", fmt.Errorf("expected request SHA-256 is invalid")
	}
	requestBytes, err := readRegularFileLimited(requestPath, 8<<20)
	if err != nil {
		return handoff.Request{}, nil, "", fmt.Errorf("read request: %w", err)
	}
	if artifact.DigestBytes(requestBytes) != expectedSHA256 {
		return handoff.Request{}, nil, "", fmt.Errorf("request SHA-256 does not match retained digest")
	}
	var request handoff.Request
	if err := artifact.DecodeCanonicalJSON(requestBytes, &request); err != nil {
		return handoff.Request{}, nil, "", fmt.Errorf("request: %w", err)
	}
	if request.SchemaVersion != handoff.RequestSchemaVersion {
		return handoff.Request{}, nil, "", fmt.Errorf("request schema_version mismatch")
	}
	wantIdentities := handoff.Identities{
		Tool:             handoff.ToolIdentity,
		Canonicalization: handoff.CanonicalizationIdentity,
		CanonicalJSON:    handoff.CanonicalJSONIdentity,
		RequestID:        handoff.RequestIDIdentity,
		CandidateID:      handoff.CandidateIDIdentity,
		Patch:            handoff.PatchIdentity,
		Review:           handoff.ReviewIdentity,
	}
	if request.Identities != wantIdentities {
		return handoff.Request{}, nil, "", fmt.Errorf("request implementation identities mismatch")
	}
	if request.Conversation.Path != "conversation.md" ||
		request.Conversation.BundlePath != handoff.ConversationPath {
		return handoff.Request{}, nil, "", fmt.Errorf("request conversation identity mismatch")
	}
	root, err := filepath.Abs(filepath.Dir(requestPath))
	if err != nil {
		return handoff.Request{}, nil, "", fmt.Errorf("resolve request root: %w", err)
	}
	root, err = filepath.EvalSymlinks(root)
	if err != nil {
		return handoff.Request{}, nil, "", fmt.Errorf("resolve request root: %w", err)
	}
	return request, requestBytes, root, nil
}

func loadSemanticDraft(draftPath string) (semanticDraft, []byte, error) {
	data, err := readRegularFileLimited(draftPath, 1<<20)
	if err != nil {
		return semanticDraft{}, nil, fmt.Errorf("read semantic draft: %w", err)
	}
	if err := validateSemanticDraftSchema(data); err != nil {
		return semanticDraft{}, nil, err
	}
	var draft semanticDraft
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&draft); err != nil {
		return semanticDraft{}, nil, fmt.Errorf("decode semantic draft: %w", err)
	}
	var trailing any
	if err := decoder.Decode(&trailing); err != io.EOF {
		if err == nil {
			return semanticDraft{}, nil, fmt.Errorf("semantic draft contains a trailing JSON value")
		}
		return semanticDraft{}, nil, fmt.Errorf("decode semantic draft trailing data: %w", err)
	}
	if draft.SchemaVersion != semanticDraftSchemaVersion {
		return semanticDraft{}, nil, fmt.Errorf("semantic draft schema_version mismatch")
	}
	if err := validateDraftValues(draft); err != nil {
		return semanticDraft{}, nil, err
	}
	canonical, err := artifact.CanonicalJSON(draft)
	if err != nil {
		return semanticDraft{}, nil, fmt.Errorf("canonicalize semantic draft: %w", err)
	}
	return draft, canonical, nil
}

func validateSemanticDraftSchema(data []byte) error {
	var schemaDocument any
	if err := json.Unmarshal(semanticDraftSchemaBytes, &schemaDocument); err != nil {
		return fmt.Errorf("decode embedded semantic-draft schema: %w", err)
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	if err := compiler.AddResource("urn:distill:handoff:semantic-draft:research:v1", schemaDocument); err != nil {
		return fmt.Errorf("load semantic-draft schema: %w", err)
	}
	schema, err := compiler.Compile("urn:distill:handoff:semantic-draft:research:v1")
	if err != nil {
		return fmt.Errorf("compile semantic-draft schema: %w", err)
	}
	var value any
	if err := json.Unmarshal(data, &value); err != nil {
		return fmt.Errorf("semantic draft JSON: %w", err)
	}
	if err := schema.Validate(value); err != nil {
		return fmt.Errorf("semantic draft schema validation: %w", err)
	}
	return nil
}

func validateDraftValues(draft semanticDraft) error {
	if err := validateNormalizedText("reason", draft.Reason, false); err != nil {
		return err
	}
	switch draft.Outcome {
	case "abstain":
		if draft.Candidate != nil {
			return fmt.Errorf("abstain draft must have a null candidate")
		}
		return nil
	case "candidate_intent":
		if draft.Candidate == nil {
			return fmt.Errorf("candidate_intent draft must include a candidate")
		}
	default:
		return fmt.Errorf("unsupported semantic outcome %q", draft.Outcome)
	}

	candidate := *draft.Candidate
	for _, field := range []struct {
		name    string
		value   string
		allowLF bool
	}{
		{name: "decision_text", value: candidate.DecisionText},
		{name: "evidence_quote", value: candidate.EvidenceQuote, allowLF: true},
		{name: "target_anchor_quote", value: candidate.TargetAnchorQuote},
		{name: "replacement_text", value: candidate.ReplacementText},
		{name: "rationale", value: candidate.Rationale},
	} {
		if err := validateNormalizedText(field.name, field.value, field.allowLF); err != nil {
			return err
		}
	}
	allowedTypes := map[string]bool{
		"api": true, "architecture": true, "operations": true, "policy": true,
		"process": true, "product": true, "other": true,
	}
	if !allowedTypes[candidate.DecisionType] {
		return fmt.Errorf("unsupported decision_type %q", candidate.DecisionType)
	}
	if candidate.EvidenceOccurrence != 1 || candidate.TargetAnchorOccurrence != 1 {
		return fmt.Errorf("quote occurrence metadata must be exactly 1")
	}
	if candidate.Operation != "replace" {
		return fmt.Errorf("unsupported operation %q", candidate.Operation)
	}
	canonical, err := artifact.CanonicalPortablePath(candidate.TargetPath)
	if err != nil {
		return fmt.Errorf("target_path: %w", err)
	}
	if canonical != candidate.TargetPath || filepath.Ext(candidate.TargetPath) != ".md" {
		return fmt.Errorf("target_path must be canonical Markdown")
	}
	return nil
}

func validateNormalizedText(name, value string, allowLF bool) error {
	if strings.TrimSpace(value) == "" {
		return fmt.Errorf("%s must not be blank", name)
	}
	normalized, err := artifact.NormalizeText([]byte(value))
	if err != nil {
		return fmt.Errorf("%s: %w", name, err)
	}
	if !bytes.Equal(normalized, []byte(value)) {
		return fmt.Errorf("%s must already use NFC and LF normalization", name)
	}
	if !allowLF && strings.ContainsRune(value, '\n') {
		return fmt.Errorf("%s must be a single line", name)
	}
	return nil
}

func compileCandidate(root string, request handoff.Request, intent candidateIntent) (handoff.Candidate, error) {
	conversation, err := readBundleInput(root, request.Conversation.BundlePath)
	if err != nil {
		return handoff.Candidate{}, fmt.Errorf("conversation: %w", err)
	}
	if artifact.DigestBytes(conversation) != request.Conversation.NormalizedSHA256 {
		return handoff.Candidate{}, fmt.Errorf("conversation normalized SHA-256 mismatch")
	}
	evidenceStart, err := uniqueMatch(conversation, []byte(intent.EvidenceQuote), "evidence quote")
	if err != nil {
		return handoff.Candidate{}, err
	}
	evidenceEnd := evidenceStart + len([]byte(intent.EvidenceQuote))

	var targetIdentity *handoff.InputIdentity
	for index := range request.Documents {
		if request.Documents[index].Path == intent.TargetPath {
			targetIdentity = &request.Documents[index]
			break
		}
	}
	if targetIdentity == nil {
		return handoff.Candidate{}, fmt.Errorf("target path is not present in the frozen request")
	}
	if targetIdentity.BundlePath != "inputs/docs/"+targetIdentity.Path {
		return handoff.Candidate{}, fmt.Errorf("target bundle path identity mismatch")
	}
	target, err := readBundleInput(root, targetIdentity.BundlePath)
	if err != nil {
		return handoff.Candidate{}, fmt.Errorf("target document: %w", err)
	}
	if artifact.DigestBytes(target) != targetIdentity.NormalizedSHA256 {
		return handoff.Candidate{}, fmt.Errorf("target normalized SHA-256 mismatch")
	}
	anchorStart, err := uniqueMatch(target, []byte(intent.TargetAnchorQuote), "target anchor quote")
	if err != nil {
		return handoff.Candidate{}, err
	}
	anchorEnd := anchorStart + len([]byte(intent.TargetAnchorQuote))
	if anchorStart > 0 && target[anchorStart-1] != '\n' {
		return handoff.Candidate{}, fmt.Errorf("target anchor is not a complete line")
	}
	if anchorEnd < len(target) && target[anchorEnd] != '\n' {
		return handoff.Candidate{}, fmt.Errorf("target anchor is not a complete line")
	}
	if !bytes.HasSuffix(target, []byte("\n")) {
		return handoff.Candidate{}, fmt.Errorf("target document must end with LF")
	}
	if intent.TargetAnchorQuote == intent.ReplacementText {
		return handoff.Candidate{}, fmt.Errorf("replacement would make no change")
	}

	patchText, err := buildWholeFileReplacementPatch(
		intent.TargetPath, target, intent.TargetAnchorQuote, intent.ReplacementText,
	)
	if err != nil {
		return handoff.Candidate{}, err
	}
	candidate := handoff.Candidate{
		DecisionText: intent.DecisionText,
		DecisionType: intent.DecisionType,
		Evidence: handoff.Evidence{
			SourcePath:             handoff.ConversationPath,
			SourceNormalizedSHA256: request.Conversation.NormalizedSHA256,
			Quote:                  intent.EvidenceQuote,
			StartByte:              int64(evidenceStart),
			EndByte:                int64(evidenceEnd),
			StartLine:              lineAt(conversation, evidenceStart),
			EndLine:                lineAt(conversation, evidenceEnd-1),
		},
		Target: handoff.Target{
			Path:             intent.TargetPath,
			OriginalSHA256:   targetIdentity.OriginalSHA256,
			NormalizedSHA256: targetIdentity.NormalizedSHA256,
		},
		Patch: handoff.Patch{
			UnifiedDiff: patchText,
			SHA256:      artifact.DigestBytes([]byte(patchText)),
		},
		Route:  handoff.RequireReview,
		Reason: intent.Rationale,
	}
	candidate.ID, err = handoff.CandidateID(candidate)
	if err != nil {
		return handoff.Candidate{}, fmt.Errorf("calculate candidate ID: %w", err)
	}
	return candidate, nil
}

func uniqueMatch(haystack, needle []byte, label string) (int, error) {
	count := bytes.Count(haystack, needle)
	switch count {
	case 0:
		return 0, fmt.Errorf("%s is missing", label)
	case 1:
		return bytes.Index(haystack, needle), nil
	default:
		return 0, fmt.Errorf("%s is ambiguous: found %d exact matches", label, count)
	}
}

func lineAt(data []byte, offset int) int {
	return 1 + bytes.Count(data[:offset], []byte("\n"))
}

func buildWholeFileReplacementPatch(targetPath string, target []byte, anchor, replacement string) (string, error) {
	lines := strings.Split(strings.TrimSuffix(string(target), "\n"), "\n")
	anchorLine := -1
	for index, line := range lines {
		if line == anchor {
			if anchorLine >= 0 {
				return "", fmt.Errorf("target anchor line is ambiguous")
			}
			anchorLine = index
		}
	}
	if anchorLine < 0 {
		return "", fmt.Errorf("target anchor line is missing")
	}
	var buffer strings.Builder
	fmt.Fprintf(&buffer, "--- a/%s\n+++ b/%s\n", targetPath, targetPath)
	fmt.Fprintf(&buffer, "@@ -1,%d +1,%d @@\n", len(lines), len(lines))
	for index, line := range lines {
		if index == anchorLine {
			buffer.WriteByte('-')
			buffer.WriteString(line)
			buffer.WriteByte('\n')
			buffer.WriteByte('+')
			buffer.WriteString(replacement)
			buffer.WriteByte('\n')
			continue
		}
		buffer.WriteByte(' ')
		buffer.WriteString(line)
		buffer.WriteByte('\n')
	}
	return buffer.String(), nil
}

func readBundleInput(root, portablePath string) ([]byte, error) {
	canonical, err := artifact.CanonicalPortablePath(portablePath)
	if err != nil || canonical != portablePath {
		return nil, fmt.Errorf("unsafe bundle path %q", portablePath)
	}
	fullPath := filepath.Join(root, filepath.FromSlash(portablePath))
	resolved, err := filepath.EvalSymlinks(fullPath)
	if err != nil {
		return nil, err
	}
	if resolved != root && !strings.HasPrefix(resolved, root+string(filepath.Separator)) {
		return nil, fmt.Errorf("bundle path escapes request root")
	}
	data, err := readRegularFileLimited(fullPath, 8<<20)
	if err != nil {
		return nil, err
	}
	normalized, err := artifact.NormalizeText(data)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(data, normalized) {
		return nil, fmt.Errorf("bundle input is not normalized")
	}
	return data, nil
}

func readRegularFileLimited(path string, limit int64) ([]byte, error) {
	info, err := os.Lstat(path)
	if err != nil {
		return nil, err
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
		return nil, fmt.Errorf("path must be a regular file, not a symlink")
	}
	if info.Size() > limit {
		return nil, fmt.Errorf("file exceeds %d-byte limit", limit)
	}
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	data, err := io.ReadAll(io.LimitReader(file, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > limit {
		return nil, fmt.Errorf("file exceeds %d-byte limit", limit)
	}
	return data, nil
}
