package studylocalv2

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"strings"
)

const (
	PublicAggregateFilename = "local-control-public-aggregate-v1.json"
	PublicReportFilename    = "local-control-result-v2.md"
	PublicChecksumsFilename = "local-control-public-evidence-v1.SHA256SUMS"

	KnownPublicAggregateSHA256 = "452890410706cabb57b6580b598171052a22c2a27cef98c2da3ccdfe481d1f47"
	KnownPublicReportSHA256    = "df1b1ab70e9b27a0bb902917c00081701b2ee85730007879818d192b224cf571"
)

type publicStudy struct {
	AttemptsConsumed                         int    `json:"attempts_consumed"`
	DevelopmentCalibrationOnly               bool   `json:"development_calibration_only"`
	ExecutionStatus                          string `json:"execution_status"`
	ImplementationMergeDistinctFromExecution bool   `json:"implementation_merge_distinct_from_execution"`
	ModelArtifact                            string `json:"model_artifact"`
	Outcome                                  string `json:"outcome"`
	Protocol                                 string `json:"protocol"`
}

type publicDesign struct {
	Conditions                 int `json:"conditions"`
	DistillBases               int `json:"distill_bases"`
	IndependentBases           int `json:"independent_bases"`
	LLMTraceFXBases            int `json:"llmtracefx_bases"`
	PrimaryObservations        int `json:"primary_observations"`
	RepeatArmPairs             int `json:"repeat_arm_pairs"`
	RepeatObservations         int `json:"repeat_observations"`
	TotalScheduledObservations int `json:"total_scheduled_observations"`
}

type publicCounts struct {
	AdapterErrors int `json:"adapter_errors"`
	MalformedJSON int `json:"malformed_json"`
	Scheduled     int `json:"scheduled"`
	Valid         int `json:"valid"`
}

type publicArmCounts struct {
	ArmA publicCounts `json:"arm_a"`
	ArmC publicCounts `json:"arm_c"`
}

type publicObservationCounts struct {
	All                 publicCounts    `json:"all"`
	AllIncludingRepeats publicArmCounts `json:"all_including_repeats"`
	Primary             publicArmCounts `json:"primary"`
}

type publicMalformedScoring struct {
	Correctness  bool   `json:"correctness"`
	Mechanism    string `json:"mechanism"`
	Review       bool   `json:"review"`
	UnsafeAccept bool   `json:"unsafe_accept"`
}

type publicMetric struct {
	ArmA         float64 `json:"arm_a"`
	ArmC         float64 `json:"arm_c"`
	DeltaCMinusA float64 `json:"delta_c_minus_a"`
}

type publicCorrectnessMetric struct {
	publicMetric
	ConventionalSignificance bool    `json:"conventional_significance"`
	ExactTwoSidedSignFlipP   float64 `json:"exact_two_sided_sign_flip_p"`
	PValueScope              string  `json:"p_value_scope"`
}

type publicUnsafeAcceptMetric struct {
	publicMetric
	HarmThreshold    float64  `json:"harm_threshold"`
	ThresholdCrossed bool     `json:"threshold_crossed"`
	PValue           *float64 `json:"p_value"`
}

type publicOutcomes struct {
	Correctness      publicCorrectnessMetric  `json:"equal_weight_correctness"`
	ReviewBurden     publicMetric             `json:"review_burden"`
	UnsafeAcceptance publicUnsafeAcceptMetric `json:"unsafe_acceptance"`
}

type publicRange struct {
	Maximum float64 `json:"maximum"`
	Minimum float64 `json:"minimum"`
}

type publicLeaveOneBaseOut struct {
	CorrectnessDeltaCMinusA  publicRange `json:"correctness_delta_c_minus_a"`
	UnsafeAcceptDeltaCMinusA publicRange `json:"unsafe_accept_delta_c_minus_a"`
}

type publicThresholdDevelopment struct {
	CalibratedConfidenceClaim bool     `json:"calibrated_confidence_claim"`
	Confidence                *float64 `json:"confidence"`
	SelectedRule              string   `json:"selected_rule"`
}

type publicRepeatability struct {
	DecisionAgreement         float64 `json:"decision_agreement"`
	EvidenceIdentifierJaccard float64 `json:"evidence_identifier_jaccard"`
	Interpretation            string  `json:"interpretation"`
	MalformedAgreement        float64 `json:"malformed_agreement"`
	ModelDeterminismClaim     bool    `json:"model_determinism_claim"`
	RepeatedConditionArmPairs int     `json:"repeated_condition_arm_pairs"`
	RouteAgreement            float64 `json:"route_agreement"`
}

type publicEfficiency struct {
	CausalComparison                    bool    `json:"causal_comparison"`
	HostConditioning                    string  `json:"host_conditioning"`
	InputTokenRatioCToA                 float64 `json:"input_token_ratio_c_to_a"`
	MeanRequestLatencyChangePercentCToA float64 `json:"mean_request_latency_change_percent_c_to_a"`
	QualityPhaseMetalTrace              *string `json:"quality_phase_metal_trace"`
}

type publicImplementationIntegrity struct {
	CIChecksPassed int    `json:"ci_checks_passed"`
	MergeCommit    string `json:"merge_commit"`
	MergeTree      string `json:"merge_tree"`
	PullRequest    int    `json:"pull_request"`
}

type publicFrozenIdentities struct {
	ContextsSHA256     string `json:"contexts_sha256"`
	CorpusSHA256       string `json:"corpus_sha256"`
	FramingSHA256      string `json:"framing_sha256"`
	PackageSHA256      string `json:"package_sha256"`
	ProtocolSHA256     string `json:"protocol_sha256"`
	ResultSchemaSHA256 string `json:"result_schema_sha256"`
	ScheduleSHA256     string `json:"schedule_sha256"`
}

type publicIntegrity struct {
	AdapterSHA256                   string                        `json:"adapter_sha256"`
	AttemptLedgerEntrySHA256        string                        `json:"attempt_ledger_entry_sha256"`
	AuthorizationSHA256             string                        `json:"authorization_sha256"`
	ExecutionReportSHA256           string                        `json:"execution_report_sha256"`
	ExactRunnerVerification         string                        `json:"exact_runner_verification"`
	FrozenIdentities                publicFrozenIdentities        `json:"frozen_identities"`
	Implementation                  publicImplementationIntegrity `json:"implementation"`
	ModelAndRuntimeCustody          string                        `json:"model_and_runtime_custody"`
	ReceiptCollectionSHA256         string                        `json:"receipt_collection_sha256"`
	RegeneratedSummaryByteIdentical bool                          `json:"regenerated_summary_byte_identical"`
	ResultSummarySHA256             string                        `json:"result_summary_sha256"`
	RunnerSHA256                    string                        `json:"runner_sha256"`
}

type publicFailureEvent struct {
	AttemptConsumed      bool   `json:"attempt_consumed"`
	AuthorizationCreated bool   `json:"authorization_created"`
	Classification       string `json:"classification"`
	Detail               string `json:"detail"`
	Gate                 string `json:"gate"`
	ModelLoads           *int   `json:"model_loads"`
	Namespace            string `json:"namespace"`
	Observations         *int   `json:"observations"`
	Sequence             int    `json:"sequence"`
}

type publicClaimBoundaries struct {
	CalibratedConfidenceClaim         *string `json:"calibrated_confidence_claim"`
	Conclusion                        string  `json:"conclusion"`
	CorrectnessSignificanceClaim      bool    `json:"correctness_significance_claim"`
	GeneralModelDeterminismClaim      *string `json:"general_model_determinism_claim"`
	LargerModelComparison             *string `json:"larger_model_comparison"`
	ScalingClaim                      *string `json:"scaling_claim"`
	UnconditionalPerformanceBenchmark *string `json:"unconditional_performance_benchmark"`
}

type PublicAggregate struct {
	ClaimBoundaries       publicClaimBoundaries      `json:"claim_boundaries"`
	DescriptiveEfficiency publicEfficiency           `json:"descriptive_efficiency"`
	Design                publicDesign               `json:"design"`
	FailureChronology     []publicFailureEvent       `json:"failure_chronology"`
	Integrity             publicIntegrity            `json:"integrity"`
	LeaveOneBaseOut       publicLeaveOneBaseOut      `json:"leave_one_base_out"`
	MalformedScoring      publicMalformedScoring     `json:"malformed_scoring"`
	ObservationCounts     publicObservationCounts    `json:"observation_counts"`
	Outcomes              publicOutcomes             `json:"outcomes"`
	RecordKind            string                     `json:"record_kind"`
	Repeatability         publicRepeatability        `json:"repeatability"`
	SchemaName            string                     `json:"schema_name"`
	SchemaVersion         int                        `json:"schema_version"`
	Study                 publicStudy                `json:"study"`
	ThresholdDevelopment  publicThresholdDevelopment `json:"threshold_development"`
}

func expectedPublicAggregate() PublicAggregate {
	zero := 0
	return PublicAggregate{
		ClaimBoundaries: publicClaimBoundaries{
			Conclusion:                   "Compiled context made the pinned local Qwen3-4B more reliably answerable, not reliably safe.",
			CorrectnessSignificanceClaim: false,
		},
		DescriptiveEfficiency: publicEfficiency{
			CausalComparison: false, HostConditioning: "quiescent-host-conditioned",
			InputTokenRatioCToA:                 2.462188731814656,
			MeanRequestLatencyChangePercentCToA: 3.060386754493316,
		},
		Design: publicDesign{
			Conditions: 118, DistillBases: 7, IndependentBases: 14, LLMTraceFXBases: 7,
			PrimaryObservations: 236, RepeatArmPairs: 48, RepeatObservations: 96,
			TotalScheduledObservations: 332,
		},
		FailureChronology: []publicFailureEvent{
			{
				AttemptConsumed: true, AuthorizationCreated: true,
				Classification: "invalid_pre_observation_startup_failure",
				Detail:         "The v1 startup failure occurred before observation. Rerun is prohibited.",
				Gate:           "adapter_startup", ModelLoads: &zero, Namespace: "v1", Observations: &zero, Sequence: 1,
			},
			{
				Classification: "NO_RUN",
				Detail:         "Colima met or exceeded the frozen 1 GiB unrelated-process RSS limit. The preparation stopped before authorization, model load, or inference.",
				Gate:           "readiness", ModelLoads: &zero, Namespace: "v2", Observations: &zero, Sequence: 2,
			},
			{
				Classification: "NO_RUN",
				Detail:         "The adapter check rejected Go nil slices encoded as JSON null instead of empty arrays. The preparation stopped before authorization, model load, or inference.",
				Gate:           "adapter_check", ModelLoads: &zero, Namespace: "v2", Observations: &zero, Sequence: 3,
			},
			{
				Classification: "outcome_independent_implementation_amendment",
				Detail:         "The producer correction changed implementation framing only. Frozen scientific identities remained unchanged.",
				Gate:           "implementation_review", Namespace: "v2_amendment_1", Sequence: 4,
			},
			{
				AttemptConsumed: true, AuthorizationCreated: true,
				Classification: "completed_valid_negative_result",
				Detail:         "One authorized attempt completed with 332 observations.",
				Gate:           "completed_run", Namespace: "v2_amendment_1", Observations: intPointer(332), Sequence: 5,
			},
		},
		Integrity: publicIntegrity{
			AdapterSHA256:            "9c85bd25470452c6564153756597f3636c8268d6eff3c90e66632b32e91f637e",
			AttemptLedgerEntrySHA256: "46196495ff620e5581b5f3a21c756c16740e888d7ce305ebfbe625540f65d8c1",
			AuthorizationSHA256:      "e2f3461d05b20327acd38672b0b9564d62b9fca22942fe9e92a29dbe8323bfa8",
			ExecutionReportSHA256:    "1fcf50d28d4dadb3997de77eff88db918b9e06e52d177aa9dae1c4b034913e7b",
			ExactRunnerVerification:  "passed",
			FrozenIdentities: publicFrozenIdentities{
				ContextsSHA256:     "d97859a33500b10453bef8869e43327ed1e33f981865e5e35dceaaaff1e0f707",
				CorpusSHA256:       "222d1a4f021022fdb048f705bf01fd3001833f441fc2f800ad960c41a174ffdc",
				FramingSHA256:      "d864b23a23ff10402bc44a9261b31903077f6f7915a2374db7c4e122caaedae3",
				PackageSHA256:      "c0ad25e2c16bb38bf80b1164696580367836248bcd4a9697d7e5e91b7a943f85",
				ProtocolSHA256:     "d1551d992e55c112b4e6e18fea5a018bdb360969947f2f4a45148a3ac50a1b27",
				ResultSchemaSHA256: "3f98d5c5427f46192fbf4413d90119e32fc618bd00679a19a32a0d742196ac11",
				ScheduleSHA256:     "46ab21daf6d64eda431364cdf2c8e81cd5fce0d2dd38321a6dd5863e991edbf0",
			},
			Implementation: publicImplementationIntegrity{
				CIChecksPassed: 12,
				MergeCommit:    "670a48a993e7db129d6b75b84516d90977b1c460",
				MergeTree:      "81b4a875473325c0d09894e1a1c175f8b6a3da6b",
				PullRequest:    109,
			},
			ModelAndRuntimeCustody:          "reverified_after_run",
			ReceiptCollectionSHA256:         "207eaea28ffc7238899e7bc037fb6b1c43cabb147f41bda1a525a79a75ea8970",
			RegeneratedSummaryByteIdentical: true,
			ResultSummarySHA256:             "bea16fbb9fe5de39bb128be76f3874a3faecdc3c59c3cc7073743afa259130f4",
			RunnerSHA256:                    "c4bd2851fa1aa0006f949cfa4dd30ba5ee28a37c0a3741dad28c38568153486e",
		},
		LeaveOneBaseOut: publicLeaveOneBaseOut{
			CorrectnessDeltaCMinusA:  publicRange{Minimum: 0.0478, Maximum: 0.1096},
			UnsafeAcceptDeltaCMinusA: publicRange{Minimum: 0.1163, Maximum: 0.1958},
		},
		MalformedScoring: publicMalformedScoring{
			Correctness: false,
			Mechanism:   "Compilation removed malformed-as-review abstentions.",
			Review:      true, UnsafeAccept: false,
		},
		ObservationCounts: publicObservationCounts{
			All: publicCounts{AdapterErrors: 0, MalformedJSON: 61, Scheduled: 332, Valid: 271},
			AllIncludingRepeats: publicArmCounts{
				ArmA: publicCounts{AdapterErrors: 0, MalformedJSON: 61, Scheduled: 166, Valid: 105},
				ArmC: publicCounts{AdapterErrors: 0, MalformedJSON: 0, Scheduled: 166, Valid: 166},
			},
			Primary: publicArmCounts{
				ArmA: publicCounts{AdapterErrors: 0, MalformedJSON: 43, Scheduled: 118, Valid: 75},
				ArmC: publicCounts{AdapterErrors: 0, MalformedJSON: 0, Scheduled: 118, Valid: 118},
			},
		},
		Outcomes: publicOutcomes{
			Correctness: publicCorrectnessMetric{
				publicMetric:             publicMetric{ArmA: 0.28354978354978355, ArmC: 0.3733766233766234, DeltaCMinusA: 0.08982683982683984},
				ConventionalSignificance: false, ExactTwoSidedSignFlipP: 0.09375,
				PValueScope: "correctness_only",
			},
			ReviewBurden: publicMetric{ArmA: 0.5720779220779221, ArmC: 0.3093073593073593, DeltaCMinusA: -0.26277056277056277},
			UnsafeAcceptance: publicUnsafeAcceptMetric{
				publicMetric:  publicMetric{ArmA: 0.26774891774891774, ArmC: 0.43528138528138527, DeltaCMinusA: 0.16753246753246756},
				HarmThreshold: 0.05, ThresholdCrossed: true,
			},
		},
		RecordKind: "public_aggregate_only",
		Repeatability: publicRepeatability{
			DecisionAgreement: 1, EvidenceIdentifierJaccard: 1,
			Interpretation: "realized_repeatability_only", MalformedAgreement: 1,
			ModelDeterminismClaim: false, RepeatedConditionArmPairs: 48, RouteAgreement: 1,
		},
		SchemaName:    "local-context-control-public-aggregate",
		SchemaVersion: 1,
		Study: publicStudy{
			AttemptsConsumed: 1, DevelopmentCalibrationOnly: true,
			ExecutionStatus:                          "valid_negative_result",
			ImplementationMergeDistinctFromExecution: true,
			ModelArtifact:                            "pinned local Qwen3-4B", Outcome: "negative",
			Protocol: "local-context-control-v2-amendment-1",
		},
		ThresholdDevelopment: publicThresholdDevelopment{
			CalibratedConfidenceClaim: false, SelectedRule: "no-safe-auto-action",
		},
	}
}

func intPointer(value int) *int {
	return &value
}

func ValidatePublicEvidence(directory string) error {
	recordBytes, err := os.ReadFile(filepath.Join(directory, PublicAggregateFilename))
	if err != nil {
		return err
	}
	reportBytes, err := os.ReadFile(filepath.Join(directory, PublicReportFilename))
	if err != nil {
		return err
	}
	checksumBytes, err := os.ReadFile(filepath.Join(directory, PublicChecksumsFilename))
	if err != nil {
		return err
	}
	for name, data := range map[string][]byte{
		PublicAggregateFilename: recordBytes,
		PublicReportFilename:    reportBytes,
		PublicChecksumsFilename: checksumBytes,
	} {
		if bytes.Contains(data, []byte("\r")) || bytes.Contains(data, []byte("\u2014")) {
			return fmt.Errorf("%s contains forbidden line endings or em dash", name)
		}
		if err := rejectPrivateFragments(name, data); err != nil {
			return err
		}
	}
	if DigestBytes(recordBytes) != KnownPublicAggregateSHA256 {
		return fmt.Errorf("public aggregate SHA-256 drifted")
	}
	if DigestBytes(reportBytes) != KnownPublicReportSHA256 {
		return fmt.Errorf("public report SHA-256 drifted")
	}
	var record PublicAggregate
	if err := strictJSONObjectFileDecode(recordBytes, &record); err != nil {
		return fmt.Errorf("public aggregate: %w", err)
	}
	if err := rejectPrivateJSONFields(recordBytes); err != nil {
		return err
	}
	if !reflect.DeepEqual(record, expectedPublicAggregate()) {
		return fmt.Errorf("public aggregate values drifted")
	}
	expectedBytes, err := canonicalJSONFile(expectedPublicAggregate())
	if err != nil {
		return err
	}
	if !bytes.Equal(recordBytes, expectedBytes) {
		return fmt.Errorf("public aggregate is not the frozen canonical record")
	}
	expectedChecksums := fmt.Sprintf(
		"%s  %s\n%s  %s\n",
		KnownPublicAggregateSHA256, PublicAggregateFilename,
		KnownPublicReportSHA256, PublicReportFilename,
	)
	if string(checksumBytes) != expectedChecksums {
		return fmt.Errorf("public evidence checksum manifest drifted")
	}
	return nil
}

func rejectPrivateJSONFields(data []byte) error {
	var value any
	if err := json.Unmarshal(bytes.TrimSuffix(data, []byte{'\n'}), &value); err != nil {
		return err
	}
	forbidden := map[string]bool{
		"authorization_content": true, "authorization_nonce": true,
		"cache_path": true, "completion": true, "completions": true,
		"evidence_id": true, "evidence_ids": true, "local_path": true,
		"local_username": true, "nonce": true, "pid": true,
		"process_id": true, "prompt": true, "prompts": true,
		"question": true, "question_id": true, "question_level_outputs": true,
		"raw_completion": true, "raw_prompt": true, "raw_response": true,
		"timestamp": true, "timestamps": true, "username": true,
	}
	var walk func(any) error
	walk = func(current any) error {
		switch typed := current.(type) {
		case map[string]any:
			for key, child := range typed {
				if forbidden[strings.ToLower(key)] {
					return fmt.Errorf("public aggregate contains forbidden private field %q", key)
				}
				if err := walk(child); err != nil {
					return err
				}
			}
		case []any:
			for _, child := range typed {
				if err := walk(child); err != nil {
					return err
				}
			}
		}
		return nil
	}
	return walk(value)
}

func rejectPrivateFragments(name string, data []byte) error {
	lower := bytes.ToLower(data)
	for _, fragment := range [][]byte{
		[]byte("/users/"), []byte("/home/"), []byte("/private/"),
		[]byte(`\users\`), []byte("library/caches"),
		[]byte("authorization_nonce"), []byte("attempt_nonce"),
		[]byte("raw_completion"), []byte("raw_prompt"),
	} {
		if bytes.Contains(lower, fragment) {
			return fmt.Errorf("%s contains forbidden private fragment %q", name, fragment)
		}
	}
	return nil
}
