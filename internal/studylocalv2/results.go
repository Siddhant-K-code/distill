package studylocalv2

import (
	"bytes"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"sort"
	"time"
)

type BaseDelta struct {
	BaseID                   string  `json:"base_id"`
	Dataset                  string  `json:"dataset"`
	ArmACorrectness          float64 `json:"arm_a_correctness"`
	ArmCCorrectness          float64 `json:"arm_c_correctness"`
	CorrectnessDeltaCMinusA  float64 `json:"correctness_delta_c_minus_a"`
	ArmAReviewBurden         float64 `json:"arm_a_review_burden"`
	ArmCReviewBurden         float64 `json:"arm_c_review_burden"`
	ReviewBurdenDeltaCMinusA float64 `json:"review_burden_delta_c_minus_a"`
	ArmAUnsafeAccept         float64 `json:"arm_a_unsafe_accept_rate"`
	ArmCUnsafeAccept         float64 `json:"arm_c_unsafe_accept_rate"`
	UnsafeAcceptDeltaCMinusA float64 `json:"unsafe_accept_delta_c_minus_a"`
}

type LeaveOneOut struct {
	OmittedBaseID         string  `json:"omitted_base_id"`
	MeanCorrectnessDelta  float64 `json:"mean_correctness_delta"`
	MeanReviewBurdenDelta float64 `json:"mean_review_burden_delta"`
	MeanUnsafeAcceptDelta float64 `json:"mean_unsafe_accept_delta"`
}

type RepeatMetrics struct {
	RepeatedConditionArmPairs  int     `json:"repeated_condition_arm_pairs"`
	AllThreeDecisionAgreement  float64 `json:"all_three_decision_agreement"`
	AllThreeRouteAgreement     float64 `json:"all_three_route_agreement"`
	AllThreeMalformedAgreement float64 `json:"all_three_malformed_agreement"`
	MeanEvidenceJaccard        float64 `json:"mean_evidence_id_jaccard"`
	MeanLatencyCV              float64 `json:"mean_latency_coefficient_of_variation"`
	MeanOutputTokenCV          float64 `json:"mean_output_token_coefficient_of_variation"`
}

type MeasuredMetrics struct {
	Observations                   int     `json:"observations"`
	Valid                          int     `json:"valid"`
	Malformed                      int     `json:"malformed"`
	AdapterErrors                  int     `json:"adapter_errors"`
	MalformedRate                  float64 `json:"malformed_rate"`
	InputTokens                    int64   `json:"input_tokens"`
	OutputTokens                   int64   `json:"output_tokens"`
	TotalRequestLatencyNanoseconds int64   `json:"total_request_latency_nanoseconds"`
	MaximumSampledRSSBytes         int64   `json:"maximum_sampled_process_rss_bytes"`
	MaximumMLXPeakBytes            int64   `json:"maximum_mlx_peak_bytes"`
	MinimumVMAvailablePct          float64 `json:"minimum_vm_available_percent"`
	MinimumPressureFreePct         float64 `json:"minimum_memory_pressure_free_percent"`
}

type InferredMetrics struct {
	EqualWeightMeanCorrectnessDelta  float64 `json:"equal_weight_mean_correctness_delta"`
	MedianCorrectnessDelta           float64 `json:"median_correctness_delta"`
	EqualWeightMeanReviewBurdenDelta float64 `json:"equal_weight_mean_review_burden_delta"`
	EqualWeightMeanUnsafeAcceptDelta float64 `json:"equal_weight_mean_unsafe_accept_delta"`
	ExactSignFlipAssignments         int     `json:"exact_sign_flip_assignments"`
	ExactTwoSidedSignFlipP           float64 `json:"exact_two_sided_sign_flip_p"`
	ValidPrimaryRate                 float64 `json:"valid_primary_rate"`
}

type IntegritySummary struct {
	CorpusSHA256             string `json:"corpus_sha256"`
	ProtocolSHA256           string `json:"protocol_sha256"`
	ContextsSHA256           string `json:"contexts_sha256"`
	ScheduleSHA256           string `json:"schedule_sha256"`
	AuthorizationSHA256      string `json:"authorization_sha256"`
	LedgerGenesisSHA256      string `json:"ledger_genesis_sha256"`
	AttemptLedgerEntrySHA256 string `json:"attempt_ledger_entry_sha256"`
	ModelManifestSHA256      string `json:"model_manifest_sha256"`
	RuntimeManifestSHA256    string `json:"runtime_manifest_sha256"`
	ReceiptCollectionSHA256  string `json:"receipt_collection_sha256"`
	ExpectedObservations     int    `json:"expected_observations"`
	VerifiedReceipts         int    `json:"verified_receipts"`
}

type FailureCount struct {
	Level  string `json:"level"`
	Status string `json:"status"`
	Count  int    `json:"count"`
}

type FamilyDelta struct {
	Family                   string  `json:"family"`
	Bases                    int     `json:"bases"`
	Conditions               int     `json:"conditions"`
	CorrectnessDeltaCMinusA  float64 `json:"correctness_delta_c_minus_a"`
	ReviewBurdenDeltaCMinusA float64 `json:"review_burden_delta_c_minus_a"`
}

type RiskOperatingPoint struct {
	Dataset              string   `json:"dataset"`
	Rule                 string   `json:"rule"`
	Bases                int      `json:"bases"`
	Accepted             int      `json:"accepted"`
	Total                int      `json:"total"`
	Unsafe               int      `json:"unsafe"`
	Coverage             float64  `json:"coverage"`
	UnsafeRisk           *float64 `json:"unsafe_risk"`
	BaseWeightedCoverage float64  `json:"base_weighted_coverage"`
}

type CaseExplorerRow struct {
	ConditionID   string   `json:"condition_id"`
	BaseID        string   `json:"base_id"`
	Dataset       string   `json:"dataset"`
	Family        string   `json:"family"`
	Truth         string   `json:"truth"`
	ArmAStatus    string   `json:"arm_a_status"`
	ArmADecision  string   `json:"arm_a_decision,omitempty"`
	ArmACitations []string `json:"arm_a_citations"`
	ArmCStatus    string   `json:"arm_c_status"`
	ArmCDecision  string   `json:"arm_c_decision,omitempty"`
	ArmCCitations []string `json:"arm_c_citations"`
}

type ArmEfficiency struct {
	Arm                       string   `json:"arm"`
	Observations              int      `json:"observations"`
	InputTokens               int64    `json:"input_tokens"`
	OutputTokens              int64    `json:"output_tokens"`
	MeanRequestLatencyNanos   float64  `json:"mean_request_latency_nanoseconds"`
	MeanOfPostFirstTokenRates *float64 `json:"mean_of_post_first_token_rates"`
	MaximumSampledRSSBytes    int64    `json:"maximum_sampled_process_rss_bytes"`
	MaximumMLXPeakBytes       int64    `json:"maximum_mlx_allocator_peak_bytes"`
	Provenance                string   `json:"provenance"`
}

type ResultSummary struct {
	SchemaVersion                     string               `json:"schema_version"`
	Outcome                           string               `json:"outcome"`
	OutcomeReason                     string               `json:"outcome_reason"`
	ClusterUnit                       string               `json:"cluster_unit"`
	BaseWeighting                     string               `json:"base_weighting"`
	BaseDeltas                        []BaseDelta          `json:"base_deltas"`
	LeaveOneBaseOut                   []LeaveOneOut        `json:"leave_one_base_out"`
	Repeatability                     RepeatMetrics        `json:"repeatability"`
	Integrity                         IntegritySummary     `json:"integrity"`
	FailureWaterfall                  []FailureCount       `json:"failure_waterfall"`
	FamilyDeltas                      []FamilyDelta        `json:"family_deltas"`
	RiskOperatingPoints               []RiskOperatingPoint `json:"risk_operating_points"`
	SelectedDistillRule               string               `json:"selected_distill_rule"`
	CaseExplorer                      []CaseExplorerRow    `json:"case_explorer"`
	EfficiencyByArm                   []ArmEfficiency      `json:"efficiency_by_arm"`
	Measured                          MeasuredMetrics      `json:"measured"`
	Inferred                          InferredMetrics      `json:"inferred"`
	ConfidenceThresholds              string               `json:"confidence_thresholds"`
	RiskCoverage                      string               `json:"risk_coverage"`
	MetalMetrics                      string               `json:"metal_metrics"`
	ProviderCalls                     int                  `json:"provider_calls"`
	CloudSpend                        string               `json:"cloud_spend"`
	HeldOutRecords                    int                  `json:"held_out_records"`
	ClaimLimit                        string               `json:"claim_limit"`
	V1Outcome                         string               `json:"v1_outcome"`
	V1PreAuthorizationWrapperFailures int                  `json:"v1_pre_authorization_wrapper_failures"`
	V1ConsumedStartupAborts           int                  `json:"v1_consumed_startup_aborts"`
	V1Observations                    int                  `json:"v1_observations"`
	V1ModelLoads                      int                  `json:"v1_model_loads"`
	V2ProspectiveBeforeModelOutput    bool                 `json:"v2_prospective_before_model_output"`
	OutcomeSelectionPossible          bool                 `json:"outcome_selection_possible"`
	QuiescenceCondition               string               `json:"quiescence_condition"`
}

type validatedRun struct {
	Package       Package
	Run           RunManifest
	Receipts      []ExecutionReceipt
	Authorization ExecutionAuthorization
	Attempt       AttemptLedgerEntry
	Completion    CompletionRecord
}

func VerifyResults(packageDirectory, authorizationDirectory, runDirectory string) error {
	_, err := validateResults(packageDirectory, authorizationDirectory, runDirectory)
	return err
}

func SummarizeResults(packageDirectory, authorizationDirectory, runDirectory string) (ResultSummary, error) {
	validated, err := validateResults(packageDirectory, authorizationDirectory, runDirectory)
	if err != nil {
		return ResultSummary{}, err
	}
	return analyzeResults(validated)
}

func normalizeExistingPath(path string) (string, error) {
	if path == "" {
		return "", fmt.Errorf("path is required")
	}
	absolute, err := filepath.Abs(filepath.Clean(path))
	if err != nil {
		return "", err
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(absolute))
	if err != nil {
		return "", err
	}
	resolved := filepath.Join(parent, filepath.Base(absolute))
	info, err := os.Lstat(resolved)
	if err != nil || info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("path must exist without a final symlink: %s", resolved)
	}
	return resolved, nil
}

func validateResults(packageDirectory, authorizationDirectory, runDirectory string) (validatedRun, error) {
	normalizedAuthorization, err := normalizeExistingPath(authorizationDirectory)
	if err != nil {
		return validatedRun{}, err
	}
	normalizedRun, err := normalizeExistingPath(runDirectory)
	if err != nil {
		return validatedRun{}, err
	}
	authorizationDirectory, runDirectory = normalizedAuthorization, normalizedRun
	pkg, err := ValidatePackage(packageDirectory)
	if err != nil {
		return validatedRun{}, err
	}
	info, err := os.Lstat(runDirectory)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return validatedRun{}, fmt.Errorf("run directory must be real and owner-only")
	}
	if err := requireCurrentOwner(info); err != nil {
		return validatedRun{}, err
	}
	authorization, _, _, _, authorizationFiles, err := LoadAuthorization(authorizationDirectory)
	if err != nil {
		return validatedRun{}, err
	}
	if authorization.OutputNamespaceSHA256 != DigestBytes([]byte(runDirectory)) ||
		authorization.AuthorizationDirectorySHA256 != DigestBytes([]byte(authorizationDirectory)) {
		return validatedRun{}, fmt.Errorf("result paths do not match the one-time authorization")
	}
	_, attempt, err := validateAttemptLedgerBytes(authorizationFiles["attempt-ledger.jsonl"], authorization, true)
	if err != nil || attempt == nil {
		return validatedRun{}, fmt.Errorf("result lacks a valid single-use authorization attempt")
	}
	required := []string{
		"adapter-check.json", "adapter-workspace", "authorization.json", "calls", "completion.json",
		"host-preflight.json", "model-manifest.json", "package-manifest.json",
		"run-manifest.json", "runtime-manifest.json",
	}
	entries, err := os.ReadDir(runDirectory)
	if err != nil {
		return validatedRun{}, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	sort.Strings(required)
	if fmt.Sprint(names) != fmt.Sprint(required) {
		return validatedRun{}, fmt.Errorf("run directory file set mismatch")
	}
	adapterWorkspace, err := os.ReadDir(filepath.Join(runDirectory, "adapter-workspace"))
	if err != nil || len(adapterWorkspace) != 0 {
		return validatedRun{}, fmt.Errorf("adapter workspace is not an empty private namespace")
	}
	readStrictFile := func(name string, target any) ([]byte, error) {
		path := filepath.Join(runDirectory, name)
		info, err := os.Lstat(path)
		if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
			info.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("%s must be an owner-only regular file", name)
		}
		if err := requireCurrentOwner(info); err != nil {
			return nil, err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, err
		}
		if err := strictJSONObjectFileDecode(data, target); err != nil {
			return nil, err
		}
		return data, nil
	}
	var run RunManifest
	if _, err := readStrictFile("run-manifest.json", &run); err != nil {
		return validatedRun{}, err
	}
	var copiedAuthorization ExecutionAuthorization
	if _, err := readStrictFile("authorization.json", &copiedAuthorization); err != nil {
		return validatedRun{}, err
	}
	authorizationBytes, _ := canonicalJSON(copiedAuthorization)
	expectedAuthorizationBytes, _ := canonicalJSON(authorization)
	if !bytes.Equal(authorizationBytes, expectedAuthorizationBytes) ||
		authorization.PackageManifestSHA256 != DigestBytes(pkg.Files["package-manifest.json"]) ||
		authorization.ScheduleSHA256 != pkg.Manifest.ScheduleSHA256 {
		return validatedRun{}, fmt.Errorf("run authorization mismatch")
	}
	var copiedCheck AdapterCheckReceipt
	checkBytes, err := readStrictFile("adapter-check.json", &copiedCheck)
	if err != nil || copiedCheck.ReceiptSHA256 != authorization.AdapterCheckReceiptSHA256 ||
		DigestBytes(checkBytes) != DigestBytes(authorizationFiles["adapter-check.json"]) {
		return validatedRun{}, fmt.Errorf("run adapter-check receipt mismatch")
	}
	var copiedPackage PackageManifest
	packageBytes, err := readStrictFile("package-manifest.json", &copiedPackage)
	if err != nil || !bytes.Equal(packageBytes, pkg.Files["package-manifest.json"]) {
		return validatedRun{}, fmt.Errorf("run package manifest mismatch")
	}
	var host HostSnapshot
	if _, err := readStrictFile("host-preflight.json", &host); err != nil || !host.Eligible {
		return validatedRun{}, fmt.Errorf("run host preflight mismatch")
	}
	var model ModelManifest
	if _, err := readStrictFile("model-manifest.json", &model); err != nil {
		return validatedRun{}, err
	}
	if err := validateModelManifestRecord(model); err != nil ||
		model.ManifestSHA256 != authorization.ModelManifestSHA256 {
		return validatedRun{}, fmt.Errorf("run model manifest mismatch")
	}
	var runtimeManifest RuntimeManifest
	if _, err := readStrictFile("runtime-manifest.json", &runtimeManifest); err != nil {
		return validatedRun{}, err
	}
	if _, _, err := validateRuntimeManifestValue(runtimeManifest); err != nil ||
		runtimeManifest.ManifestSHA256 != authorization.RuntimeManifestSHA256 ||
		runtimeManifest.FullTreeSHA256 != authorization.RuntimeFullTreeSHA256 {
		return validatedRun{}, fmt.Errorf("run runtime manifest mismatch")
	}
	if run.SchemaVersion != SchemaVersion+"/run-manifest" ||
		run.PackageManifestSHA256 != DigestBytes(pkg.Files["package-manifest.json"]) ||
		run.ScheduleSHA256 != pkg.Manifest.ScheduleSHA256 || run.ObservationsExpected != len(pkg.Schedule) ||
		run.AttemptsPerObservation != 1 || run.ModelID != TargetModelID ||
		run.ModelRevision != TargetModelRevision || run.ScalingControlEnabled || run.NetworkAllowed ||
		run.AuthorizationSHA256 != authorization.AuthorizationSHA256 ||
		run.ImplementationCommit != authorization.ImplementationCommit ||
		run.AttemptNonce != authorization.AttemptNonce ||
		run.AttemptLedgerEntrySHA256 != attempt.EntrySHA256 {
		return validatedRun{}, fmt.Errorf("run manifest mismatch")
	}
	var completion CompletionRecord
	if _, err := readStrictFile("completion.json", &completion); err != nil {
		return validatedRun{}, err
	}
	if completion.SchemaVersion != SchemaVersion+"/completion" ||
		completion.ObservationsExpected != len(pkg.Schedule) ||
		completion.ObservationsRecorded != len(pkg.Schedule) ||
		!completion.RuntimeReverified || !completion.ModelReverified ||
		completion.CompletedAt.IsZero() ||
		completion.Valid+completion.Malformed+completion.AdapterErrors != completion.ObservationsRecorded ||
		completion.AdapterErrors != 0 {
		return validatedRun{}, fmt.Errorf("completion record mismatch")
	}
	callsDirectory := filepath.Join(runDirectory, "calls")
	callEntries, err := os.ReadDir(callsDirectory)
	if err != nil || len(callEntries) != len(pkg.Schedule) {
		return validatedRun{}, fmt.Errorf("call directory count mismatch")
	}
	receipts := make([]ExecutionReceipt, 0, len(pkg.Schedule))
	digests := make([]string, 0, len(pkg.Schedule))
	for i, entry := range pkg.Schedule {
		expectedName := fmt.Sprintf("%03d-%s", entry.Index, entry.ObservationID)
		if callEntries[i].Name() != expectedName || !callEntries[i].IsDir() {
			return validatedRun{}, fmt.Errorf("call directory order mismatch at %d", entry.Index)
		}
		receipt, err := validateObservation(filepath.Join(callsDirectory, expectedName), entry, pkg)
		if err != nil {
			return validatedRun{}, err
		}
		receipts = append(receipts, receipt)
		if receipt.AuthorizationSHA256 != authorization.AuthorizationSHA256 ||
			receipt.AttemptNonce != authorization.AttemptNonce ||
			receipt.AttemptLedgerEntrySHA256 != attempt.EntrySHA256 ||
			receipt.ModelManifestSHA256 != model.ManifestSHA256 ||
			receipt.RuntimeManifestSHA256 != runtimeManifest.ManifestSHA256 {
			return validatedRun{}, fmt.Errorf("receipt binding mismatch at %s", entry.ObservationID)
		}
		digests = append(digests, receipt.ReceiptSHA256)
	}
	valid, malformed, adapterErrors := 0, 0, 0
	for _, receipt := range receipts {
		switch receipt.Status {
		case "valid":
			valid++
		case "malformed":
			malformed++
		case "adapter_error":
			adapterErrors++
		}
	}
	if valid != completion.Valid || malformed != completion.Malformed || adapterErrors != completion.AdapterErrors {
		return validatedRun{}, fmt.Errorf("completion counters disagree with receipts")
	}
	collection, _ := DigestDomain("receipt-collection", digests)
	if collection != completion.ReceiptCollectionSHA256 {
		return validatedRun{}, fmt.Errorf("receipt collection digest mismatch")
	}
	return validatedRun{
		Package: pkg, Run: run, Receipts: receipts, Authorization: authorization,
		Attempt: *attempt, Completion: completion,
	}, nil
}

func validateObservation(directory string, entry ScheduleEntry, pkg Package) (ExecutionReceipt, error) {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	files := map[string][]byte{}
	for _, item := range entries {
		info, err := item.Info()
		if err != nil || item.Type()&os.ModeSymlink != 0 || !item.Type().IsRegular() ||
			info.Mode().Perm()&0o077 != 0 {
			return ExecutionReceipt{}, fmt.Errorf("unsafe observation artifact")
		}
		files[item.Name()], err = os.ReadFile(filepath.Join(directory, item.Name()))
		if err != nil {
			return ExecutionReceipt{}, err
		}
	}
	for _, required := range []string{"request.json", "adapter-stdout.bin", "adapter-stderr.bin", "receipt.json"} {
		if _, ok := files[required]; !ok {
			return ExecutionReceipt{}, fmt.Errorf("observation %s lacks %s", entry.ObservationID, required)
		}
	}
	var receipt ExecutionReceipt
	if err := strictJSONObjectFileDecode(files["receipt.json"], &receipt); err != nil {
		return ExecutionReceipt{}, err
	}
	if receipt.SchemaVersion != SchemaVersion+"/execution-receipt" ||
		receipt.ObservationID != entry.ObservationID || receipt.ScheduleIndex != entry.Index ||
		receipt.ConditionID != entry.ConditionID || receipt.BaseID != entry.BaseID ||
		receipt.Dataset != entry.Dataset || receipt.Arm != entry.Arm || receipt.Replicate != entry.Replicate ||
		(receipt.Status != "valid" && receipt.Status != "malformed" && receipt.Status != "adapter_error") ||
		receipt.RequestSHA256 != DigestBytes(files["request.json"]) ||
		receipt.AdapterStderrSHA256 != DigestBytes(files["adapter-stderr.bin"]) {
		return ExecutionReceipt{}, fmt.Errorf("observation receipt mismatch for %s", entry.ObservationID)
	}
	maximumRequestLatency := int64(time.Duration(pkg.Protocol.Isolation.ObservationTimeoutSeconds) * time.Second)
	if receipt.StartedAt.IsZero() || receipt.FinishedAt.Before(receipt.StartedAt) ||
		receipt.RequestLatencyNanos < 0 || receipt.RequestLatencyNanos > maximumRequestLatency {
		return ExecutionReceipt{}, fmt.Errorf("observation timing mismatch for %s", entry.ObservationID)
	}
	if receipt.Status != "adapter_error" && receipt.AdapterEnvelopeSHA256 != DigestBytes(files["adapter-stdout.bin"]) {
		return ExecutionReceipt{}, fmt.Errorf("adapter envelope digest mismatch for %s", entry.ObservationID)
	}
	receiptDigest, _ := DigestDomain("execution-receipt", receiptProjection(receipt))
	if receiptDigest != receipt.ReceiptSHA256 {
		return ExecutionReceipt{}, fmt.Errorf("receipt digest mismatch for %s", entry.ObservationID)
	}
	conditions := map[string]Condition{}
	contexts := map[string]Context{}
	for _, condition := range pkg.Corpus.Conditions {
		conditions[condition.ID] = condition
	}
	for _, context := range pkg.Contexts {
		contexts[context.ConditionID+"\x00"+context.Arm] = context
	}
	request, err := BuildAdapterRequest(entry, conditions[entry.ConditionID], contexts[entry.ConditionID+"\x00"+entry.Arm], pkg.Protocol.Generation)
	if err != nil {
		return ExecutionReceipt{}, err
	}
	expectedRequest, _ := canonicalJSONFile(request)
	if !bytes.Equal(expectedRequest, files["request.json"]) {
		return ExecutionReceipt{}, fmt.Errorf("request bytes mismatch for %s", entry.ObservationID)
	}
	var adapterResponse AdapterResponse
	if receipt.Status != "adapter_error" {
		if err := strictJSONPayloadDecode(files["adapter-stdout.bin"], &adapterResponse, true); err != nil {
			return ExecutionReceipt{}, fmt.Errorf("adapter stdout envelope for %s: %w", entry.ObservationID, err)
		}
		if err := validateAdapterResponse(request, adapterResponse); err != nil {
			return ExecutionReceipt{}, fmt.Errorf("adapter stdout contract for %s: %w", entry.ObservationID, err)
		}
		if len(receipt.Questions) != len(request.Questions) {
			return ExecutionReceipt{}, fmt.Errorf("question count mismatch")
		}
		anyMalformed := false
		for i, question := range request.Questions {
			result := receipt.Questions[i]
			envelopeResult := adapterResponse.Questions[i]
			rawName := "raw-" + question.ID + ".bin"
			raw, ok := files[rawName]
			if !ok || result.QuestionID != question.ID || result.RawResponseSHA256 != DigestBytes(raw) ||
				result.RawResponseBytes != len(raw) || result.PromptTokens != len(result.PromptTokenIDs) ||
				result.GeneratedTokens != len(result.GeneratedTokenIDs) ||
				result.GeneratedTokens > pkg.Protocol.Generation.MaxOutputTokens ||
				result.Timing.TokenizeNanoseconds < 0 || result.Timing.TTFTNanoseconds < 0 ||
				result.Timing.DecodeNanoseconds < 0 || result.Timing.TotalNanoseconds < 0 ||
				result.Memory.ActiveBytes < 0 || result.Memory.CacheBytes < 0 ||
				result.Memory.PeakBytes < 0 ||
				result.Memory.PeakBytes > pkg.Protocol.Isolation.DuringMaximumMLXPeakBytes {
				return ExecutionReceipt{}, fmt.Errorf("raw response mismatch for %s/%s", entry.ObservationID, question.ID)
			}
			expectedProjection := ParseCategorical(raw, question.Allowed)
			expectedBytes, _ := canonicalJSON(expectedProjection)
			actualBytes, _ := canonicalJSON(result.Projection)
			if !bytes.Equal(expectedBytes, actualBytes) {
				return ExecutionReceipt{}, fmt.Errorf("projection mismatch for %s/%s", entry.ObservationID, question.ID)
			}
			envelopeReceipt := QuestionReceipt{
				QuestionID: envelopeResult.QuestionID, RawResponseSHA256: envelopeResult.RawResponseSHA256,
				RawResponseBytes: len(raw), PromptTokenIDs: envelopeResult.PromptTokenIDs,
				GeneratedTokenIDs: envelopeResult.GeneratedTokenIDs,
				PromptTokens:      envelopeResult.PromptTokens, GeneratedTokens: envelopeResult.GeneratedTokens,
				Projection: envelopeResult.Projection, Timing: envelopeResult.Timing, Memory: envelopeResult.Memory,
			}
			if envelopeResult.Timing.DecodeNanoseconds > 0 && envelopeResult.GeneratedTokens > 1 {
				rate := float64(envelopeResult.GeneratedTokens-1) / (float64(envelopeResult.Timing.DecodeNanoseconds) / 1e9)
				envelopeReceipt.TokensPerSecond = &rate
			}
			envelopeReceiptBytes, _ := canonicalJSON(envelopeReceipt)
			actualReceiptBytes, _ := canonicalJSON(result)
			if !bytes.Equal(envelopeReceiptBytes, actualReceiptBytes) {
				return ExecutionReceipt{}, fmt.Errorf("adapter stdout and receipt disagree for %s/%s", entry.ObservationID, question.ID)
			}
			if result.Projection.ParseStatus != ParseValid {
				anyMalformed = true
			}
			if result.TokensPerSecond != nil {
				if result.Timing.DecodeNanoseconds <= 0 || result.GeneratedTokens <= 1 {
					return ExecutionReceipt{}, fmt.Errorf("throughput denominator is not positive")
				}
				expectedRate := float64(result.GeneratedTokens-1) / (float64(result.Timing.DecodeNanoseconds) / 1e9)
				if math.Abs(*result.TokensPerSecond-expectedRate) > 1e-12 {
					return ExecutionReceipt{}, fmt.Errorf("throughput formula mismatch")
				}
			}
		}
		expectedStatus := "valid"
		if anyMalformed {
			expectedStatus = "malformed"
		}
		if receipt.Status != expectedStatus {
			return ExecutionReceipt{}, fmt.Errorf("receipt status disagrees with parse outcomes")
		}
	}
	if receipt.ResourceSummary.SampleCount != len(receipt.ResourceSummary.Samples) ||
		receipt.ResourceSummary.SampleCount < 2 ||
		receipt.ResourceSummary.RSSMeasurementSemantics != "sampled process RSS every two seconds; not a true high-water mark" {
		return ExecutionReceipt{}, fmt.Errorf("resource samples are incomplete for %s", entry.ObservationID)
	}
	if err := validateResourceSummary(receipt.ResourceSummary, pkg.Protocol.Isolation); err != nil {
		return ExecutionReceipt{}, fmt.Errorf("%s: %w", entry.ObservationID, err)
	}
	expectedFiles := map[string]bool{
		"request.json": true, "adapter-stdout.bin": true, "adapter-stderr.bin": true, "receipt.json": true,
	}
	if receipt.Status != "adapter_error" {
		for _, question := range request.Questions {
			expectedFiles["raw-"+question.ID+".bin"] = true
		}
	}
	if len(files) != len(expectedFiles) {
		return ExecutionReceipt{}, fmt.Errorf("unexpected observation files for %s", entry.ObservationID)
	}
	for name := range files {
		if !expectedFiles[name] {
			return ExecutionReceipt{}, fmt.Errorf("unexpected observation file %s", name)
		}
	}
	return receipt, nil
}

func validateResourceSummary(summary ObservationResourceSummary, limits IsolationLimits) error {
	minVM, minPressure, minDisk := 101.0, 101.0, int64(math.MaxInt64)
	var maxSwap, maxRSS int64
	for _, sample := range summary.Samples {
		if sample.CollectedAt.IsZero() || sample.VMAvailablePercent < 0 || sample.VMAvailablePercent > 100 ||
			sample.MemoryPressureFreePercent < 0 || sample.MemoryPressureFreePercent > 100 ||
			sample.SwapUsedBytes < 0 || sample.DiskFreeBytes < 0 || sample.ProcessRSSBytes < 0 {
			return fmt.Errorf("invalid resource sample")
		}
		minVM = math.Min(minVM, sample.VMAvailablePercent)
		minPressure = math.Min(minPressure, sample.MemoryPressureFreePercent)
		if sample.DiskFreeBytes < minDisk {
			minDisk = sample.DiskFreeBytes
		}
		if sample.SwapUsedBytes > maxSwap {
			maxSwap = sample.SwapUsedBytes
		}
		if sample.ProcessRSSBytes > maxRSS {
			maxRSS = sample.ProcessRSSBytes
		}
	}
	if summary.MinimumVMAvailablePct != minVM || summary.MinimumPressureFreePct != minPressure ||
		summary.MinimumDiskFreeBytes != minDisk || summary.MaximumSwapUsedBytes != maxSwap ||
		summary.MaximumSampledRSSBytes != maxRSS ||
		minVM < float64(limits.DuringMinimumFreeMemoryPercent) ||
		minPressure < float64(limits.DuringMinimumFreeMemoryPercent) ||
		minDisk < limits.DuringMinimumDiskFreeBytes ||
		maxSwap > limits.DuringMaximumSwapUsedBytes ||
		maxRSS > limits.DuringMaximumProcessRSSBytes ||
		summary.MaximumSwapGrowthBytes > limits.DuringMaximumSwapGrowthBytes {
		return fmt.Errorf("resource summary disagrees with samples or exceeds a bound")
	}
	return nil
}

type observationScore struct {
	receipt   ExecutionReceipt
	decision  string
	citations []string
	correct   bool
	review    bool
	unsafe    bool
	malformed bool
	input     int64
	output    int64
}

func analyzeResults(validated validatedRun) (ResultSummary, error) {
	conditions := map[string]Condition{}
	for _, condition := range validated.Package.Corpus.Conditions {
		conditions[condition.ID] = condition
	}
	scores := make([]observationScore, 0, len(validated.Receipts))
	summary := ResultSummary{
		SchemaVersion: SchemaVersion + "/result-summary", ClusterUnit: "independent_base",
		BaseWeighting:        "equal weight per base after within-base Arm C minus Arm A averaging",
		ConfidenceThresholds: "not applicable: confidence is unavailable and no threshold was selected",
		RiskCoverage:         "structural operating points only: candidate rules are evaluated on Distill, then exactly one frozen selected rule is evaluated on LLMTraceFX; these are not calibrated-confidence curves or AURC",
		MetalMetrics:         "not collected in the untraced 332-observation quality phase",
		ProviderCalls:        0, CloudSpend: "0", HeldOutRecords: 0,
		ClaimLimit:                        "This run can claim only Arm A versus Arm C effects for the pinned local Qwen3-4B artifact. A larger-model or better-context-beats-scaling claim is prohibited until a separately amended scaling control runs.",
		V1Outcome:                         "invalid_pre_observation_adapter_startup_failure",
		V1PreAuthorizationWrapperFailures: 4, V1ConsumedStartupAborts: 1,
		V1Observations: 0, V1ModelLoads: 0, V2ProspectiveBeforeModelOutput: true,
		OutcomeSelectionPossible: false,
		QuiescenceCondition:      "Absolute performance is conditioned on the frozen eligible and quiescent host state; it is not an unconditional hardware benchmark.",
	}
	summary.Integrity = IntegritySummary{
		CorpusSHA256:             validated.Package.Corpus.CorpusSHA256,
		ProtocolSHA256:           validated.Package.Protocol.ProtocolSHA256,
		ContextsSHA256:           validated.Package.Manifest.ContextsSHA256,
		ScheduleSHA256:           validated.Package.Manifest.ScheduleSHA256,
		AuthorizationSHA256:      validated.Authorization.AuthorizationSHA256,
		LedgerGenesisSHA256:      validated.Authorization.LedgerGenesisSHA256,
		AttemptLedgerEntrySHA256: validated.Attempt.EntrySHA256,
		ModelManifestSHA256:      validated.Authorization.ModelManifestSHA256,
		RuntimeManifestSHA256:    validated.Authorization.RuntimeManifestSHA256,
		ReceiptCollectionSHA256:  validated.Completion.ReceiptCollectionSHA256,
		ExpectedObservations:     ObservationsPerModel, VerifiedReceipts: len(validated.Receipts),
	}
	observationFailures := map[string]int{}
	parseFailures := map[string]int{}
	summary.Measured.MinimumVMAvailablePct = 101
	summary.Measured.MinimumPressureFreePct = 101
	for _, receipt := range validated.Receipts {
		condition := conditions[receipt.ConditionID]
		score := scoreObservation(receipt, condition)
		scores = append(scores, score)
		summary.Measured.Observations++
		switch receipt.Status {
		case "valid":
			summary.Measured.Valid++
		case "malformed":
			summary.Measured.Malformed++
		case "adapter_error":
			summary.Measured.AdapterErrors++
		}
		observationFailures[receipt.Status]++
		for _, question := range receipt.Questions {
			if question.Projection.ParseStatus != ParseValid {
				parseFailures[question.Projection.ParseStatus]++
			}
		}
		summary.Measured.InputTokens += score.input
		summary.Measured.OutputTokens += score.output
		summary.Measured.TotalRequestLatencyNanoseconds += receipt.RequestLatencyNanos
		if receipt.ResourceSummary.MaximumSampledRSSBytes > summary.Measured.MaximumSampledRSSBytes {
			summary.Measured.MaximumSampledRSSBytes = receipt.ResourceSummary.MaximumSampledRSSBytes
		}
		if receipt.ResourceSummary.MinimumVMAvailablePct < summary.Measured.MinimumVMAvailablePct {
			summary.Measured.MinimumVMAvailablePct = receipt.ResourceSummary.MinimumVMAvailablePct
		}
		if receipt.ResourceSummary.MinimumPressureFreePct < summary.Measured.MinimumPressureFreePct {
			summary.Measured.MinimumPressureFreePct = receipt.ResourceSummary.MinimumPressureFreePct
		}
		for _, question := range receipt.Questions {
			if question.Memory.PeakBytes > summary.Measured.MaximumMLXPeakBytes {
				summary.Measured.MaximumMLXPeakBytes = question.Memory.PeakBytes
			}
		}
	}
	if summary.Measured.Observations > 0 {
		summary.Measured.MalformedRate = float64(summary.Measured.Malformed) / float64(summary.Measured.Observations)
	}
	primary := make([]observationScore, 0, PrimaryObservationCount)
	for _, score := range scores {
		if score.receipt.Replicate == 1 {
			primary = append(primary, score)
		}
	}
	validPrimary := 0
	for _, score := range primary {
		if score.receipt.Status == "valid" {
			validPrimary++
		}
	}
	summary.Inferred.ValidPrimaryRate = float64(validPrimary) / float64(len(primary))
	summary.BaseDeltas = computeBaseDeltas(primary, validated.Package.Corpus.Bases)
	summary.FamilyDeltas = computeFamilyDeltas(primary, conditions)
	summary.RiskOperatingPoints, summary.SelectedDistillRule = computeRiskOperatingPoints(
		primary, validated.Package.Corpus.Bases, conditions, validated.Package.Protocol.OutcomeRules,
	)
	summary.CaseExplorer = buildCaseExplorer(primary, conditions)
	summary.EfficiencyByArm = computeEfficiency(scores)
	for status, count := range observationFailures {
		summary.FailureWaterfall = append(summary.FailureWaterfall, FailureCount{Level: "observation", Status: status, Count: count})
	}
	for status, count := range parseFailures {
		summary.FailureWaterfall = append(summary.FailureWaterfall, FailureCount{Level: "parse", Status: status, Count: count})
	}
	sort.Slice(summary.FailureWaterfall, func(i, j int) bool {
		if summary.FailureWaterfall[i].Level == summary.FailureWaterfall[j].Level {
			return summary.FailureWaterfall[i].Status < summary.FailureWaterfall[j].Status
		}
		return summary.FailureWaterfall[i].Level < summary.FailureWaterfall[j].Level
	})
	correctness := make([]float64, len(summary.BaseDeltas))
	review := make([]float64, len(summary.BaseDeltas))
	unsafe := make([]float64, len(summary.BaseDeltas))
	for i, delta := range summary.BaseDeltas {
		correctness[i] = delta.CorrectnessDeltaCMinusA
		review[i] = delta.ReviewBurdenDeltaCMinusA
		unsafe[i] = delta.UnsafeAcceptDeltaCMinusA
	}
	summary.Inferred.EqualWeightMeanCorrectnessDelta = mean(correctness)
	summary.Inferred.MedianCorrectnessDelta = median(correctness)
	summary.Inferred.EqualWeightMeanReviewBurdenDelta = mean(review)
	summary.Inferred.EqualWeightMeanUnsafeAcceptDelta = mean(unsafe)
	summary.Inferred.ExactSignFlipAssignments = 1 << len(correctness)
	summary.Inferred.ExactTwoSidedSignFlipP = exactSignFlipP(correctness)
	summary.LeaveOneBaseOut = leaveOneOut(summary.BaseDeltas)
	summary.Repeatability = repeatMetrics(scores)
	switch {
	case summary.Inferred.ValidPrimaryRate < validated.Package.Protocol.OutcomeRules.MinimumValidPrimaryRate:
		summary.Outcome = "non-evaluable"
		summary.OutcomeReason = "valid primary observation rate is below the prospectively frozen 0.80 floor"
	case summary.Inferred.EqualWeightMeanCorrectnessDelta <= validated.Package.Protocol.OutcomeRules.NegativeDecisionDeltaMaximum ||
		summary.Inferred.EqualWeightMeanUnsafeAcceptDelta >= 0.05:
		summary.Outcome = "negative"
		summary.OutcomeReason = "compiled context reduced equal-weight verified correctness by at least 0.05 or increased unsafe accept by at least 0.05"
	case (summary.Inferred.EqualWeightMeanCorrectnessDelta >= validated.Package.Protocol.OutcomeRules.PositiveDecisionDeltaMinimum &&
		summary.Inferred.EqualWeightMeanReviewBurdenDelta <= 0.05 &&
		summary.Inferred.EqualWeightMeanUnsafeAcceptDelta <= 0) ||
		(summary.Inferred.EqualWeightMeanReviewBurdenDelta <= validated.Package.Protocol.OutcomeRules.ReviewBurdenDeltaMinimum &&
			summary.Inferred.EqualWeightMeanCorrectnessDelta > validated.Package.Protocol.OutcomeRules.NegativeDecisionDeltaMaximum &&
			summary.Inferred.EqualWeightMeanUnsafeAcceptDelta <= 0):
		summary.Outcome = "positive"
		summary.OutcomeReason = "compiled context improved equal-weight verified correctness or reduced review burden without a prospectively material correctness loss or unsafe-accept increase"
	default:
		summary.Outcome = "null"
		summary.OutcomeReason = "prospectively frozen positive and negative effect thresholds were not crossed"
	}
	return summary, nil
}

func scoreObservation(receipt ExecutionReceipt, condition Condition) observationScore {
	score := observationScore{receipt: receipt, malformed: receipt.Status != "valid"}
	if receipt.Status != "valid" {
		score.review = true
		return score
	}
	for _, question := range receipt.Questions {
		score.input += int64(question.PromptTokens)
		score.output += int64(question.GeneratedTokens)
		if question.QuestionID == "decision_with_evidence" && question.Projection.Answer != nil {
			decision, citations, err := ParseDecisionEvidence(*question.Projection.Answer, condition.Sources)
			if err == nil {
				score.decision = decision
				score.citations = citations
			}
		}
	}
	sort.Strings(score.citations)
	score.correct = score.decision == condition.Adjudication.Decision
	score.review = score.decision != "accept"
	score.unsafe = score.decision == "accept" && condition.Adjudication.Decision != "accept"
	return score
}

func computeBaseDeltas(scores []observationScore, bases []Base) []BaseDelta {
	var out []BaseDelta
	for _, base := range bases {
		arm := map[string][]observationScore{ArmRaw: {}, ArmDistillLock: {}}
		for _, score := range scores {
			if score.receipt.BaseID == base.ID {
				arm[score.receipt.Arm] = append(arm[score.receipt.Arm], score)
			}
		}
		aCorrect, cCorrect := rate(arm[ArmRaw], func(score observationScore) bool { return score.correct }), rate(arm[ArmDistillLock], func(score observationScore) bool { return score.correct })
		aReview, cReview := rate(arm[ArmRaw], func(score observationScore) bool { return score.review }), rate(arm[ArmDistillLock], func(score observationScore) bool { return score.review })
		aUnsafe, cUnsafe := rate(arm[ArmRaw], func(score observationScore) bool { return score.unsafe }), rate(arm[ArmDistillLock], func(score observationScore) bool { return score.unsafe })
		out = append(out, BaseDelta{
			BaseID: base.ID, Dataset: base.Dataset, ArmACorrectness: aCorrect, ArmCCorrectness: cCorrect,
			CorrectnessDeltaCMinusA: cCorrect - aCorrect, ArmAReviewBurden: aReview, ArmCReviewBurden: cReview,
			ReviewBurdenDeltaCMinusA: cReview - aReview, ArmAUnsafeAccept: aUnsafe, ArmCUnsafeAccept: cUnsafe,
			UnsafeAcceptDeltaCMinusA: cUnsafe - aUnsafe,
		})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].BaseID < out[j].BaseID })
	return out
}

func rate(scores []observationScore, predicate func(observationScore) bool) float64 {
	if len(scores) == 0 {
		return 0
	}
	count := 0
	for _, score := range scores {
		if predicate(score) {
			count++
		}
	}
	return float64(count) / float64(len(scores))
}

func mean(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	var total float64
	for _, value := range values {
		total += value
	}
	return total / float64(len(values))
}

func median(values []float64) float64 {
	if len(values) == 0 {
		return 0
	}
	sorted := append([]float64(nil), values...)
	sort.Float64s(sorted)
	if len(sorted)%2 == 1 {
		return sorted[len(sorted)/2]
	}
	return (sorted[len(sorted)/2-1] + sorted[len(sorted)/2]) / 2
}

func exactSignFlipP(values []float64) float64 {
	observed := math.Abs(mean(values))
	extreme := 0
	total := 1 << len(values)
	for assignment := 0; assignment < total; assignment++ {
		var sum float64
		for i, value := range values {
			if assignment&(1<<i) == 0 {
				sum += value
			} else {
				sum -= value
			}
		}
		if math.Abs(sum/float64(len(values))) >= observed-1e-15 {
			extreme++
		}
	}
	return float64(extreme) / float64(total)
}

func leaveOneOut(deltas []BaseDelta) []LeaveOneOut {
	out := make([]LeaveOneOut, 0, len(deltas))
	for omitted := range deltas {
		var correctness, review, unsafe []float64
		for i, delta := range deltas {
			if i == omitted {
				continue
			}
			correctness = append(correctness, delta.CorrectnessDeltaCMinusA)
			review = append(review, delta.ReviewBurdenDeltaCMinusA)
			unsafe = append(unsafe, delta.UnsafeAcceptDeltaCMinusA)
		}
		out = append(out, LeaveOneOut{
			OmittedBaseID: deltas[omitted].BaseID, MeanCorrectnessDelta: mean(correctness),
			MeanReviewBurdenDelta: mean(review), MeanUnsafeAcceptDelta: mean(unsafe),
		})
	}
	return out
}

func computeFamilyDeltas(scores []observationScore, conditions map[string]Condition) []FamilyDelta {
	type familyGroup struct {
		conditions map[string]bool
		bases      map[string]bool
		arms       map[string][]observationScore
	}
	groups := map[string]*familyGroup{}
	for _, score := range scores {
		family := conditions[score.receipt.ConditionID].Family
		group := groups[family]
		if group == nil {
			group = &familyGroup{
				conditions: map[string]bool{}, bases: map[string]bool{},
				arms: map[string][]observationScore{ArmRaw: {}, ArmDistillLock: {}},
			}
			groups[family] = group
		}
		group.conditions[score.receipt.ConditionID] = true
		group.bases[score.receipt.BaseID] = true
		group.arms[score.receipt.Arm] = append(group.arms[score.receipt.Arm], score)
	}
	families := make([]string, 0, len(groups))
	for family := range groups {
		families = append(families, family)
	}
	sort.Strings(families)
	out := make([]FamilyDelta, 0, len(families))
	for _, family := range families {
		group := groups[family]
		aCorrect := rate(group.arms[ArmRaw], func(score observationScore) bool { return score.correct })
		cCorrect := rate(group.arms[ArmDistillLock], func(score observationScore) bool { return score.correct })
		aReview := rate(group.arms[ArmRaw], func(score observationScore) bool { return score.review })
		cReview := rate(group.arms[ArmDistillLock], func(score observationScore) bool { return score.review })
		out = append(out, FamilyDelta{
			Family: family, Bases: len(group.bases), Conditions: len(group.conditions),
			CorrectnessDeltaCMinusA:  cCorrect - aCorrect,
			ReviewBurdenDeltaCMinusA: cReview - aReview,
		})
	}
	return out
}

func computeRiskOperatingPoints(
	scores []observationScore,
	bases []Base,
	conditions map[string]Condition,
	rules OutcomeRules,
) ([]RiskOperatingPoint, string) {
	var points []RiskOperatingPoint
	buildPoint := func(dataset, rule string) RiskOperatingPoint {
		point := RiskOperatingPoint{Dataset: dataset, Rule: rule}
		perBaseAccepted := map[string]int{}
		perBaseTotal := map[string]int{}
		for _, score := range scores {
			if score.receipt.Dataset != dataset {
				continue
			}
			point.Total++
			perBaseTotal[score.receipt.BaseID]++
			if structuralAccept(rule, score, conditions[score.receipt.ConditionID]) {
				point.Accepted++
				perBaseAccepted[score.receipt.BaseID]++
				if score.unsafe {
					point.Unsafe++
				}
			}
		}
		for _, base := range bases {
			if base.Dataset != dataset {
				continue
			}
			point.Bases++
			if perBaseTotal[base.ID] > 0 {
				point.BaseWeightedCoverage += float64(perBaseAccepted[base.ID]) / float64(perBaseTotal[base.ID])
			}
		}
		if point.Bases > 0 {
			point.BaseWeightedCoverage /= float64(point.Bases)
		}
		if point.Total > 0 {
			point.Coverage = float64(point.Accepted) / float64(point.Total)
		}
		if point.Accepted > 0 {
			risk := float64(point.Unsafe) / float64(point.Accepted)
			point.UnsafeRisk = &risk
		}
		return point
	}
	for _, rule := range rules.RoutingRuleOrder {
		points = append(points, buildPoint("distill", rule))
	}
	selected := "no-safe-auto-action"
	bestCoverage := -1.0
	for _, point := range points {
		if point.UnsafeRisk == nil || *point.UnsafeRisk > rules.RoutingUnsafeCeiling ||
			point.Accepted < rules.RoutingMinimumAccepted {
			continue
		}
		if point.BaseWeightedCoverage > bestCoverage {
			bestCoverage = point.BaseWeightedCoverage
			selected = point.Rule
		}
	}
	if selected != "no-safe-auto-action" {
		points = append(points, buildPoint("llmtracefx", selected))
	}
	return points, selected
}

func structuralAccept(rule string, score observationScore, condition Condition) bool {
	if score.receipt.Status != "valid" || score.decision != "accept" {
		return false
	}
	switch rule {
	case "generated_accept":
		return true
	case "accept_with_any_evidence":
		return len(score.citations) > 0
	case "accept_with_all_evidence":
		want := make([]string, len(condition.Sources))
		for i, source := range condition.Sources {
			want[i] = source.ID
		}
		sort.Strings(want)
		return fmt.Sprint(want) == fmt.Sprint(score.citations)
	default:
		return false
	}
}

func buildCaseExplorer(scores []observationScore, conditions map[string]Condition) []CaseExplorerRow {
	rows := map[string]*CaseExplorerRow{}
	for _, score := range scores {
		condition := conditions[score.receipt.ConditionID]
		row := rows[condition.ID]
		if row == nil {
			row = &CaseExplorerRow{
				ConditionID: condition.ID, BaseID: condition.BaseID,
				Dataset: condition.Dataset, Family: condition.Family, Truth: condition.Adjudication.Decision,
			}
			rows[condition.ID] = row
		}
		if score.receipt.Arm == ArmRaw {
			row.ArmAStatus, row.ArmADecision = score.receipt.Status, score.decision
			row.ArmACitations = append([]string(nil), score.citations...)
		} else {
			row.ArmCStatus, row.ArmCDecision = score.receipt.Status, score.decision
			row.ArmCCitations = append([]string(nil), score.citations...)
		}
	}
	ids := make([]string, 0, len(rows))
	for id := range rows {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	out := make([]CaseExplorerRow, 0, len(ids))
	for _, id := range ids {
		out = append(out, *rows[id])
	}
	return out
}

func computeEfficiency(scores []observationScore) []ArmEfficiency {
	out := []ArmEfficiency{{Arm: ArmRaw}, {Arm: ArmDistillLock}}
	for i := range out {
		var rates []float64
		for _, score := range scores {
			if score.receipt.Arm != out[i].Arm {
				continue
			}
			out[i].Observations++
			out[i].InputTokens += score.input
			out[i].OutputTokens += score.output
			out[i].MeanRequestLatencyNanos += float64(score.receipt.RequestLatencyNanos)
			if score.receipt.ResourceSummary.MaximumSampledRSSBytes > out[i].MaximumSampledRSSBytes {
				out[i].MaximumSampledRSSBytes = score.receipt.ResourceSummary.MaximumSampledRSSBytes
			}
			for _, question := range score.receipt.Questions {
				if question.TokensPerSecond != nil {
					rates = append(rates, *question.TokensPerSecond)
				}
				if question.Memory.PeakBytes > out[i].MaximumMLXPeakBytes {
					out[i].MaximumMLXPeakBytes = question.Memory.PeakBytes
				}
			}
		}
		if out[i].Observations > 0 {
			out[i].MeanRequestLatencyNanos /= float64(out[i].Observations)
		}
		if len(rates) > 0 {
			rate := mean(rates)
			out[i].MeanOfPostFirstTokenRates = &rate
		}
		out[i].Provenance = "tokens are exact persisted tokenizer/generated token IDs; request latency is Go monotonic elapsed time around the adapter round-trip after initial resource preflight, while concurrent two-second resource-monitor subprocess probes remain inside the measured window and can differentially perturb longer observations, so per-arm latency is descriptive rather than a clean head-to-head performance effect; each post-first-token rate is (generated tokens - 1) divided by synchronized decode duration and this field is their arithmetic mean; RSS is sampled and MLX peak is allocator-reported"
	}
	return out
}

func repeatMetrics(scores []observationScore) RepeatMetrics {
	groups := map[string][]observationScore{}
	for _, score := range scores {
		if score.receipt.Replicate > 0 {
			key := score.receipt.ConditionID + "\x00" + score.receipt.Arm
			groups[key] = append(groups[key], score)
		}
	}
	var metrics RepeatMetrics
	var decisionAgree, routeAgree, malformedAgree int
	var jaccards, latencyCVs, tokenCVs []float64
	for _, group := range groups {
		if len(group) != 3 {
			continue
		}
		sort.Slice(group, func(i, j int) bool { return group[i].receipt.Replicate < group[j].receipt.Replicate })
		metrics.RepeatedConditionArmPairs++
		if group[0].decision == group[1].decision && group[1].decision == group[2].decision {
			decisionAgree++
		}
		if group[0].review == group[1].review && group[1].review == group[2].review {
			routeAgree++
		}
		if group[0].malformed == group[1].malformed && group[1].malformed == group[2].malformed {
			malformedAgree++
		}
		jaccards = append(jaccards, threeWayJaccard(group[0].citations, group[1].citations, group[2].citations))
		latencyCVs = append(latencyCVs, coefficientOfVariation([]float64{
			float64(group[0].receipt.RequestLatencyNanos), float64(group[1].receipt.RequestLatencyNanos), float64(group[2].receipt.RequestLatencyNanos),
		}))
		tokenCVs = append(tokenCVs, coefficientOfVariation([]float64{float64(group[0].output), float64(group[1].output), float64(group[2].output)}))
	}
	if metrics.RepeatedConditionArmPairs > 0 {
		denominator := float64(metrics.RepeatedConditionArmPairs)
		metrics.AllThreeDecisionAgreement = float64(decisionAgree) / denominator
		metrics.AllThreeRouteAgreement = float64(routeAgree) / denominator
		metrics.AllThreeMalformedAgreement = float64(malformedAgree) / denominator
		metrics.MeanEvidenceJaccard = mean(jaccards)
		metrics.MeanLatencyCV = mean(latencyCVs)
		metrics.MeanOutputTokenCV = mean(tokenCVs)
	}
	return metrics
}

func threeWayJaccard(values ...[]string) float64 {
	union := map[string]bool{}
	counts := map[string]int{}
	for _, items := range values {
		seen := map[string]bool{}
		for _, item := range items {
			union[item] = true
			seen[item] = true
		}
		for item := range seen {
			counts[item]++
		}
	}
	if len(union) == 0 {
		return 1
	}
	intersection := 0
	for _, count := range counts {
		if count == len(values) {
			intersection++
		}
	}
	return float64(intersection) / float64(len(union))
}

func coefficientOfVariation(values []float64) float64 {
	average := mean(values)
	if average == 0 {
		return 0
	}
	var variance float64
	for _, value := range values {
		delta := value - average
		variance += delta * delta
	}
	variance /= float64(len(values))
	return math.Sqrt(variance) / average
}
