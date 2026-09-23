package studylocal

import (
	"fmt"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

func BuildProtocol() (Protocol, error) {
	protocol := Protocol{
		SchemaVersion: SchemaVersion + "/protocol", Status: "prospective_frozen_no_observations",
		ProspectiveAmendment:       "research/context-is-a-build-artifact/local-control-amendment-v1.md",
		ProspectiveAmendmentSHA256: contextartifact.LocalControlAmendmentSHA256,
		StudyKind:                  "development_calibration_local_open_model", IndependentUnit: "base",
		ConditionRecordsClustered: true, Arms: []string{ArmRaw, ArmDistillLock},
		ArmDefinitions: map[string]string{
			ArmRaw:         "Validate UTF-8 and emit sources in submitted manifest order with the frozen ASCII BEGIN/END wrappers; perform no normalization, deduplication, chunking, ranking, or truncation.",
			ArmDistillLock: "Compile the identical source allowlist with the reviewed pkg/lock Distill Lock v0 implementation using 1024-byte UTF-8 chunks, an 8192 estimated-token budget, exact normalized-content deduplication, lexical selection, and verified bundle/lock/manifest/checksum artifacts.",
		},
		BaseCount: 14, ConditionCount: 118, PrimaryObservations: PrimaryObservationCount,
		RepeatConditionCount: RepeatConditionCount, RepeatObservations: RepeatObservationCount,
		ObservationsPerModel: ObservationsPerModel, RepeatConditionIDs: RepeatConditionIDs(),
		TargetModelID: TargetModelID, TargetModelRevision: TargetModelRevision,
		TargetModelFormat: TargetModelFormat, TargetModelArtifactSHA256: TargetModelArtifactSHA256,
		RuntimeStatus:                  "fresh_runtime_frozen_execution_requires_clean_host_preflight",
		RetiredRuntimeSHA256:           RuntimeIdentitySHA256,
		RuntimePackageClosureSHA256:    RuntimeClosureSHA256,
		RuntimeRequirementsInputSHA256: "4f6fad6a9e96d8ee1efdf59f1bd423c4425647c6d72db83ab9d335f5fea7c21b",
		RuntimeRequirementsSHA256:      "779c01eeabc50358a94c00d204dbe02526c19165505fd0904c1ed37583a8fee1",
		RuntimeWheelVerificationSHA256: RuntimeWheelVerificationSHA256,
		RuntimeVenvManifestSHA256:      RuntimeVenvManifestSHA256,
		BasePythonBinarySHA256:         BasePythonBinarySHA256,
		BasePythonManifestSHA256:       BasePythonManifestSHA256,
		ScalingControl:                 "disabled_in_v1_pending_separate_pinned_model_hardware_clean_preflight_and_explicit_spending_authorization",
		Generation: GenerationConfig{
			Sampling: "greedy_mlx_make_sampler_temp_0", Temperature: floatPointer(0), Seed: 20260921,
			SeedGuarantee:        "Seed is passed and recorded, but MLX seeded determinism is not assumed; the 24-condition repeat subset measures realized stability.",
			MaxOutputTokens:      256,
			ChatTemplate:         "Use the exact tokenizer_config.json chat_template bound by the verified model artifact.",
			ChatTemplateOptions:  []string{"tokenize=true", "add_generation_prompt=true", "enable_thinking=false"},
			StopRules:            []string{"model_eos_token_only", "no_client_substring_stop", "preserve_all_generated_bytes"},
			AttemptsPerScheduled: 1,
		},
		ResponseProjection:    `Each observation is one strict categorical projection {"answer":"<allowed-enum>"}. The finite enum combines disposition and the sorted cited-evidence subset as decision|E01,E02, or decision|- for no citation. No other key is allowed.`,
		ConfidenceSemantics:   "unavailable; the wrapper records confidence=null and confidence_basis=unavailable and no confidence threshold is used",
		MalformedOutputPolicy: "Preserve raw bytes and record one terminal malformed observation; never repair, extract, retry, censor, or replace.",
		NetworkPolicy:         "prepare, validate, summarize, authorize, run, result verification, and result summarization perform no network access; model execution requires a deny-network OS sandbox and offline runtime flags.",
		Isolation: IsolationLimits{
			RequiredOS: "macOS 27.0 build 26A428", RequiredArchitecture: "arm64",
			RequiredHardware: "Apple M5 Pro", RequiredPhysicalMemoryBytes: 24 << 30,
			MinimumFreeMemoryPercent: 25, MaximumSwapUsedBytes: 12 << 30,
			MinimumDiskFreeBytes: 20 << 30, MaximumUnrelatedRSSBytes: 1 << 30,
			DuringMinimumFreeMemoryPercent: 15, DuringMaximumSwapUsedBytes: 14 << 30,
			DuringMaximumSwapGrowthBytes: 2 << 30, DuringMinimumDiskFreeBytes: 12 << 30,
			DuringMaximumProcessRSSBytes: 12 << 30, DuringMaximumMLXPeakBytes: 8 << 30,
			ObservationTimeoutSeconds: 12 * 60, CleanupTimeoutSeconds: 5,
			ProcessSampleIntervalSeconds: 2, RequireACPower: true,
			RequireSleepInhibition: true, RequireNetworkDenySandbox: true,
			RequireMetalSystemTraceAvailable: false,
		},
		Measurements: []string{
			"decision correctness against digest-verified independent adjudication",
			"repeat decision-label consistency", "repeat evidence-citation consistency",
			"malformed rate", "input tokens", "output tokens", "request latency",
			"generation tokens per second", "peak adapter-process RSS when observed",
			"system memory pressure before and after each observation",
		},
		InferredMetrics: []string{
			"review-or-invalid burden", "unsafe accept rate", "paired arm deltas averaged by independent base",
		},
		AnalysisPlan: []string{
			"Compute each base's mean Arm C minus Arm A verified-correctness delta using replicate 1 only, then weight all 14 bases equally.",
			"Report all 14 paired base deltas, their mean and median, the exact paired sign-flip distribution over all 2^14 assignments, and leave-one-base-out sensitivity.",
			"Use repeats only for all-three label agreement, evidence-ID Jaccard, route/malformed agreement, and latency/token dispersion; retain failures.",
			"Do not use asymptotic cluster-robust inference as the primary analysis and do not tune thresholds on outcomes presented as confirmatory.",
		},
		PerformanceTracing: "The 332-observation quality schedule is untraced. PID-attributed Metal tracing is an optional separately identified secondary phase on a prospectively amended small subset; its latency is never pooled with untraced latency, and missing or ambiguous Metal data cannot censor a valid quality observation.",
		OutcomeRules: OutcomeRules{
			PositiveDecisionDeltaMinimum: 0.05, NegativeDecisionDeltaMaximum: -0.05,
			ReviewBurdenDeltaMinimum: -0.05, ReviewBurdenDeltaMaximum: 0.05,
			UnsafeAcceptDeltaMaximum: 0.05, MinimumValidPrimaryRate: 0.80,
			ClusterUnit:          "independent_base",
			ThresholdPolicy:      "No confidence threshold is selected because confidence is unavailable. Select on pooled Arm A+C Distill observations only: choose the highest equal-base-weighted-coverage rule meeting unsafe risk <=0.15 and at least 3 accepted observations; ties use the frozen strict-to-lenient order. Per-arm Distill rows are diagnostics only. Evaluate only that selected rule once on LLMTraceFX and report pooled plus per-arm diagnostic rows. If none qualifies, report no-safe-auto-action and do not evaluate a LLMTraceFX rule.",
			RoutingUnsafeCeiling: 0.15, RoutingMinimumAccepted: 3,
			RoutingRuleOrder: []string{"accept_with_all_evidence", "accept_with_any_evidence", "generated_accept"},
		},
		SeparationStatement: "This development/calibration local study is not the frozen TypeSafe final study, uses no held-out AgentTrace evidence, cannot update or fix the frozen TypeSafe protocol based on its outcomes, and authorizes no cloud provisioning or spend.",
	}
	protocol.ProtocolSHA256, _ = DigestDomain("protocol", protocolDigestProjection(protocol))
	if err := ValidateProtocol(protocol); err != nil {
		return Protocol{}, err
	}
	return protocol, nil
}

func protocolDigestProjection(protocol Protocol) Protocol {
	protocol.ProtocolSHA256 = ""
	return protocol
}

func ValidateProtocol(protocol Protocol) error {
	if protocol.SchemaVersion != SchemaVersion+"/protocol" ||
		protocol.Status != "prospective_frozen_no_observations" ||
		protocol.ProspectiveAmendment != "research/context-is-a-build-artifact/local-control-amendment-v1.md" ||
		protocol.ProspectiveAmendmentSHA256 != contextartifact.LocalControlAmendmentSHA256 ||
		DigestBytes(contextartifact.LocalControlAmendment) != contextartifact.LocalControlAmendmentSHA256 ||
		protocol.IndependentUnit != "base" || !protocol.ConditionRecordsClustered ||
		protocol.BaseCount != 14 || protocol.ConditionCount != 118 ||
		protocol.PrimaryObservations != PrimaryObservationCount ||
		protocol.RepeatConditionCount != RepeatConditionCount ||
		protocol.RepeatObservations != RepeatObservationCount ||
		protocol.ObservationsPerModel != ObservationsPerModel ||
		protocol.TargetModelID != TargetModelID || protocol.TargetModelRevision != TargetModelRevision ||
		protocol.TargetModelArtifactSHA256 != TargetModelArtifactSHA256 ||
		protocol.RuntimeStatus != "fresh_runtime_frozen_execution_requires_clean_host_preflight" ||
		protocol.RetiredRuntimeSHA256 != RuntimeIdentitySHA256 ||
		protocol.RuntimePackageClosureSHA256 != RuntimeClosureSHA256 ||
		protocol.RuntimeRequirementsInputSHA256 != "4f6fad6a9e96d8ee1efdf59f1bd423c4425647c6d72db83ab9d335f5fea7c21b" ||
		protocol.RuntimeRequirementsSHA256 != "779c01eeabc50358a94c00d204dbe02526c19165505fd0904c1ed37583a8fee1" ||
		protocol.RuntimeWheelVerificationSHA256 != RuntimeWheelVerificationSHA256 ||
		protocol.RuntimeVenvManifestSHA256 != RuntimeVenvManifestSHA256 ||
		protocol.BasePythonBinarySHA256 != BasePythonBinarySHA256 ||
		protocol.BasePythonManifestSHA256 != BasePythonManifestSHA256 ||
		len(protocol.Arms) != 2 || protocol.Arms[0] != ArmRaw || protocol.Arms[1] != ArmDistillLock ||
		protocol.Generation.AttemptsPerScheduled != 1 || protocol.Generation.MaxOutputTokens != 256 ||
		protocol.Generation.Sampling != "greedy_mlx_make_sampler_temp_0" ||
		protocol.Generation.Temperature == nil || *protocol.Generation.Temperature != 0 ||
		protocol.OutcomeRules.PositiveDecisionDeltaMinimum != 0.05 ||
		protocol.OutcomeRules.NegativeDecisionDeltaMaximum != -0.05 ||
		protocol.OutcomeRules.ReviewBurdenDeltaMinimum != -0.05 ||
		protocol.OutcomeRules.RoutingUnsafeCeiling != 0.15 ||
		protocol.OutcomeRules.ReviewBurdenDeltaMaximum != 0.05 ||
		protocol.OutcomeRules.UnsafeAcceptDeltaMaximum != 0.05 ||
		protocol.OutcomeRules.MinimumValidPrimaryRate != 0.80 ||
		protocol.OutcomeRules.RoutingMinimumAccepted != 3 ||
		fmt.Sprint(protocol.OutcomeRules.RoutingRuleOrder) != fmt.Sprint([]string{"accept_with_all_evidence", "accept_with_any_evidence", "generated_accept"}) ||
		len(protocol.RepeatConditionIDs) != RepeatConditionCount {
		return fmt.Errorf("local-control protocol constants drifted")
	}
	for i, id := range repeatConditionIDs {
		if protocol.RepeatConditionIDs[i] != id {
			return fmt.Errorf("repeat subset drifted")
		}
	}
	digest, _ := DigestDomain("protocol", protocolDigestProjection(protocol))
	if digest != protocol.ProtocolSHA256 {
		return fmt.Errorf("protocol digest mismatch")
	}
	return nil
}

func floatPointer(value float64) *float64 {
	return &value
}
