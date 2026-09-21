package studyfinal

import "time"

const (
	SchemaVersion           = "context-build-artifact/final-study/v1"
	DistillCommit           = "a9b14667024c27c32c9c9dfb9c8dc35865990979"
	LLMTraceFXCommit        = "f9385e1d9ebf862ba46272fb8a45d7881b0b2a9c"
	AgentTraceCommit        = "b109ec5b3714b842746e97ee8e975329d8582667"
	DistillLicenseSHA256    = "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca"
	LLMTraceLicenseSHA256   = "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4"
	AgentTraceLicenseSHA256 = "0a46f53c37edf8ba569b283cbc95fca5b37c7abee62fa5bd46efadf34fa82a80"
	OfflineUnresolved       = "offline-unresolved"
	JevModel                = "jev-1.13.0"
	ArmRaw                  = "A"
	ArmDistillLock          = "C"
	ArmBOmission            = "prospectively-omitted"
	PriorSpendNanoUSD       = int64(760_242)
	TotalCapNanoUSD         = int64(5_000_000_000)
	RemainingCapNanoUSD     = int64(4_999_239_758)
	InputPriceNanoUSD       = int64(42)
	DefaultMaxInputToken    = int64(64_000)
	PricingVersion          = "jev-1.13.0-input-0.042-per-million-output-free-v1"
	DefaultCorpusSeed       = "final-transform-v1:2026-09-20"
	DefaultScheduleSeed     = "final-repeatability-v1:2026-09-20"
	TotalCapUSD             = "5.000000"
	PriorSpendUSD           = "0.000760242"
	RemainingCapUSD         = "4.999239758"
	AgentTraceHeldOut       = "validated-clean-held-out"
	AgentTraceDowngraded    = "downgraded-confirmatory-secondary"
)

type Config struct {
	LLMTraceFXCommit        string                       `json:"llmtracefx_commit"`
	Seed                    string                       `json:"seed"`
	TokenBound              int64                        `json:"token_bound"`
	AgentTraceContamination *ValidatedAgentTraceArtifact `json:"-"`
}

type Question struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Allowed []string `json:"allowed"`
}

var frozenQuestions = []Question{
	{ID: "evidence_complete", Text: "Are all evidence items required by the registered rubric present?", Allowed: []string{"yes", "no", "unknown"}},
	{ID: "observed_tests_support", Text: "Do the preserved observed test results support the proposed handoff?", Allowed: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "verifier_support", Text: "Does the registered verifier outcome support the proposed handoff?", Allowed: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "cleanup_complete", Text: "Are all registered cleanup obligations complete?", Allowed: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "external_effects_resolved", Text: "Are all registered external effects resolved or explicitly accepted?", Allowed: []string{"yes", "no", "unknown", "not_applicable"}},
	{ID: "patch_risk", Text: "What is the evidence-supported patch-risk class?", Allowed: []string{"low", "medium", "high", "unknown"}},
	{ID: "recommended_disposition", Text: "What disposition does the evidence support?", Allowed: []string{"accept", "review", "reject"}},
}

func AtomicQuestions() []Question {
	out := make([]Question, len(frozenQuestions))
	for i, question := range frozenQuestions {
		out[i] = question
		out[i].Allowed = append([]string(nil), question.Allowed...)
	}
	return out
}

type SourceRegistryEntry struct {
	Repository    string `json:"repository"`
	RepositoryURL string `json:"repository_url"`
	Kind          string `json:"kind"`
	BaseID        string `json:"base_id"`
	Commit        string `json:"commit"`
	License       string `json:"license"`
	LicensePath   string `json:"license_path"`
	LicenseSHA256 string `json:"license_sha256"`
	Path          string `json:"path"`
	ContentSHA256 string `json:"content_sha256"`
	Resolution    string `json:"resolution"`
}

type BaseCase struct {
	ID             string `json:"id"`
	Repository     string `json:"repository"`
	RepositoryRole string `json:"repository_role"`
	Split          string `json:"split"`
	EvidencePath   string `json:"evidence_path"`
	Evidence       string `json:"evidence"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type Condition struct {
	ID                     string   `json:"id"`
	BaseID                 string   `json:"base_id"`
	Repository             string   `json:"repository"`
	RepositoryRole         string   `json:"repository_role"`
	Split                  string   `json:"split"`
	Family                 string   `json:"family"`
	TransformVersion       string   `json:"transform_version"`
	Ordinal                int      `json:"ordinal"`
	Seed                   string   `json:"seed"`
	Applicable             bool     `json:"applicable"`
	ExpectedEquivalent     bool     `json:"expected_equivalent"`
	BaseEvidenceSHA256     string   `json:"base_evidence_sha256"`
	SourcePaths            []string `json:"source_paths"`
	SourceContents         []string `json:"source_contents"`
	SourceDigests          []string `json:"source_digests"`
	ConditionSHA256        string   `json:"condition_sha256"`
	TransformReceiptSHA256 string   `json:"transform_receipt_sha256"`
	LabelCommitmentSlot    string   `json:"label_commitment_slot"`
	CustodyProtocolSHA256  string   `json:"custody_protocol_sha256"`
}

type ContaminationEntry struct {
	Repository string `json:"repository"`
	Status     string `json:"status"`
	Reason     string `json:"reason"`
}

type DedupEntry struct {
	ConditionID string `json:"condition_id"`
	GroupKey    string `json:"group_key"`
	Decision    string `json:"decision"`
}

type Dataset struct {
	SchemaVersion                   string                   `json:"schema_version"`
	GeneratedOffline                bool                     `json:"generated_offline"`
	Executable                      bool                     `json:"executable"`
	ArmB                            string                   `json:"arm_b"`
	ArmD                            string                   `json:"arm_d"`
	DistillLock                     DistillLockConfig        `json:"distill_lock"`
	AgentTraceContamination         ContaminationDisposition `json:"agenttrace_contamination"`
	AgentTraceContaminationArtifact []byte                   `json:"agenttrace_contamination_artifact"`
	SourceRegistry                  []byte                   `json:"source_registry"`
	SourceRegistrySHA256            string                   `json:"source_registry_sha256"`
	Registry                        []SourceRegistryEntry    `json:"registry"`
	Bases                           []BaseCase               `json:"bases"`
	Conditions                      []Condition              `json:"conditions"`
	ContaminationLedger             []ContaminationEntry     `json:"contamination_ledger"`
	DeduplicationLedger             []DedupEntry             `json:"deduplication_ledger"`
	QuestionSchemaSHA256            string                   `json:"question_schema_sha256"`
	CorpusSHA256                    string                   `json:"corpus_sha256"`
	SplitSHA256                     string                   `json:"split_sha256"`
}

type ContaminationDisposition struct {
	Status         string `json:"status"`
	EvidenceSHA256 string `json:"evidence_sha256"`
	Verified       bool   `json:"verified"`
	ArtifactPath   string `json:"artifact_path"`
}

type DistillLockConfig struct {
	MergeCommit string `json:"merge_commit"`
	ChunkBytes  int    `json:"chunk_bytes"`
	TokenBudget int    `json:"token_budget"`
	Algorithm   string `json:"algorithm"`
}

type ContextArtifact struct {
	ConditionID string `json:"condition_id"`
	Arm         string `json:"arm"`
	Content     string `json:"content"`
	SHA256      string `json:"sha256"`
}

type ScheduleEntry struct {
	Index           int    `json:"index"`
	CallID          string `json:"call_id"`
	ConditionID     string `json:"condition_id"`
	RepositoryRole  string `json:"repository_role"`
	Split           string `json:"split"`
	Arm             string `json:"arm"`
	Replicate       int    `json:"replicate"`
	Repeatability   bool   `json:"repeatability"`
	ContextSHA256   string `json:"context_sha256"`
	CorpusSHA256    string `json:"corpus_sha256"`
	QuestionSHA256  string `json:"question_sha256"`
	DecisionSHA256  string `json:"decision_sha256"`
	ThresholdSHA256 string `json:"threshold_sha256"`
}

type Answer struct {
	SelectedLabel            string             `json:"selected_label"`
	Probabilities            map[string]float64 `json:"probabilities"`
	RawConfidence            *float64           `json:"raw_confidence"`
	SelectedLabelProbability float64            `json:"selected_label_probability"`
}

type Decision struct {
	System          string            `json:"system"`
	Model           string            `json:"model"`
	Answers         map[string]Answer `json:"answers"`
	PolicyFacts     PolicyFacts       `json:"policy_facts"`
	Disposition     string            `json:"disposition"`
	ReasonCodes     []string          `json:"reason_codes"`
	AcceptanceScore *float64          `json:"acceptance_score"`
}

type PolicyFacts struct {
	SchemaContradiction            bool `json:"schema_contradiction"`
	CryptoContradiction            bool `json:"crypto_contradiction"`
	PrivacyContradiction           bool `json:"privacy_contradiction"`
	AffirmativeUnsupportedClaim    bool `json:"affirmative_unsupported_claim"`
	HonestIncomplete               bool `json:"honest_incomplete"`
	HonestUnsupported              bool `json:"honest_unsupported"`
	HonestRefusal                  bool `json:"honest_refusal"`
	ExplicitNoncomparability       bool `json:"explicit_noncomparability"`
	ClaimedPoolingIdentityMismatch bool `json:"claimed_pooling_identity_mismatch"`
	TeardownClaimed                bool `json:"teardown_claimed"`
	IndependentProviderEvidence    bool `json:"independent_provider_evidence"`
	BufferedTimingClaimedAsTTFT    bool `json:"buffered_timing_claimed_as_ttft"`
	ChecksumValidClosedRegistryBad bool `json:"checksum_valid_closed_registry_invalid"`
	MissingRequired                bool `json:"missing_required"`
	StaleEvidence                  bool `json:"stale_evidence"`
	LocalRedactionReview           bool `json:"local_redaction_review"`
	ObservedTestsNotApplicable     bool `json:"observed_tests_not_applicable"`
	VerifierNotApplicable          bool `json:"verifier_not_applicable"`
	CleanupNotApplicable           bool `json:"cleanup_not_applicable"`
	ExternalEffectsNotApplicable   bool `json:"external_effects_not_applicable"`
}

type Usage struct {
	InputTokens         *int64 `json:"input_tokens"`
	OutputTokens        *int64 `json:"output_tokens"`
	InputTokensPresent  bool   `json:"-"`
	OutputTokensPresent bool   `json:"-"`
}

func (u Usage) InputState() string {
	if !u.InputTokensPresent && u.InputTokens == nil {
		return "missing"
	}
	if u.InputTokens == nil {
		return "null"
	}
	return "measured"
}

func (u Usage) OutputState() string {
	if !u.OutputTokensPresent && u.OutputTokens == nil {
		return "missing"
	}
	if u.OutputTokens == nil {
		return "null"
	}
	return "measured"
}

type Receipt struct {
	SchemaVersion               string      `json:"schema_version"`
	CallID                      string      `json:"call_id"`
	ScheduleIndex               int         `json:"schedule_index"`
	ConditionID                 string      `json:"condition_id"`
	RepositoryRole              string      `json:"repository_role"`
	Split                       string      `json:"split"`
	Arm                         string      `json:"arm"`
	Replicate                   int         `json:"replicate"`
	Status                      string      `json:"status"`
	Model                       string      `json:"model"`
	AuthorizationSHA256         string      `json:"authorization_sha256"`
	LedgerAttemptSHA256         string      `json:"ledger_attempt_sha256"`
	LedgerSettlementSHA256      string      `json:"ledger_settlement_sha256"`
	Decision                    *Decision   `json:"decision"`
	ProviderPolicyFacts         PolicyFacts `json:"provider_policy_facts"`
	IndependentPolicyFacts      PolicyFacts `json:"independent_policy_facts"`
	FactsAgreement              bool        `json:"facts_agreement"`
	IndependentDisposition      string      `json:"independent_disposition"`
	IndependentReasonCodes      []string    `json:"independent_reason_codes"`
	RequestSHA256               string      `json:"request_sha256"`
	RawResponse                 []byte      `json:"raw_response"`
	RawResponseKind             string      `json:"raw_response_kind"`
	ResponseSHA256              *string     `json:"response_sha256"`
	ParsedResponseSHA256        *string     `json:"parsed_response_sha256"`
	HTTPStatus                  int         `json:"http_status"`
	Usage                       Usage       `json:"usage"`
	LatencyNanoseconds          int64       `json:"latency_nanoseconds"`
	ProviderRequestID           *string     `json:"provider_request_id"`
	ProviderReportedCostNanoUSD *int64      `json:"provider_reported_cost_nano_usd"`
	InferredCostNanoUSD         *int64      `json:"inferred_cost_nano_usd"`
	CostBasis                   string      `json:"cost_basis"`
	ErrorCode                   *string     `json:"error_code"`
	RawError                    []byte      `json:"raw_error"`
	ErrorArtifactSHA256         *string     `json:"error_artifact_sha256"`
	ErrorMessageSHA256          *string     `json:"error_message_sha256"`
	StartedAt                   time.Time   `json:"started_at"`
	FinishedAt                  time.Time   `json:"finished_at"`
	ReceiptSHA256               string      `json:"receipt_sha256"`
}

type ReceiptBinding struct {
	CallID         string `json:"call_id"`
	ConditionID    string `json:"condition_id"`
	RepositoryRole string `json:"repository_role"`
	Split          string `json:"split"`
	Arm            string `json:"arm"`
	Replicate      int    `json:"replicate"`
}

type ReceiptRegistry struct {
	SchemaVersion        string           `json:"schema_version"`
	ReceiptSchemaSHA256  string           `json:"receipt_schema_sha256"`
	SourceRegistrySHA256 string           `json:"source_registry_sha256"`
	CorpusSHA256         string           `json:"corpus_sha256"`
	ScheduleSHA256       string           `json:"schedule_sha256"`
	Bindings             []ReceiptBinding `json:"bindings"`
	RegistrySHA256       string           `json:"registry_sha256"`
}
