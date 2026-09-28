// Package handoff freezes untrusted decision-to-docs proposals for human review.
// It performs no model, provider, network, patch application, or source mutation.
package handoff

const (
	RequestSchemaVersion  = "distill-handoff/request/v0alpha1"
	ProposalSchemaVersion = "distill-handoff/proposal/v0alpha1"
	ReceiptSchemaVersion  = "distill-handoff/receipt/v0alpha1"

	ToolIdentity             = "github.com/Siddhant-K-code/distill/distill-handoff-v0alpha1"
	CanonicalizationIdentity = "utf8-nfc-lf-v1"
	CanonicalJSONIdentity    = "go-struct-json-indent-v1"
	RequestIDIdentity        = "sha256-domain-separated-request-v1"
	CandidateIDIdentity      = "sha256-domain-separated-candidate-v1"
	PatchIdentity            = "unified-diff-existing-markdown-single-file-v1"
	ReviewIdentity           = "markdown-review-v1"

	RequestFileName      = "handoff.request.json"
	InstructionsFileName = "agent-instructions.md"
	ProposalSchemaName   = "proposal.schema.json"
	ChecksumsFileName    = "SHA256SUMS"
	ConversationPath     = "inputs/conversation.md"

	ReviewFileName  = "review.md"
	ReceiptFileName = "handoff.receipt.json"

	OutcomeCandidates = "candidate_decisions"
	OutcomeNoDecision = "no_decision"
	RequireReview     = "require_review"
)

type Identities struct {
	Tool             string `json:"tool"`
	Canonicalization string `json:"canonicalization"`
	CanonicalJSON    string `json:"canonical_json"`
	RequestID        string `json:"request_id"`
	CandidateID      string `json:"candidate_id"`
	Patch            string `json:"patch"`
	Review           string `json:"review"`
}

type InputIdentity struct {
	Path             string `json:"path"`
	BundlePath       string `json:"bundle_path"`
	OriginalSHA256   string `json:"original_sha256"`
	OriginalBytes    int64  `json:"original_bytes"`
	NormalizedSHA256 string `json:"normalized_sha256"`
	NormalizedBytes  int64  `json:"normalized_bytes"`
}

type ArtifactIdentity struct {
	Path   string `json:"path"`
	SHA256 string `json:"sha256"`
	Bytes  int64  `json:"bytes"`
}

type Request struct {
	SchemaVersion  string           `json:"schema_version"`
	RequestID      string           `json:"request_id"`
	Identities     Identities       `json:"identities"`
	Conversation   InputIdentity    `json:"conversation"`
	Documents      []InputIdentity  `json:"documents"`
	Instructions   ArtifactIdentity `json:"instructions"`
	ProposalSchema ArtifactIdentity `json:"proposal_schema"`
}

type Proposal struct {
	SchemaVersion string      `json:"schema_version"`
	RequestID     string      `json:"request_id"`
	RequestSHA256 string      `json:"request_sha256"`
	Outcome       string      `json:"outcome"`
	Route         string      `json:"route"`
	Reason        string      `json:"reason"`
	Candidates    []Candidate `json:"candidates"`
}

type Candidate struct {
	ID           string   `json:"id"`
	DecisionText string   `json:"decision_text"`
	DecisionType string   `json:"decision_type"`
	Evidence     Evidence `json:"evidence"`
	Target       Target   `json:"target"`
	Patch        Patch    `json:"patch"`
	Route        string   `json:"route"`
	Reason       string   `json:"reason"`
}

type Evidence struct {
	SourcePath             string `json:"source_path"`
	SourceNormalizedSHA256 string `json:"source_normalized_sha256"`
	Quote                  string `json:"quote"`
	StartByte              int64  `json:"start_byte"`
	EndByte                int64  `json:"end_byte"`
	StartLine              int    `json:"start_line"`
	EndLine                int    `json:"end_line"`
}

type Target struct {
	Path             string `json:"path"`
	OriginalSHA256   string `json:"original_sha256"`
	NormalizedSHA256 string `json:"normalized_sha256"`
}

type Patch struct {
	UnifiedDiff string `json:"unified_diff"`
	SHA256      string `json:"sha256"`
}

type Receipt struct {
	SchemaVersion  string             `json:"schema_version"`
	RequestID      string             `json:"request_id"`
	RequestSHA256  string             `json:"request_sha256"`
	ProposalSHA256 string             `json:"proposal_sha256"`
	Outcome        string             `json:"outcome"`
	Route          string             `json:"route"`
	Reason         string             `json:"reason"`
	CandidateCount int                `json:"candidate_count"`
	Candidates     []CandidateReceipt `json:"candidates"`
	Review         ArtifactIdentity   `json:"review"`
	Patches        []ArtifactIdentity `json:"patches"`
}

type CandidateReceipt struct {
	ID                      string   `json:"id"`
	DecisionType            string   `json:"decision_type"`
	Evidence                Evidence `json:"evidence"`
	Target                  Target   `json:"target"`
	PatchSHA256             string   `json:"patch_sha256"`
	PatchedNormalizedSHA256 string   `json:"patched_normalized_sha256"`
	Route                   string   `json:"route"`
}

type Summary struct {
	RequestID      string
	RequestSHA256  string
	ProposalSHA256 string
	ReceiptSHA256  string
	Outcome        string
	DocumentCount  int
	CandidateCount int
}

func currentIdentities() Identities {
	return Identities{
		Tool:             ToolIdentity,
		Canonicalization: CanonicalizationIdentity,
		CanonicalJSON:    CanonicalJSONIdentity,
		RequestID:        RequestIDIdentity,
		CandidateID:      CandidateIDIdentity,
		Patch:            PatchIdentity,
		Review:           ReviewIdentity,
	}
}
