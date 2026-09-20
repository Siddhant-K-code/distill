package jevpilot

import (
	"time"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
)

const (
	AuthorizationSchema = "context-build-artifact/typesafe-jev-authorization/v1"
	ProviderSchema      = "context-build-artifact/typesafe-jev-provider/v1"
	ScheduleSchema      = "context-build-artifact/typesafe-jev-execution-schedule/v1"
	LedgerSchema        = "context-build-artifact/typesafe-jev-call-ledger/v1"

	ProviderName       = "TypeSafe AI"
	ModelID            = "jev-1.13.0"
	ModelVersion       = "1.13.0"
	APIEndpoint        = "https://api.typesafe.ai/v1/systemone"
	APIVersion         = "v1"
	PricingVersion     = "typesafe-models-docs-2026-09-20"
	InputNanoUSD       = int64(42)
	MaxInputTokens     = int64(64_000)
	AuthorizedNanoUSD  = int64(5_000_000_000)
	WorstCallNanoUSD   = MaxInputTokens * InputNanoUSD
	RequestTimeout     = 30 * time.Second
	ResponseLimitBytes = 2 << 20
)

type EvidenceSource struct {
	URL         string `json:"url"`
	RetrievedAt string `json:"retrieved_at"`
	SHA256      string `json:"sha256"`
}

type ExternalGates struct {
	SyntheticPublicDataSubmission string `json:"synthetic_public_data_submission"`
	PublicationIndependence       string `json:"publication_independence"`
	HeldOutNonTuning              string `json:"held_out_non_tuning"`
	DataRetention                 string `json:"data_retention"`
	TrainingUse                   string `json:"training_use"`
	ProviderSideSpendCap          string `json:"provider_side_spend_cap"`
}

type ProviderRecord struct {
	SchemaVersion              string           `json:"schema_version"`
	Provider                   string           `json:"provider"`
	ModelID                    string           `json:"model_id"`
	ModelVersion               string           `json:"model_version"`
	ModelListAliases           []string         `json:"model_list_aliases"`
	ModelListResponseSHA256    string           `json:"model_list_response_sha256"`
	ModelListRequestIDRecorded bool             `json:"model_list_request_id_recorded"`
	APIEndpoint                string           `json:"api_endpoint"`
	APIVersion                 string           `json:"api_version"`
	HTTPClient                 string           `json:"http_client"`
	SDKUsedForExecution        bool             `json:"sdk_used_for_execution"`
	ReferenceSDK               string           `json:"reference_sdk"`
	ReferenceSDKVersion        string           `json:"reference_sdk_version"`
	ReferenceSDKCommit         string           `json:"reference_sdk_commit"`
	Authentication             string           `json:"authentication"`
	RetryPolicy                string           `json:"retry_policy"`
	TimeoutSeconds             int              `json:"timeout_seconds"`
	InputPriceUSDPerMillion    string           `json:"input_price_usd_per_million"`
	OutputPriceUSDPerMillion   string           `json:"output_price_usd_per_million"`
	MaxInputTokensPerCall      int64            `json:"max_input_tokens_per_call"`
	WorstCaseCostUSDPerCall    string           `json:"worst_case_cost_usd_per_call"`
	RateLimits                 string           `json:"rate_limits"`
	UsageFields                []string         `json:"usage_fields"`
	ProviderCostField          string           `json:"provider_cost_field"`
	RequestIDField             string           `json:"request_id_field"`
	ConfidenceSemantics        string           `json:"confidence_semantics"`
	KnownJaggedEdges           []string         `json:"known_jagged_edges"`
	ExternalGates              ExternalGates    `json:"external_gates"`
	Evidence                   []EvidenceSource `json:"evidence"`
	FinalStudyEligible         bool             `json:"final_study_eligible"`
}

type ExecutionScheduleEntry struct {
	SchemaVersion   string `json:"schema_version"`
	ScheduleIndex   int    `json:"schedule_index"`
	ScheduledCallID string `json:"scheduled_call_id"`
	RequestID       string `json:"request_id"`
	CaseID          string `json:"case_id"`
	Category        string `json:"category"`
	ReplicateIndex  int    `json:"replicate_index"`
}

type Authorization struct {
	SchemaVersion           string   `json:"schema_version"`
	CreatedAt               string   `json:"created_at"`
	Stage                   string   `json:"stage"`
	Excluded                bool     `json:"excluded"`
	FinalStudyEligible      bool     `json:"final_study_eligible"`
	AuthorizedBudgetUSD     string   `json:"authorized_budget_usd"`
	AuthorizedBudgetNanoUSD int64    `json:"authorized_budget_nano_usd"`
	ModelID                 string   `json:"model_id"`
	PricingVersion          string   `json:"pricing_version"`
	InputPriceUSDPerMillion string   `json:"input_price_usd_per_million"`
	MaxInputTokensPerCall   int64    `json:"max_input_tokens_per_call"`
	WorstCaseCostUSDPerCall string   `json:"worst_case_cost_usd_per_call"`
	ScheduledCalls          int      `json:"scheduled_calls"`
	MaxScheduledCostUSD     string   `json:"max_scheduled_cost_usd"`
	ScheduleSHA256          string   `json:"schedule_sha256"`
	OfflineChecksumsSHA256  string   `json:"offline_checksums_sha256"`
	OfflineManifestSHA256   string   `json:"offline_manifest_sha256"`
	RequestsSHA256          string   `json:"requests_sha256"`
	ProviderRecordSHA256    string   `json:"provider_record_sha256"`
	ZeroAdaptiveExtension   bool     `json:"zero_adaptive_extension"`
	StopBehavior            string   `json:"stop_behavior"`
	RepeatabilityDesign     string   `json:"repeatability_design"`
	RepeatabilityCaseIDs    []string `json:"repeatability_case_ids"`
	UnresolvedExternalGates []string `json:"unresolved_external_gates"`
}

type APIQuestion struct {
	Type         string         `json:"type"`
	Instructions string         `json:"instructions"`
	Criteria     map[string]any `json:"criteria"`
}

type APIRequest struct {
	State     string                 `json:"state"`
	Model     string                 `json:"model"`
	Questions map[string]APIQuestion `json:"questions"`
}

type ChoiceAnswer struct {
	Type          string             `json:"type"`
	Choice        string             `json:"choice"`
	Probabilities map[string]float64 `json:"probabilities"`
	Confidence    float64            `json:"confidence"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type APIResponse struct {
	Model   string                  `json:"model"`
	Answers map[string]ChoiceAnswer `json:"answers"`
	Usage   Usage                   `json:"usage"`
}

type ResponseMetadata struct {
	HTTPStatus        int    `json:"http_status"`
	ProviderRequestID string `json:"provider_request_id"`
}

type LedgerEntry struct {
	SchemaVersion           string  `json:"schema_version"`
	ScheduleIndex           int     `json:"schedule_index"`
	ScheduledCallID         string  `json:"scheduled_call_id"`
	CaseID                  string  `json:"case_id"`
	ReplicateIndex          int     `json:"replicate_index"`
	Status                  string  `json:"status"`
	RequestSHA256           string  `json:"request_sha256"`
	ResponseSHA256          *string `json:"response_sha256"`
	ReceiptSHA256           string  `json:"receipt_sha256"`
	InputTokens             *int    `json:"input_tokens"`
	OutputTokens            *int    `json:"output_tokens"`
	ProviderReportedCostUSD *string `json:"provider_reported_cost_usd"`
	InferredCostUSD         *string `json:"inferred_cost_usd"`
	CumulativeInferredUSD   string  `json:"cumulative_inferred_cost_usd"`
	LatencyMilliseconds     float64 `json:"latency_ms"`
	ProviderRequestID       *string `json:"provider_request_id"`
}

type pilotData struct {
	Cases     []studypilot.CaseRecord
	Requests  []studypilot.RequestRecord
	Schedules []studypilot.ScheduleRecord
	Files     map[string][]byte
}

type callResult struct {
	RequestBody  []byte
	Response     *APIResponse
	RawBody      []byte
	Metadata     ResponseMetadata
	StartedAt    time.Time
	FinishedAt   time.Time
	Err          error
	ErrorStage   string
	ErrorCode    string
	NotAttempted bool
}

type receiptMaterial struct {
	Receipt   []byte
	Registry  studypilot.ReceiptRegistry
	Artifacts map[string][]byte
}
