package studylocal

const (
	SchemaVersion = "context-build-artifact/local-context-control/v1"

	DistillSourceCommit  = "a9b14667024c27c32c9c9dfb9c8dc35865990979"
	LLMTraceSourceCommit = "f9385e1d9ebf862ba46272fb8a45d7881b0b2a9c"

	TargetModelID             = "Qwen/Qwen3-4B"
	TargetModelRevision       = "1cfa9a7208912126459214e8b04321603b3df60c"
	TargetModelArtifactSHA256 = "057a37f4ebc76420f8ab2edb17bc8442e050c8d13f7334f829356e2f9cab6802"
	TargetModelFormat         = "mlx-affine-4bit-group64"

	ArmRaw         = "A"
	ArmDistillLock = "C"

	CorpusSeed         = "local-control-corpus-v1:2026-09-21"
	ConditionOrderSeed = "local-control-condition-order-v1:2026-09-21"
	ArmOrderSeed       = "local-control-arm-order-v1:2026-09-21"

	PrimaryObservationCount = 236
	RepeatConditionCount    = 24
	RepeatObservationCount  = 96
	ObservationsPerModel    = 332
)

type SourceAnchor struct {
	Dataset       string `json:"dataset"`
	BaseID        string `json:"base_id"`
	Repository    string `json:"repository"`
	Commit        string `json:"commit"`
	License       string `json:"license"`
	LicenseSHA256 string `json:"license_sha256"`
	Path          string `json:"path"`
	SHA256        string `json:"sha256"`
}

type EvidenceSource struct {
	ID     string `json:"id"`
	Path   string `json:"path"`
	Bytes  string `json:"bytes"`
	SHA256 string `json:"sha256"`
}

type Adjudication struct {
	Decision              string   `json:"decision"`
	SourceIDs             []string `json:"source_ids"`
	SourceSHA256          []string `json:"source_sha256"`
	SupportingEvidenceIDs []string `json:"supporting_evidence_ids"`
}

type Base struct {
	ID         string `json:"id"`
	Dataset    string `json:"dataset"`
	Role       string `json:"role"`
	Repository string `json:"repository"`
	Commit     string `json:"commit"`
}

type Condition struct {
	ID                 string           `json:"id"`
	BaseID             string           `json:"base_id"`
	Dataset            string           `json:"dataset"`
	Role               string           `json:"role"`
	Family             string           `json:"family"`
	Ordinal            int              `json:"ordinal"`
	ExpectedEquivalent bool             `json:"expected_equivalent"`
	TransformVersion   string           `json:"transform_version"`
	Seed               string           `json:"seed"`
	Sources            []EvidenceSource `json:"sources"`
	SourceSetSHA256    string           `json:"source_set_sha256"`
	TransformSHA256    string           `json:"transform_sha256"`
	Adjudication       Adjudication     `json:"adjudication"`
}

type Corpus struct {
	SchemaVersion           string         `json:"schema_version"`
	StudyKind               string         `json:"study_kind"`
	DevelopmentCalibration  bool           `json:"development_calibration"`
	FinalStudyEligible      bool           `json:"final_study_eligible"`
	HeldOutEvidenceIncluded bool           `json:"held_out_evidence_included"`
	ProviderCalls           int            `json:"provider_calls"`
	ExclusionStatement      string         `json:"exclusion_statement"`
	Bases                   []Base         `json:"bases"`
	Conditions              []Condition    `json:"conditions"`
	SourceRegistry          []SourceAnchor `json:"source_registry"`
	CorpusSHA256            string         `json:"corpus_sha256"`
}

type Context struct {
	ConditionID        string `json:"condition_id"`
	Arm                string `json:"arm"`
	Compiler           string `json:"compiler"`
	Content            string `json:"content"`
	ContentSHA256      string `json:"content_sha256"`
	LockSHA256         string `json:"lock_sha256,omitempty"`
	ManifestSHA256     string `json:"manifest_sha256,omitempty"`
	ChecksumsSHA256    string `json:"checksums_sha256,omitempty"`
	ConfigurationHash  string `json:"configuration_sha256,omitempty"`
	LockSourceCount    int    `json:"lock_source_count,omitempty"`
	LockChunkCount     int    `json:"lock_chunk_count,omitempty"`
	LockDuplicateCount int    `json:"lock_duplicate_count,omitempty"`
	LockSelectedCount  int    `json:"lock_selected_count,omitempty"`
	LockSelectedTokens int    `json:"lock_selected_tokens,omitempty"`
}

type ScheduleEntry struct {
	Index             int    `json:"index"`
	ObservationID     string `json:"observation_id"`
	ConditionID       string `json:"condition_id"`
	BaseID            string `json:"base_id"`
	Dataset           string `json:"dataset"`
	Arm               string `json:"arm"`
	Replicate         int    `json:"replicate"`
	RepeatSubset      bool   `json:"repeat_subset"`
	ContextSHA256     string `json:"context_sha256"`
	CorpusSHA256      string `json:"corpus_sha256"`
	ProtocolSHA256    string `json:"protocol_sha256"`
	ModelContractHash string `json:"model_contract_sha256"`
}

type GenerationConfig struct {
	Sampling             string   `json:"sampling"`
	Temperature          *float64 `json:"temperature"`
	Seed                 uint64   `json:"seed"`
	SeedGuarantee        string   `json:"seed_guarantee"`
	MaxOutputTokens      int      `json:"max_output_tokens"`
	ChatTemplate         string   `json:"chat_template"`
	ChatTemplateOptions  []string `json:"chat_template_options"`
	StopRules            []string `json:"stop_rules"`
	AttemptsPerScheduled int      `json:"attempts_per_scheduled_observation"`
}

type IsolationLimits struct {
	RequiredOS                       string `json:"required_os"`
	RequiredArchitecture             string `json:"required_architecture"`
	RequiredHardware                 string `json:"required_hardware"`
	RequiredPhysicalMemoryBytes      int64  `json:"required_physical_memory_bytes"`
	MinimumFreeMemoryPercent         int    `json:"minimum_free_memory_percent"`
	MaximumSwapUsedBytes             int64  `json:"maximum_swap_used_bytes"`
	MinimumDiskFreeBytes             int64  `json:"minimum_disk_free_bytes"`
	MaximumUnrelatedRSSBytes         int64  `json:"maximum_unrelated_process_rss_bytes"`
	DuringMinimumFreeMemoryPercent   int    `json:"during_minimum_free_memory_percent"`
	DuringMaximumSwapUsedBytes       int64  `json:"during_maximum_swap_used_bytes"`
	DuringMaximumSwapGrowthBytes     int64  `json:"during_maximum_swap_growth_bytes"`
	DuringMinimumDiskFreeBytes       int64  `json:"during_minimum_disk_free_bytes"`
	DuringMaximumProcessRSSBytes     int64  `json:"during_maximum_process_rss_bytes"`
	DuringMaximumMLXPeakBytes        int64  `json:"during_maximum_mlx_peak_bytes"`
	ObservationTimeoutSeconds        int    `json:"observation_timeout_seconds"`
	CleanupTimeoutSeconds            int    `json:"cleanup_timeout_seconds"`
	ProcessSampleIntervalSeconds     int    `json:"process_sample_interval_seconds"`
	RequireACPower                   bool   `json:"require_ac_power"`
	RequireSleepInhibition           bool   `json:"require_sleep_inhibition"`
	RequireNetworkDenySandbox        bool   `json:"require_network_deny_sandbox"`
	RequireMetalSystemTraceAvailable bool   `json:"require_metal_system_trace_available"`
}

type OutcomeRules struct {
	PositiveDecisionDeltaMinimum float64  `json:"positive_decision_delta_minimum"`
	NegativeDecisionDeltaMaximum float64  `json:"negative_decision_delta_maximum"`
	ReviewBurdenDeltaMinimum     float64  `json:"review_burden_delta_minimum"`
	ReviewBurdenDeltaMaximum     float64  `json:"review_burden_delta_maximum"`
	UnsafeAcceptDeltaMaximum     float64  `json:"unsafe_accept_delta_maximum"`
	MinimumValidPrimaryRate      float64  `json:"minimum_valid_primary_rate"`
	ClusterUnit                  string   `json:"cluster_unit"`
	ThresholdPolicy              string   `json:"threshold_policy"`
	RoutingUnsafeCeiling         float64  `json:"routing_unsafe_ceiling"`
	RoutingMinimumAccepted       int      `json:"routing_minimum_accepted"`
	RoutingRuleOrder             []string `json:"routing_rule_order"`
}

type Protocol struct {
	SchemaVersion                  string            `json:"schema_version"`
	Status                         string            `json:"status"`
	ProspectiveAmendment           string            `json:"prospective_amendment"`
	ProspectiveAmendmentSHA256     string            `json:"prospective_amendment_sha256"`
	StudyKind                      string            `json:"study_kind"`
	IndependentUnit                string            `json:"independent_unit"`
	ConditionRecordsClustered      bool              `json:"condition_records_clustered"`
	Arms                           []string          `json:"arms"`
	ArmDefinitions                 map[string]string `json:"arm_definitions"`
	BaseCount                      int               `json:"base_count"`
	ConditionCount                 int               `json:"condition_count"`
	PrimaryObservations            int               `json:"primary_observations_per_model"`
	RepeatConditionCount           int               `json:"repeat_condition_count"`
	RepeatObservations             int               `json:"repeat_observations_per_model"`
	ObservationsPerModel           int               `json:"observations_per_model"`
	RepeatConditionIDs             []string          `json:"repeat_condition_ids"`
	TargetModelID                  string            `json:"target_model_id"`
	TargetModelRevision            string            `json:"target_model_revision"`
	TargetModelFormat              string            `json:"target_model_format"`
	TargetModelArtifactSHA256      string            `json:"target_model_artifact_sha256"`
	RuntimeStatus                  string            `json:"runtime_status"`
	RetiredRuntimeSHA256           string            `json:"retired_runtime_identity_sha256"`
	RuntimePackageClosureSHA256    string            `json:"runtime_package_closure_sha256"`
	RuntimeRequirementsInputSHA256 string            `json:"runtime_requirements_input_sha256"`
	RuntimeRequirementsSHA256      string            `json:"runtime_requirements_lock_sha256"`
	RuntimeWheelVerificationSHA256 string            `json:"runtime_wheel_verification_sha256"`
	RuntimeVenvManifestSHA256      string            `json:"runtime_venv_manifest_sha256"`
	BasePythonBinarySHA256         string            `json:"base_python_binary_sha256"`
	BasePythonManifestSHA256       string            `json:"base_python_manifest_sha256"`
	ScalingControl                 string            `json:"scaling_control"`
	Generation                     GenerationConfig  `json:"generation"`
	ResponseProjection             string            `json:"response_projection"`
	ConfidenceSemantics            string            `json:"confidence_semantics"`
	MalformedOutputPolicy          string            `json:"malformed_output_policy"`
	NetworkPolicy                  string            `json:"network_policy"`
	Isolation                      IsolationLimits   `json:"isolation"`
	Measurements                   []string          `json:"measurements"`
	InferredMetrics                []string          `json:"inferred_metrics"`
	AnalysisPlan                   []string          `json:"analysis_plan"`
	PerformanceTracing             string            `json:"performance_tracing"`
	OutcomeRules                   OutcomeRules      `json:"outcome_rules"`
	SeparationStatement            string            `json:"separation_statement"`
	ProtocolSHA256                 string            `json:"protocol_sha256"`
}

type CategoricalQuestion struct {
	ID      string   `json:"id"`
	Text    string   `json:"text"`
	Allowed []string `json:"allowed"`
}

type AdapterRequest struct {
	SchemaVersion string                `json:"schema_version"`
	ObservationID string                `json:"observation_id"`
	SystemPrompt  string                `json:"system_prompt"`
	Context       string                `json:"context"`
	Questions     []CategoricalQuestion `json:"questions"`
	Generation    GenerationConfig      `json:"generation"`
}

type Projection struct {
	Answer          *string  `json:"answer"`
	Confidence      *float64 `json:"confidence"`
	ConfidenceBasis string   `json:"confidence_basis"`
	ParseStatus     string   `json:"parse_status"`
}

type PackageManifest struct {
	SchemaVersion       string `json:"schema_version"`
	CorpusSHA256        string `json:"corpus_sha256"`
	ProtocolSHA256      string `json:"protocol_sha256"`
	ContextsSHA256      string `json:"contexts_sha256"`
	ScheduleSHA256      string `json:"schedule_sha256"`
	ResultSchemaSHA256  string `json:"result_schema_sha256"`
	SourceRegistryHash  string `json:"source_registry_sha256"`
	PackageFileCount    int    `json:"package_file_count"`
	ProviderCalls       int    `json:"provider_calls"`
	ExecutionAuthorized bool   `json:"execution_authorized"`
}

type Package struct {
	Corpus   Corpus
	Protocol Protocol
	Contexts []Context
	Schedule []ScheduleEntry
	Manifest PackageManifest
	Files    map[string][]byte
}

type Summary struct {
	SchemaVersion         string `json:"schema_version"`
	Bases                 int    `json:"bases"`
	DistillBases          int    `json:"distill_bases"`
	LLMTraceFXBases       int    `json:"llmtracefx_bases"`
	Conditions            int    `json:"conditions"`
	DistillConditions     int    `json:"distill_conditions"`
	LLMTraceFXConditions  int    `json:"llmtracefx_conditions"`
	PrimaryObservations   int    `json:"primary_observations_per_model"`
	RepeatConditions      int    `json:"repeat_conditions"`
	RepeatObservations    int    `json:"repeat_observations_per_model"`
	ObservationsPerModel  int    `json:"observations_per_model"`
	CorpusSHA256          string `json:"corpus_sha256"`
	ProtocolSHA256        string `json:"protocol_sha256"`
	ContextsSHA256        string `json:"contexts_sha256"`
	ScheduleSHA256        string `json:"schedule_sha256"`
	ExecutionAuthorized   bool   `json:"execution_authorized"`
	ScalingControlEnabled bool   `json:"scaling_control_enabled"`
	HeldOutRecords        int    `json:"held_out_records"`
	ProviderCalls         int    `json:"provider_calls"`
}
