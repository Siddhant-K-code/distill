package studypilot

const (
	ArtifactSchemaVersion  = "context-build-artifact/offline-pilot/v1"
	RequestProtocol        = "context-build-artifact/jev-choice-request/v1"
	PolicyID               = "context-build-artifact/offline-safety-policy"
	PolicyVersion          = "v1"
	QuestionSchemaID       = "context-build-artifact/seven-atomic-questions"
	QuestionSchemaVersion  = "v1"
	TransformVersion       = "context-build-artifact/synthetic-transform/v1"
	AcceptanceScoreVersion = "minimum-safe-label-probability/v1"
	DatasetSnapshotID      = "context-build-artifact/synthetic-public-pilot/v1"
	LockToolIdentity       = "github.com/Siddhant-K-code/distill/distill-lock-v0"
	LockRuntimeIdentity    = "go1.24-go1.26"
	LockMergeCommit        = "a9b14667024c27c32c9c9dfb9c8dc35865990979"
)

var artifactFileNames = []string{
	"cases.jsonl",
	"labels.jsonl",
	"pilot-manifest.json",
	"policy-baseline.jsonl",
	"receipt-schema.json",
	"requests.jsonl",
	"schedule.jsonl",
	"SHA256SUMS",
}

type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"question"`
	Choices []string `json:"choices"`
}

var Questions = []Question{
	{ID: "evidence_complete", Text: "Are all evidence items required by the registered rubric present?", Choices: []string{"yes", "no", "unknown"}},
	{ID: "observed_tests_support", Text: "Do the preserved observed test results support the proposed handoff?", Choices: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "verifier_support", Text: "Does the registered verifier outcome support the proposed handoff?", Choices: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "cleanup_complete", Text: "Are all registered cleanup obligations complete?", Choices: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "external_effects_resolved", Text: "Are all registered external effects resolved or explicitly accepted?", Choices: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "patch_risk", Text: "What is the evidence-supported patch-risk class?", Choices: []string{"low", "medium", "high", "unknown"}},
	{ID: "recommended_disposition", Text: "What disposition does the evidence support?", Choices: []string{"accept", "review", "reject"}},
}

type RecordEnvelope struct {
	SchemaVersion      string `json:"schema_version"`
	Phase              string `json:"phase"`
	Excluded           bool   `json:"excluded"`
	FinalStudyEligible bool   `json:"final_study_eligible"`
}

type FrozenIdentities struct {
	DatasetSnapshotDigest string              `json:"dataset_snapshot_digest"`
	Config                ConfigIdentity      `json:"config"`
	DistillLock           DistillLockIdentity `json:"distill_lock"`
	QuestionSchemaDigest  string              `json:"question_schema_digest"`
	PolicyDigest          string              `json:"policy_digest"`
	ReceiptSchemaDigest   string              `json:"receipt_schema_digest"`
}

type ConfigIdentity struct {
	SchemaVersion   string `json:"schema_version"`
	ChunkBytes      int    `json:"chunk_bytes"`
	TokenBudget     int    `json:"token_budget"`
	MetadataRemoval string `json:"metadata_removal"`
	Digest          string `json:"digest"`
}

type DistillLockIdentity struct {
	Tool                  string `json:"tool"`
	Runtime               string `json:"runtime"`
	MergeCommit           string `json:"merge_commit"`
	Canonicalization      string `json:"canonicalization"`
	Chunking              string `json:"chunking"`
	TokenEstimation       string `json:"token_estimation"`
	ExactDeduplication    string `json:"exact_deduplication"`
	Selection             string `json:"selection"`
	BundleRendering       string `json:"bundle_rendering"`
	CanonicalJSON         string `json:"canonical_json"`
	ContextBundleSHA256   string `json:"context_bundle_sha256"`
	ContextLockSHA256     string `json:"context_lock_sha256"`
	ContextManifestSHA256 string `json:"context_manifest_sha256"`
	ChecksumsSHA256       string `json:"checksums_sha256"`
}

type Source struct {
	SourceID                string `json:"source_id"`
	RelativePath            string `json:"relative_path"`
	MediaType               string `json:"media_type"`
	ByteLength              int    `json:"byte_length"`
	ContentDigest           string `json:"content_digest"`
	NormalizedContentDigest string `json:"normalized_content_digest"`
	EvidenceRole            string `json:"evidence_role"`
	Content                 string `json:"content"`
}

type SourceSet struct {
	SourceSetID     string   `json:"source_set_id"`
	ManifestOrder   []string `json:"manifest_order"`
	Sources         []Source `json:"sources"`
	SourceSetDigest string   `json:"source_set_digest"`
}

type Operation struct {
	Kind   string `json:"kind"`
	Target string `json:"target"`
	Detail string `json:"detail"`
}

type Perturbation struct {
	PerturbationID                  string      `json:"perturbation_id"`
	Type                            string      `json:"type"`
	TransformVersion                string      `json:"transform_version"`
	Seed                            uint64      `json:"seed"`
	ExpectedEquivalence             string      `json:"expected_equivalence"`
	BaseSourceSetDigest             string      `json:"base_source_set_digest"`
	ResultSourceSetDigest           string      `json:"result_source_set_digest"`
	TransformationReceiptArtifactID string      `json:"transformation_receipt_artifact_id"`
	TransformationReceiptDigest     string      `json:"transformation_receipt_digest"`
	Operations                      []Operation `json:"operations"`
}

type Evidence struct {
	EvidenceComplete        string        `json:"evidence_complete"`
	ObservedTestsSupport    string        `json:"observed_tests_support"`
	VerifierSupport         string        `json:"verifier_support"`
	CleanupComplete         string        `json:"cleanup_complete"`
	ExternalEffectsResolved string        `json:"external_effects_resolved"`
	PatchRisk               string        `json:"patch_risk"`
	RecommendedDisposition  string        `json:"recommended_disposition"`
	Contradiction           bool          `json:"contradiction"`
	Stale                   bool          `json:"stale"`
	Applicability           Applicability `json:"applicability"`
}

type Applicability struct {
	ObservedTests   bool `json:"observed_tests"`
	Verifier        bool `json:"verifier"`
	Cleanup         bool `json:"cleanup"`
	ExternalEffects bool `json:"external_effects"`
}

type CaseRecord struct {
	RecordEnvelope
	CaseID                 string           `json:"case_id"`
	Category               string           `json:"category"`
	RepositoryURL          string           `json:"repository_url"`
	RepositoryCommit       string           `json:"repository_commit"`
	RepositoryRole         string           `json:"repository_role"`
	Split                  string           `json:"split"`
	TaskKind               string           `json:"task_kind"`
	Frozen                 FrozenIdentities `json:"frozen_identities"`
	BaseSourceSet          SourceSet        `json:"base_source_set"`
	SourceSet              SourceSet        `json:"source_set"`
	Perturbation           Perturbation     `json:"perturbation"`
	Evidence               Evidence         `json:"evidence"`
	LabelSealDigest        string           `json:"label_seal_digest"`
	ExpectedBaselineAction string           `json:"expected_baseline_action"`
}

type GroundTruthAnswer struct {
	QuestionID                string   `json:"question_id"`
	Label                     string   `json:"label"`
	SupportingEvidenceDigests []string `json:"supporting_evidence_digests"`
}

type GroundTruth struct {
	Answers           []GroundTruthAnswer `json:"answers"`
	PolicyEvidence    PolicyEvidence      `json:"policy_evidence"`
	DerivationReasons []string            `json:"derivation_reasons"`
}

type PolicyEvidence struct {
	Contradiction bool `json:"contradiction"`
	Stale         bool `json:"stale"`
}

type LabelRecord struct {
	RecordEnvelope
	CaseID             string           `json:"case_id"`
	SourceSetDigest    string           `json:"source_set_digest"`
	Frozen             FrozenIdentities `json:"frozen_identities"`
	LabelSealDigest    string           `json:"label_seal_digest"`
	SealedFromRequests bool             `json:"sealed_from_requests"`
	GroundTruth        GroundTruth      `json:"ground_truth"`
}

type AnswerContract struct {
	SelectedLabel string   `json:"selected_label"`
	Probabilities []string `json:"probability_keys"`
	Confidence    string   `json:"confidence"`
	SumTolerance  float64  `json:"probability_sum_tolerance"`
}

type RequestQuestion struct {
	Field          string         `json:"field"`
	Kind           string         `json:"kind"`
	Question       string         `json:"question"`
	AllowedChoices []string       `json:"allowed_choices"`
	AnswerContract AnswerContract `json:"answer_contract"`
}

type RequestState struct {
	MediaType string `json:"media_type"`
	Content   string `json:"content"`
	SHA256    string `json:"sha256"`
}

type RequestRecord struct {
	RecordEnvelope
	ProtocolVersion       string            `json:"protocol_version"`
	RequestID             string            `json:"request_id"`
	CaseID                string            `json:"case_id"`
	SourceSetDigest       string            `json:"source_set_digest"`
	Frozen                FrozenIdentities  `json:"frozen_identities"`
	State                 RequestState      `json:"state"`
	QuestionSchemaID      string            `json:"question_schema_id"`
	QuestionSchemaVersion string            `json:"question_schema_version"`
	QuestionSchemaDigest  string            `json:"question_schema_digest"`
	Questions             []RequestQuestion `json:"questions"`
}

type ScheduleRecord struct {
	RecordEnvelope
	ScheduledCallID        string           `json:"scheduled_call_id"`
	ScheduleIndex          int              `json:"schedule_index"`
	RecordedExecutionOrder int              `json:"recorded_execution_order"`
	RequestID              string           `json:"request_id"`
	CaseID                 string           `json:"case_id"`
	SourceSetDigest        string           `json:"source_set_digest"`
	Frozen                 FrozenIdentities `json:"frozen_identities"`
	PerturbationDigest     string           `json:"perturbation_digest"`
	ContextArm             string           `json:"context_arm"`
	CompilerDigest         string           `json:"compiler_digest"`
	QuestionSchemaDigest   string           `json:"question_schema_digest"`
	DecisionSystemDigest   string           `json:"decision_system_digest"`
	PolicyDigest           string           `json:"policy_digest"`
	ThresholdSetDigest     string           `json:"threshold_set_digest"`
	RepositoryRole         string           `json:"repository_role"`
	ReplicateIndex         int              `json:"replicate_index"`
}

type PolicyResult struct {
	Result      string   `json:"result"`
	ReasonCodes []string `json:"reason_codes"`
}

type PolicyRecord struct {
	RecordEnvelope
	CaseID          string           `json:"case_id"`
	SourceSetDigest string           `json:"source_set_digest"`
	Frozen          FrozenIdentities `json:"frozen_identities"`
	DecisionStatus  string           `json:"decision_status"`
	PolicyID        string           `json:"policy_id"`
	PolicyVersion   string           `json:"policy_version"`
	PolicyDigest    string           `json:"policy_digest"`
	Evidence        Evidence         `json:"evidence"`
	Result          PolicyResult     `json:"result"`
}

type FileIdentity struct {
	Path   string `json:"path"`
	Bytes  int    `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type Manifest struct {
	RecordEnvelope
	ArtifactSetID             string           `json:"artifact_set_id"`
	ProtocolClarification     string           `json:"protocol_clarification"`
	CaseCount                 int              `json:"case_count"`
	ProviderCalls             int              `json:"provider_calls"`
	LabelsAvailableToRequests bool             `json:"labels_available_to_requests"`
	Frozen                    FrozenIdentities `json:"frozen_identities"`
	Files                     []FileIdentity   `json:"files"`
}

type Summary struct {
	CaseCount             int
	RequestCount          int
	PolicyAccept          int
	PolicyReview          int
	PolicyReject          int
	ProviderCalls         int
	FinalStudyEligible    bool
	ManifestSHA256        string
	ChecksumsSHA256       string
	DatasetSnapshotDigest string
}
