package studyfinal

import (
	"bytes"
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"unicode/utf8"
)

var contaminationCategories = []string{"cases", "fixtures", "labels", "outputs", "pilot_inputs"}

type ContaminationCategory struct {
	Name           string `json:"name"`
	Decision       string `json:"decision"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type AgentTraceContaminationArtifact struct {
	SchemaVersion    string                  `json:"schema_version"`
	RepositoryURL    string                  `json:"repository_url"`
	Commit           string                  `json:"commit"`
	Decision         string                  `json:"decision"`
	Categories       []ContaminationCategory `json:"categories"`
	TrustRootVersion string                  `json:"trust_root_version"`
	TrustRootSHA256  string                  `json:"trust_root_sha256"`
	Approvals        []ReviewApproval        `json:"approvals"`
	StatementSHA256  string                  `json:"statement_sha256"`
	ArtifactSHA256   string                  `json:"artifact_sha256"`
}

type ValidatedAgentTraceArtifact struct {
	raw      []byte
	digest   string
	decision string
}

func contaminationStatementDigest(artifact AgentTraceContaminationArtifact) (string, error) {
	artifact.Approvals = nil
	artifact.StatementSHA256 = ""
	artifact.ArtifactSHA256 = ""
	return DigestDomain("contamination-statement", artifact)
}

func contaminationArtifactDigest(artifact AgentTraceContaminationArtifact) (string, error) {
	artifact.ArtifactSHA256 = ""
	return DigestDomain("contamination-artifact", artifact)
}

func finalizeContaminationArtifact(artifact AgentTraceContaminationArtifact, signers map[string]ed25519.PrivateKey) ([]byte, error) {
	sort.Slice(artifact.Categories, func(i, j int) bool { return artifact.Categories[i].Name < artifact.Categories[j].Name })
	artifact.Approvals = nil
	statement, err := contaminationStatementDigest(artifact)
	if err != nil {
		return nil, err
	}
	artifact.StatementSHA256 = statement
	for _, review := range requiredReviews() {
		key, ok := signers[review]
		if !ok {
			return nil, fmt.Errorf("missing contamination reviewer %s", review)
		}
		reviewerID := review + "-reviewer"
		message := []byte("agenttrace-contamination\x00" + statement + "\x00" + review + "\x00" + reviewerID)
		artifact.Approvals = append(artifact.Approvals, ReviewApproval{
			Review: review, ReviewerID: reviewerID,
			PublicKeyHex: hex.EncodeToString(key.Public().(ed25519.PublicKey)),
			SignatureHex: hex.EncodeToString(ed25519.Sign(key, message)),
		})
	}
	artifact.ArtifactSHA256, _ = contaminationArtifactDigest(artifact)
	return canonicalJSON(artifact)
}

func ValidateAgentTraceArtifact(raw []byte, expectedSHA256 string) (*ValidatedAgentTraceArtifact, error) {
	return validateAgentTraceArtifact(raw, expectedSHA256, productionAuthorizationEnvironment())
}

func validateAgentTraceArtifact(raw []byte, expectedSHA256 string, env authorizationEnvironment) (*ValidatedAgentTraceArtifact, error) {
	if len(raw) == 0 {
		return nil, fmt.Errorf("contamination artifact is empty")
	}
	if len(expectedSHA256) != 64 || digestBytes(raw) != expectedSHA256 ||
		env.contaminationDigest == "" || expectedSHA256 != env.contaminationDigest {
		return nil, fmt.Errorf("contamination artifact raw SHA-256 mismatch")
	}
	var artifact AgentTraceContaminationArtifact
	if err := strictDecode(bytes.NewReader(raw), &artifact); err != nil {
		return validateAgentTraceMarkdown(raw, expectedSHA256)
	}

	canonical, _ := canonicalJSON(artifact)
	if !bytes.Equal(canonical, raw) {
		return nil, fmt.Errorf("contamination artifact must be canonical")
	}
	if artifact.SchemaVersion != SchemaVersion+"/agenttrace-contamination" ||
		artifact.RepositoryURL != repositoryURL("AgentTrace") || artifact.Commit != AgentTraceCommit ||
		(artifact.Decision != "clean-held-out" && artifact.Decision != "contaminated-downgrade") {
		return nil, fmt.Errorf("invalid contamination artifact identity or decision")
	}
	trustDigest, _ := trustedReviewersDigest(env.trusted)
	if len(env.trusted) != len(requiredReviews()) || trustDigest != env.trustDigest ||
		artifact.TrustRootVersion != env.trustVersion || artifact.TrustRootSHA256 != env.trustDigest {
		return nil, fmt.Errorf("contamination artifact trust root mismatch")
	}
	seenKeys := map[string]bool{}
	for _, review := range requiredReviews() {
		key, ok := env.trusted[review]
		encoded := hex.EncodeToString(key)
		if !ok || len(key) != ed25519.PublicKeySize || seenKeys[encoded] {
			return nil, fmt.Errorf("contamination reviewer trust root is incomplete")
		}
		seenKeys[encoded] = true
	}
	if len(artifact.Categories) != len(contaminationCategories) {
		return nil, fmt.Errorf("contamination category count mismatch")
	}
	seen, contaminated := map[string]bool{}, false
	for _, category := range artifact.Categories {
		if seen[category.Name] || !contains(contaminationCategories, category.Name) ||
			(category.Decision != "not-used" && category.Decision != "contaminated") ||
			len(category.EvidenceSHA256) != 64 {
			return nil, fmt.Errorf("invalid contamination category")
		}
		seen[category.Name] = true
		contaminated = contaminated || category.Decision == "contaminated"
	}
	if (artifact.Decision == "clean-held-out" && contaminated) ||
		(artifact.Decision == "contaminated-downgrade" && !contaminated) {
		return nil, fmt.Errorf("contamination conclusion contradicts categories")
	}
	statement, _ := contaminationStatementDigest(artifact)
	if statement != artifact.StatementSHA256 || len(artifact.Approvals) != len(requiredReviews()) {
		return nil, fmt.Errorf("contamination statement mismatch")
	}
	seenReviews := map[string]bool{}
	for _, approval := range artifact.Approvals {
		key, ok := env.trusted[approval.Review]
		signature, err := hex.DecodeString(approval.SignatureHex)
		message := []byte("agenttrace-contamination\x00" + statement + "\x00" + approval.Review + "\x00" + approval.ReviewerID)
		if !ok || seenReviews[approval.Review] || approval.PublicKeyHex != hex.EncodeToString(key) ||
			err != nil || !ed25519.Verify(key, message, signature) {
			return nil, fmt.Errorf("invalid contamination review approval")
		}
		seenReviews[approval.Review] = true
	}
	digest, _ := contaminationArtifactDigest(artifact)
	if digest != artifact.ArtifactSHA256 {
		return nil, fmt.Errorf("contamination artifact digest mismatch")
	}
	return &ValidatedAgentTraceArtifact{raw: append([]byte(nil), raw...), digest: expectedSHA256, decision: artifact.Decision}, nil
}

func validateAgentTraceMarkdown(raw []byte, expectedSHA256 string) (*ValidatedAgentTraceArtifact, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("contamination ledger is not UTF-8")
	}
	text := string(raw)
	required := []string{
		"# Final-Study Contamination Ledger v1",
		"**Record:** `context-build-artifact/final-contamination-ledger/v1`",
		"## Repository role decision",
		AgentTraceCommit,
		"is eligible for the\nrepository-level threshold holdout",
		"## Pilot exclusion audit",
		"AgentTrace repository name or commit in pilot source/generated artifacts | absent",
		"AgentTrace final family IDs in pilot source/generated artifacts | absent",
		"Exact AgentTrace anchor-file digest reused by a pilot artifact | absent",
		"Exact AgentTrace public synthetic-bundle member digest reused by a pilot artifact | absent",
		"Pilot case/request/label identifiers reused as final identifiers | prohibited",
		"Pilot artifacts admitted to the final namespace | prohibited",
		"Any early access or exact derivative discovered later downgrades AgentTrace",
	}
	for _, value := range required {
		if !strings.Contains(text, value) {
			return nil, fmt.Errorf("contamination ledger missing required record %q", value)
		}
	}
	if strings.Contains(text, "| present |") || strings.Contains(text, "is not eligible") {
		return nil, fmt.Errorf("contamination ledger does not support held-out status")
	}
	return &ValidatedAgentTraceArtifact{raw: append([]byte(nil), raw...), digest: expectedSHA256, decision: "clean-held-out"}, nil
}
