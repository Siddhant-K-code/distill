package studylocalv2

import "fmt"

func BuildProtocol() (Protocol, error) {
	protocol := Protocol{
		SchemaVersion: SchemaVersion + "/protocol", Status: "prospective_frozen_v2_no_observations",
		StudyKind: "development_calibration_local_open_model_v2", IndependentUnit: "base",
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
		RuntimeStatus:                  "blocked_recreate_clean_runtime_and_freeze_new_full_tree_digest",
		RetiredRuntimeSHA256:           RuntimeIdentitySHA256,
		RuntimePackageClosureSHA256:    RuntimeClosureSHA256,
		RuntimeRequirementsInputSHA256: "4f6fad6a9e96d8ee1efdf59f1bd423c4425647c6d72db83ab9d335f5fea7c21b",
		RuntimeRequirementsSHA256:      "779c01eeabc50358a94c00d204dbe02526c19165505fd0904c1ed37583a8fee1",
		RuntimeWheelVerificationSHA256: RuntimeWheelVerificationSHA256,
		RuntimeVenvManifestSHA256:      RuntimeVenvManifestSHA256,
		BasePythonBinarySHA256:         BasePythonBinarySHA256,
		BasePythonManifestSHA256:       BasePythonManifestSHA256,
		ScalingControl:                 "disabled_in_v2_pending_separate_pinned_model_hardware_clean_preflight_and_explicit_spending_authorization",
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
			ReviewBurdenDeltaMinimum: -0.05, MinimumValidPrimaryRate: 0.80,
			ClusterUnit:          "independent_base",
			ThresholdPolicy:      "No confidence threshold is selected because confidence is unavailable. On Distill only, choose the highest-base-weighted-coverage rule meeting unsafe risk <=0.15 and at least 3 accepted observations; ties use the frozen strict-to-lenient order. Evaluate only that selected rule once on LLMTraceFX. If none qualifies, report no-safe-auto-action and do not evaluate a LLMTraceFX rule.",
			RoutingUnsafeCeiling: 0.15, RoutingMinimumAccepted: 3,
			RoutingRuleOrder: []string{"accept_with_all_evidence", "accept_with_any_evidence", "generated_accept"},
		},
		SeparationStatement:   "This development/calibration local study is not the frozen TypeSafe final study, uses no held-out AgentTrace evidence, cannot update or fix the frozen TypeSafe protocol based on its outcomes, and authorizes no cloud provisioning or spend.",
		V1Outcome:             "invalid_pre_observation_adapter_startup_failure",
		V1Observations:        0,
		V1ModelLoads:          0,
		ProspectiveBasis:      "V2 is a new prospective protocol, not a retry of v1. It was frozen after an outcome-free v1 startup failure and before any local-model output existed, so no model outcome could select this design.",
		FramingContract:       "Distill-native disk JSON is RFC8785/JCS UTF-8 plus exactly one terminal LF; JSONL is one canonical object per LF-terminated line; transport frames are one canonical object plus one LF stripped only by the framing layer. Missing or extra LF, CRLF, whitespace, prose, concatenation, duplicate keys, nonfinite or out-of-range numbers, and wrong top-level types fail closed.",
		AuthorizationBoundary: "A fresh, unexpired, path-bound adapter-check receipt must validate framing, source/build/runtime/model/package identities, host preflight, and the absent output namespace with model_loaded=false before authorization. The sole attempt is consumed only after a second pre-load handshake and immediately before the load command.",
		QuiescenceCondition:   "Absolute performance observations are valid only under the frozen host quiescence, power, memory, swap, disk, process-RSS, sleep-inhibition, and deny-network conditions.",
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
		protocol.Status != "prospective_frozen_v2_no_observations" ||
		protocol.StudyKind != "development_calibration_local_open_model_v2" ||
		protocol.IndependentUnit != "base" || !protocol.ConditionRecordsClustered ||
		protocol.BaseCount != 14 || protocol.ConditionCount != 118 ||
		protocol.PrimaryObservations != PrimaryObservationCount ||
		protocol.RepeatConditionCount != RepeatConditionCount ||
		protocol.RepeatObservations != RepeatObservationCount ||
		protocol.ObservationsPerModel != ObservationsPerModel ||
		protocol.TargetModelID != TargetModelID || protocol.TargetModelRevision != TargetModelRevision ||
		protocol.TargetModelArtifactSHA256 != TargetModelArtifactSHA256 ||
		protocol.RuntimeStatus != "blocked_recreate_clean_runtime_and_freeze_new_full_tree_digest" ||
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
		protocol.OutcomeRules.RoutingUnsafeCeiling != 0.15 ||
		protocol.OutcomeRules.RoutingMinimumAccepted != 3 ||
		protocol.V1Outcome != "invalid_pre_observation_adapter_startup_failure" ||
		protocol.V1Observations != 0 || protocol.V1ModelLoads != 0 ||
		protocol.ProspectiveBasis == "" || protocol.FramingContract == "" ||
		protocol.AuthorizationBoundary == "" || protocol.QuiescenceCondition == "" ||
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
