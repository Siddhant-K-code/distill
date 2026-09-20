package studypilot

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/dlclark/regexp2"
	"github.com/gowebpki/jcs"
	"github.com/santhosh-tekuri/jsonschema/v6"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

var movingAlias = regexp.MustCompile(`(?i)(^|[-_/:@.])(latest|stable|preview|newest)($|[-_/:@.])`)

type ecmaRegexp struct {
	*regexp2.Regexp
}

func (expression ecmaRegexp) MatchString(value string) bool {
	matched, err := expression.Regexp.MatchString(value)
	return err == nil && matched
}

// ReceiptRegistry is the mandatory pre-execution registry. Callers must fill
// every identity and supply exact artifact bytes; validation fails closed when
// external evidence cannot be resolved.
type ReceiptRegistry struct {
	CaseID                       string
	RepositoryURL                string
	RepositoryCommit             string
	DatasetSnapshotDigest        string
	SplitAssignmentDigest        string
	CaseManifestDigest           string
	GroundTruthCommitmentDigest  string
	AccessLogDigest              string
	ScheduledCallID              string
	ScheduleIndex                int
	ReplicateIndex               int
	ContextArm                   string
	RepositoryRole               string
	RunScheduleDigest            string
	SourceSetDigest              string
	BaseSourceSetDigests         map[string]bool
	TransformationDigest         string
	CompilerDigest               string
	CompiledContextDigest        string
	QuestionSchemaDigest         string
	DecisionSystemDigest         string
	DecisionSystemVersion        string
	Provider                     string
	ModelID                      string
	ModelVersion                 string
	ModelSnapshotDigest          string
	ParametersDigest             string
	APIVersion                   string
	SDKName                      string
	SDKVersion                   string
	SDKPackageDigest             string
	ConfidenceSemanticsVersion   string
	ExecutionAuthorizationDigest string
	PolicyID                     string
	PolicyVersion                string
	PolicyDigest                 string
	ThresholdSetDigest           string
	AcceptanceDefinitionDigest   string
	ValidatorConfigDigest        string
	SemanticValidatorDigest      string
	Contradiction                bool
	Stale                        bool
	Applicability                Applicability
	Artifacts                    map[string][]byte
	ParseProviderArtifacts       func(map[string][]byte) (ProviderObservation, error)
}

// ProviderObservation is the normalized result of a pinned parser over exact
// provider artifacts. The parser implementation is bound by the semantic
// validator digest in the registry and receipt.
type ProviderObservation struct {
	Outputs           []any
	Usage             map[string]any
	Cost              map[string]any
	ProviderRequestID string
}

// ValidateReceipt applies the frozen Draft 2020-12 schema with format
// assertion, then the cross-record and digest checks in receipt-validation.md.
func ValidateReceipt(data []byte, registry ReceiptRegistry) error {
	if err := validateRegistry(registry); err != nil {
		return err
	}
	if len(data) > maxArtifactBytes {
		return fmt.Errorf("receipt exceeds size limit")
	}
	if err := validateJSONStructuralLimits(data); err != nil {
		return err
	}
	canonical, err := jcs.Transform(data)
	if err != nil || !bytes.Equal(canonical, data) {
		return fmt.Errorf("receipt is not RFC 8785 canonical JSON")
	}
	var receipt map[string]any
	decoder := json.NewDecoder(bytes.NewReader(data))
	if err := decoder.Decode(&receipt); err != nil {
		return fmt.Errorf("decode receipt: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return fmt.Errorf("receipt has trailing JSON value")
	}
	if err := checkJSONValue(receipt, 0); err != nil {
		return err
	}
	if err := validateReceiptSchema(receipt); err != nil {
		return err
	}
	if err := validateReceiptIdentities(receipt, registry); err != nil {
		return err
	}
	if err := validateReceiptQuestions(receipt, registry); err != nil {
		return err
	}
	if err := validateReceiptCustody(receipt); err != nil {
		return err
	}
	if err := validateReceiptMeasurement(receipt); err != nil {
		return err
	}

	if err := validateReceiptArtifacts(receipt, registry); err != nil {
		return err
	}
	if err := validateReceiptSourceAndPerturbation(receipt, registry); err != nil {
		return err
	}
	if err := validateReceiptPolicy(receipt, registry); err != nil {
		return err
	}
	if err := validateReceiptDigests(receipt); err != nil {
		return err
	}
	return nil
}

func validateReceiptCustody(receipt map[string]any) error {
	commitmentDigest := stringAt(receipt, "ground_truth_commitment_digest")
	if stringAt(receipt, "custody", "commitment_published_digest") != commitmentDigest ||
		boolAt(receipt, "custody", "labels_available_to_runner") {
		return fmt.Errorf("ground-truth custody commitment mismatch")
	}
	stage := stringAt(receipt, "record_stage")
	if stage == "runner" {
		if valueAt(receipt, "ground_truth") != nil ||
			valueAt(receipt, "custody", "reveal_nonce") != nil ||
			valueAt(receipt, "custody", "unsealed_at") != nil {
			return fmt.Errorf("runner receipt exposes sealed labels")
		}
		return nil
	}
	nonceText := stringAt(receipt, "custody", "reveal_nonce")
	nonce, err := decodeNonce(nonceText)
	if err != nil {
		return err
	}
	truth := valueAt(receipt, "ground_truth")
	truthBytes, err := canonicalJSON(truth)
	if err != nil {
		return err
	}
	preimage := append([]byte("context-build-artifact/ground-truth-commitment/v1\n"), nonce...)
	preimage = append(preimage, truthBytes...)
	if digestBytes(preimage) != commitmentDigest {
		return fmt.Errorf("revealed ground truth does not match commitment")
	}
	answers := sliceAt(mapAt(receipt, "ground_truth"), "answers")
	if len(answers) != len(Questions) {
		return fmt.Errorf("ground truth does not contain exactly seven answers")
	}
	seen := make(map[string]bool)
	for _, raw := range answers {
		answer, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("ground-truth question set mismatch")
		}
		questionID := stringAt(answer, "question_id")
		question, exists := questionByID(questionID)
		if !exists || seen[questionID] {
			return fmt.Errorf("ground-truth question set mismatch")
		}
		seen[questionID] = true
		label := stringAt(answer, "label")
		if label != "indeterminate" && !contains(question.Choices, label) {
			return fmt.Errorf("ground-truth label is invalid for %s", questionID)
		}
	}
	return nil
}

// SealReceipt recomputes the three frozen structured digests in dependency
// order. It is useful to construct offline fixtures; validation remains
// separate and still requires an external registry.
func SealReceipt(receipt map[string]any) error {
	identityProjection := receiptIdentityProjection(receipt)
	identity, err := structuredDigest("condition", identityProjection)
	if err != nil {
		return err
	}
	receipt["identity_digest"] = identity
	outcome, err := structuredDigest("outcome", receiptOutcomeProjection(receipt))
	if err != nil {
		return err
	}
	receipt["outcome_digest"] = outcome
	full := make(map[string]any, len(receipt))
	for key, value := range receipt {
		if key != "receipt_digest" {
			full[key] = value
		}
	}
	digest, err := structuredDigest("receipt", full)
	if err != nil {
		return err
	}
	receipt["receipt_digest"] = digest
	return nil
}

func validateRegistry(registry ReceiptRegistry) error {
	required := map[string]string{
		"case ID": registry.CaseID, "scheduled call ID": registry.ScheduledCallID,
		"context arm": registry.ContextArm, "repository role": registry.RepositoryRole,
		"repository URL": registry.RepositoryURL, "repository commit": registry.RepositoryCommit,
		"dataset snapshot digest": registry.DatasetSnapshotDigest,
		"split assignment digest": registry.SplitAssignmentDigest,
		"case manifest digest":    registry.CaseManifestDigest,
		"ground-truth commitment": registry.GroundTruthCommitmentDigest,
		"access log":              registry.AccessLogDigest,
		"run schedule digest":     registry.RunScheduleDigest,
		"source-set digest":       registry.SourceSetDigest,
		"transformation digest":   registry.TransformationDigest,
		"compiler digest":         registry.CompilerDigest,
		"compiled-context digest": registry.CompiledContextDigest,
		"question schema digest":  registry.QuestionSchemaDigest,
		"decision system digest":  registry.DecisionSystemDigest,
		"policy ID":               registry.PolicyID, "policy version": registry.PolicyVersion,
		"policy digest": registry.PolicyDigest, "threshold set digest": registry.ThresholdSetDigest,
		"acceptance definition digest": registry.AcceptanceDefinitionDigest,
		"validator config digest":      registry.ValidatorConfigDigest,
		"semantic validator digest":    registry.SemanticValidatorDigest,
	}
	for name, value := range required {
		if value == "" {
			return fmt.Errorf("external artifact registry is required: missing %s", name)
		}
		if registry.ScheduleIndex < 0 || registry.ReplicateIndex < 1 || registry.ReplicateIndex > 3 {
			return fmt.Errorf("external artifact registry has invalid schedule coordinates")
		}
	}
	if registry.Artifacts == nil {
		return fmt.Errorf("external artifact registry is required: missing artifact resolver")
	}
	if registry.BaseSourceSetDigests == nil {
		return fmt.Errorf("external artifact registry is required: missing base source-set registry")
	}
	frozen, err := frozenIdentities()
	if err != nil {
		return err
	}
	if registry.QuestionSchemaDigest != frozen.QuestionSchemaDigest ||
		registry.PolicyID != PolicyID || registry.PolicyVersion != PolicyVersion ||
		registry.PolicyDigest != frozen.PolicyDigest ||
		registry.ThresholdSetDigest != digestText("context-build-artifact/not-applicable-threshold-set/v1") ||
		registry.AcceptanceDefinitionDigest != digestText(AcceptanceScoreVersion) {
		return fmt.Errorf("registry does not match the frozen offline pilot question/policy identities")
	}
	return nil
}

func validateReceiptSchema(receipt any) error {
	if digestBytes(contextartifact.PilotSchema) != contextartifact.PilotSchemaSHA256 {
		return fmt.Errorf("embedded authoritative receipt schema digest mismatch")
	}
	compiler := jsonschema.NewCompiler()
	compiler.DefaultDraft(jsonschema.Draft2020)
	compiler.AssertFormat()
	compiler.UseRegexpEngine(func(expression string) (jsonschema.Regexp, error) {
		compiled, err := regexp2.Compile(expression, regexp2.ECMAScript)
		if err != nil {
			return nil, err
		}
		return ecmaRegexp{Regexp: compiled}, nil
	})
	var schemaDocument any
	if err := json.Unmarshal(contextartifact.PilotSchema, &schemaDocument); err != nil {
		return fmt.Errorf("decode receipt schema: %w", err)
	}

	if err := compiler.AddResource("urn:distill:context-build-artifact:receipt:v1", schemaDocument); err != nil {
		return fmt.Errorf("load receipt schema: %w", err)
	}
	schema, err := compiler.Compile("urn:distill:context-build-artifact:receipt:v1")
	if err != nil {
		return fmt.Errorf("compile receipt schema: %w", err)
	}
	if err := schema.Validate(receipt); err != nil {
		return fmt.Errorf("receipt schema validation: %w", err)
	}
	return nil
}

func validateReceiptIdentities(receipt map[string]any, registry ReceiptRegistry) error {
	if stringAt(receipt, "schema_version") != "context-build-artifact/receipt/v1" ||
		stringAt(receipt, "receipt_schema_digest") != contextartifact.PilotSchemaSHA256 ||
		stringAt(receipt, "study_phase") != "pilot" ||
		stringAt(receipt, "case", "split") != "pilot" ||
		stringAt(receipt, "case", "repository_role") != "pilot_development" {
		return fmt.Errorf("receipt is outside the excluded pilot namespace")
	}
	caseID := stringAt(receipt, "case", "case_id")
	callID := stringAt(receipt, "measurement", "scheduled_call_id")
	if !strings.HasPrefix(caseID, "pilot-") || !strings.HasPrefix(callID, "pilot-") ||
		caseID != registry.CaseID || callID != registry.ScheduledCallID {
		return fmt.Errorf("case or scheduled-call identity mismatch")
	}
	if intAt(receipt, "measurement", "schedule_index") != registry.ScheduleIndex ||
		intAt(receipt, "measurement", "replicate_index") != registry.ReplicateIndex ||
		stringAt(receipt, "compiled_context", "arm") != registry.ContextArm ||
		stringAt(receipt, "case", "repository_role") != registry.RepositoryRole {
		return fmt.Errorf("receipt does not match the registered schedule entry")
	}
	exact := []struct {
		name   string
		actual string
		want   string
	}{
		{"run schedule", stringAt(receipt, "run_schedule_digest"), registry.RunScheduleDigest},
		{"repository URL", stringAt(receipt, "case", "repository_url"), registry.RepositoryURL},
		{"repository commit", stringAt(receipt, "case", "repository_commit"), registry.RepositoryCommit},
		{"dataset snapshot", stringAt(receipt, "case", "dataset_snapshot_digest"), registry.DatasetSnapshotDigest},
		{"split assignment", stringAt(receipt, "case", "split_assignment_digest"), registry.SplitAssignmentDigest},
		{"case manifest", stringAt(receipt, "case", "case_manifest_digest"), registry.CaseManifestDigest},
		{"ground-truth commitment", stringAt(receipt, "ground_truth_commitment_digest"), registry.GroundTruthCommitmentDigest},
		{"published commitment", stringAt(receipt, "custody", "commitment_published_digest"), registry.GroundTruthCommitmentDigest},
		{"access log", stringAt(receipt, "custody", "access_log_digest"), registry.AccessLogDigest},
		{"source set", stringAt(receipt, "source_set", "source_set_digest"), registry.SourceSetDigest},
		{"transformation", stringAt(receipt, "perturbation", "transformation_receipt_digest"), registry.TransformationDigest},
		{"compiler", stringAt(receipt, "compiled_context", "compiler_digest"), registry.CompilerDigest},
		{"compiled context", stringAt(receipt, "compiled_context", "compiled_context_digest"), registry.CompiledContextDigest},
		{"question schema", stringAt(receipt, "question_schema", "question_schema_digest"), registry.QuestionSchemaDigest},
		{"decision system", stringAt(receipt, "decision_system", "system_digest"), registry.DecisionSystemDigest},
		{"policy ID", stringAt(receipt, "policy", "policy_id"), registry.PolicyID},
		{"policy version", stringAt(receipt, "policy", "policy_version"), registry.PolicyVersion},
		{"policy", stringAt(receipt, "policy", "policy_digest"), registry.PolicyDigest},
		{"threshold set", stringAt(receipt, "policy", "threshold_set_digest"), registry.ThresholdSetDigest},
		{"acceptance definition", stringAt(receipt, "policy", "acceptance_score_definition_digest"), registry.AcceptanceDefinitionDigest},
		{"validator config", stringAt(receipt, "validator", "validator_config_digest"), registry.ValidatorConfigDigest},
		{"semantic validator", stringAt(receipt, "validator", "semantic_validator_digest"), registry.SemanticValidatorDigest},
	}
	for _, item := range exact {
		if item.actual != item.want {
			return fmt.Errorf("%s identity mismatch", item.name)
		}
	}
	if stringAt(receipt, "validator", "json_schema_validator") != "github.com/santhosh-tekuri/jsonschema/v6" ||
		stringAt(receipt, "validator", "json_schema_validator_version") != "v6.0.3" ||
		!boolAt(receipt, "validator", "format_assertion_enabled") ||
		stringAt(receipt, "validator", "semantic_validator_version") != "context-build-artifact/semantic-validator/v1" {
		return fmt.Errorf("validator identity mismatch")
	}
	if stringAt(receipt, "question_schema", "schema_id") != QuestionSchemaID ||
		stringAt(receipt, "question_schema", "schema_version") != QuestionSchemaVersion {
		return fmt.Errorf("question schema identity mismatch")
	}

	kind := stringAt(receipt, "decision_system", "kind")
	model := valueAt(receipt, "decision_system", "model")
	if kind == "deterministic_policy" {
		if model != nil || valueAt(receipt, "decision_system", "api_version") != nil ||
			valueAt(receipt, "decision_system", "sdk") != nil ||
			valueAt(receipt, "execution_authorization") != nil {
			return fmt.Errorf("deterministic policy contains provider identity")
		}
		if registry.ModelID != "" || registry.ModelVersion != "" || registry.APIVersion != "" ||
			registry.SDKName != "" || registry.SDKVersion != "" {
			return fmt.Errorf("registry supplies provider identity for deterministic policy")
		}
		return nil
	}
	if kind != "jev_pinned" && kind != "frontier_llm_pinned" {
		return fmt.Errorf("unknown decision-system kind")
	}
	if registry.ModelID == "" || registry.ModelVersion == "" || registry.ModelSnapshotDigest == "" ||
		registry.ParametersDigest == "" || registry.APIVersion == "" || registry.SDKName == "" ||
		registry.SDKVersion == "" || registry.SDKPackageDigest == "" ||
		registry.ExecutionAuthorizationDigest == "" || registry.ParseProviderArtifacts == nil ||
		registry.Provider == "" || registry.DecisionSystemVersion == "" ||
		registry.ConfidenceSemanticsVersion == "" {
		return fmt.Errorf("external registry lacks exact model/API/SDK identity")
	}
	modelID := stringAt(receipt, "decision_system", "model", "model_id")
	modelVersion := stringAt(receipt, "decision_system", "model", "model_version")
	if movingAlias.MatchString(modelID) || movingAlias.MatchString(modelVersion) {
		return fmt.Errorf("moving model alias is forbidden")
	}
	providerExact := []struct{ actual, want string }{
		{stringAt(receipt, "decision_system", "system_version"), registry.DecisionSystemVersion},
		{stringAt(receipt, "decision_system", "model", "provider"), registry.Provider},
		{modelID, registry.ModelID},
		{modelVersion, registry.ModelVersion},
		{stringAt(receipt, "decision_system", "model", "model_snapshot_digest"), registry.ModelSnapshotDigest},
		{stringAt(receipt, "decision_system", "model", "parameters_digest"), registry.ParametersDigest},
		{stringAt(receipt, "decision_system", "api_version"), registry.APIVersion},
		{stringAt(receipt, "decision_system", "sdk", "name"), registry.SDKName},
		{stringAt(receipt, "decision_system", "sdk", "version"), registry.SDKVersion},
		{stringAt(receipt, "decision_system", "sdk", "package_digest"), registry.SDKPackageDigest},
		{stringAt(receipt, "decision_system", "model", "confidence_semantics_version"), registry.ConfidenceSemanticsVersion},
	}
	for _, item := range providerExact {
		if item.actual != item.want {
			return fmt.Errorf("model/API/SDK registry mismatch")
		}
	}
	if stringAt(receipt, "execution_authorization", "stage") != "excluded_provider_pilot" {
		return fmt.Errorf("provider execution is not authorized for excluded pilot")
	}
	authorizationBytes, err := canonicalJSON(valueAt(receipt, "execution_authorization"))
	if err != nil || digestBytes(authorizationBytes) != registry.ExecutionAuthorizationDigest {
		return fmt.Errorf("execution authorization registry mismatch")
	}
	return nil
}

func validateReceiptQuestions(receipt map[string]any, registry ReceiptRegistry) error {
	questionIDs := stringsAt(receipt, "question_schema", "question_ids")
	if !equalStringSets(questionIDs, questionIDList()) {
		return fmt.Errorf("question schema is not the exact seven-question set")
	}
	status := stringAt(receipt, "decision", "status")
	outputs, _ := valueAt(receipt, "decision", "outputs").([]any)
	if status != "valid" {
		if len(outputs) != 0 {
			return fmt.Errorf("non-valid decision has outputs")
		}
		return nil
	}
	if len(outputs) != len(Questions) {
		return fmt.Errorf("valid decision must contain exactly seven outputs")
	}
	probabilistic := stringAt(receipt, "decision_system", "kind") != "deterministic_policy"
	seen := make(map[string]bool)
	for _, raw := range outputs {
		output, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("decision output question order/set mismatch")
		}
		questionID := stringAt(output, "question_id")
		question, exists := questionByID(questionID)
		if !exists || seen[questionID] {
			return fmt.Errorf("decision output question order/set mismatch")
		}
		seen[questionID] = true
		selected := stringAt(output, "selected_label")
		if !contains(question.Choices, selected) {
			return fmt.Errorf("question %s selected invalid label", questionID)
		}
		if applicable, tracked := receiptApplicability(registry.Applicability, questionID); tracked {
			if applicable && selected == "not_applicable" {
				return fmt.Errorf("question %s is applicable but selected not_applicable", questionID)
			}
			if !applicable && selected != "not_applicable" {
				return fmt.Errorf("question %s is inapplicable but selected %s", questionID, selected)
			}
		}
		probabilities, probabilitiesOK := output["probabilities"].(map[string]any)
		if !probabilistic {
			if output["probabilities"] != nil || output["confidence"] != nil ||
				stringAt(output, "confidence_semantics") != "not_available" {
				return fmt.Errorf("deterministic output must not claim probabilities")
			}
			continue
		}
		if !probabilitiesOK || len(probabilities) != len(question.Choices) {
			return fmt.Errorf("question %s probability keys are incomplete", questionID)
		}
		sum := 0.0
		for _, choice := range question.Choices {
			value, ok := probabilities[choice].(float64)
			if !ok || math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 1 {
				return fmt.Errorf("question %s has invalid probability", questionID)
			}
			sum += value
		}
		if math.Abs(sum-1) > 1e-9 {
			return fmt.Errorf("question %s probabilities do not sum to one", questionID)
		}
		confidence, ok := output["confidence"].(float64)
		if !ok || math.IsNaN(confidence) || math.IsInf(confidence, 0) ||
			stringAt(output, "confidence_semantics") != "selected_label_probability" ||
			math.Abs(confidence-probabilities[selected].(float64)) > 1e-12 {
			return fmt.Errorf("question %s confidence semantics mismatch", questionID)
		}
	}
	return nil
}

func receiptApplicability(applicability Applicability, questionID string) (bool, bool) {
	switch questionID {
	case "observed_tests_support":
		return applicability.ObservedTests, true
	case "verifier_support":
		return applicability.Verifier, true
	case "cleanup_complete":
		return applicability.Cleanup, true
	case "external_effects_resolved":
		return applicability.ExternalEffects, true
	default:
		return false, false
	}
}

func validateReceiptMeasurement(receipt map[string]any) error {
	status := stringAt(receipt, "decision", "status")
	attempts := intAt(receipt, "measurement", "attempt_count")
	if intAt(receipt, "measurement", "retry_count") != 0 {
		return fmt.Errorf("receipt retries are forbidden")
	}
	if status == "not_attempted" {
		if attempts != 0 || valueAt(receipt, "measurement", "attempt_started_at") != nil ||
			valueAt(receipt, "measurement", "attempt_finished_at") != nil ||
			valueAt(receipt, "measurement", "latency_ms") != nil ||
			valueAt(receipt, "measurement", "usage") != nil ||
			valueAt(receipt, "measurement", "provider_reported_cost") != nil ||
			valueAt(receipt, "measurement", "provider_request_id") != nil ||
			len(sliceAt(receipt, "measurement", "rate_limit_waits")) != 0 {
			return fmt.Errorf("not-attempted measurement contains success-shaped fields")
		}
		return validateReceiptError(receipt)
	}
	if attempts != 1 {
		return fmt.Errorf("attempted decision must have exactly one attempt")
	}
	start, err := time.Parse(time.RFC3339Nano, stringAt(receipt, "measurement", "attempt_started_at"))
	if err != nil {
		return fmt.Errorf("invalid attempt start timestamp")
	}
	finish, err := time.Parse(time.RFC3339Nano, stringAt(receipt, "measurement", "attempt_finished_at"))
	if err != nil || finish.Before(start) {
		return fmt.Errorf("invalid attempt finish timestamp")
	}
	latency, ok := valueAt(receipt, "measurement", "latency_ms").(float64)
	if !ok || math.IsNaN(latency) || math.IsInf(latency, 0) || latency < 0 {
		return fmt.Errorf("invalid latency")
	}
	for _, raw := range sliceAt(receipt, "measurement", "rate_limit_waits") {
		wait := raw.(map[string]any)
		waitStart, err := time.Parse(time.RFC3339Nano, stringAt(wait, "started_at"))
		duration, ok := wait["duration_ms"].(float64)
		if err != nil || !ok || math.IsNaN(duration) || math.IsInf(duration, 0) || duration < 0 ||
			waitStart.Before(start) || waitStart.After(finish) ||
			waitStart.Add(time.Duration(duration*float64(time.Millisecond))).After(finish) {
			return fmt.Errorf("rate-limit wait falls outside attempt interval")
		}
	}
	if status == "valid" && stringAt(receipt, "decision_system", "kind") != "deterministic_policy" {
		if valueAt(receipt, "measurement", "usage") == nil ||
			stringAt(receipt, "measurement", "provider_request_id") == "" {
			return fmt.Errorf("valid provider decision lacks usage or request identity")
		}
		usage := mapAt(receipt, "measurement", "usage")
		if intAt(usage, "total_tokens") != intAt(usage, "input_tokens")+intAt(usage, "output_tokens") {
			return fmt.Errorf("provider token usage total mismatch")
		}
	}
	if status == "failed" {
		return validateReceiptError(receipt)
	}
	if valueAt(receipt, "decision", "error") != nil {
		return fmt.Errorf("valid decision contains an error")
	}
	return nil
}

func validateReceiptError(receipt map[string]any) error {
	errorValue, ok := valueAt(receipt, "decision", "error").(map[string]any)
	if !ok {
		return fmt.Errorf("failed/not-attempted decision lacks structured error")
	}
	if intAt(errorValue, "retry_count") != 0 {
		return fmt.Errorf("error retry count is not zero")
	}
	if stringAt(receipt, "decision", "status") == "not_attempted" && stringAt(errorValue, "stage") != "scheduler" {
		return fmt.Errorf("not-attempted error must be scheduler stage")
	}
	return nil
}

func validateReceiptArtifacts(receipt map[string]any, registry ReceiptRegistry) error {
	artifacts := registry.Artifacts
	all := append([]any(nil), sliceAt(receipt, "evidence_hashes")...)
	all = append(all, sliceAt(receipt, "receipt_artifact_hashes")...)
	seen := make(map[string]bool)
	for _, raw := range all {
		entry, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid artifact entry")
		}
		id := stringAt(entry, "artifact_id")
		if seen[id] {
			return fmt.Errorf("duplicate artifact ID %q", id)
		}
		seen[id] = true
		data, ok := artifacts[id]
		if !ok {
			return fmt.Errorf("external artifact registry cannot resolve %q", id)
		}
		if intAt(entry, "byte_length") != len(data) || stringAt(entry, "sha256") != digestBytes(data) {
			return fmt.Errorf("artifact %q identity mismatch", id)
		}
	}
	contextMatched := false
	for _, raw := range sliceAt(receipt, "compiled_context", "artifact_hashes") {
		entry, ok := raw.(map[string]any)
		if !ok {
			return fmt.Errorf("invalid compiled-context artifact entry")
		}
		id := stringAt(entry, "artifact_id")
		data, ok := artifacts[id]
		if !ok || len(data) != intAt(entry, "byte_length") || digestBytes(data) != stringAt(entry, "sha256") {
			return fmt.Errorf("compiled-context artifact %q does not resolve", id)
		}
		if len(data) == intAt(receipt, "compiled_context", "context_byte_length") &&
			digestBytes(data) == stringAt(receipt, "compiled_context", "compiled_context_digest") {
			contextMatched = true
		}
	}
	if !contextMatched {
		return fmt.Errorf("compiled context bytes do not resolve")
	}
	status := stringAt(receipt, "decision", "status")
	if status == "valid" {
		rawDigest := stringAt(receipt, "decision", "raw_output_digest")
		if !artifactDigestPresent(sliceAt(receipt, "receipt_artifact_hashes"), rawDigest) {
			return fmt.Errorf("raw output digest is not bound as a receipt artifact")
		}
		if usage := mapAt(receipt, "measurement", "usage"); usage != nil {
			if !artifactDigestPresent(sliceAt(receipt, "receipt_artifact_hashes"), stringAt(usage, "provider_usage_json_digest")) {
				return fmt.Errorf("provider usage digest is not bound as a receipt artifact")
			}
		}
		if stringAt(receipt, "decision_system", "kind") != "deterministic_policy" {
			observation, err := registry.ParseProviderArtifacts(registry.Artifacts)
			if err != nil {
				return fmt.Errorf("parse pinned provider artifacts: %w", err)
			}
			if !canonicalEqual(observation.Outputs, sliceAt(receipt, "decision", "outputs")) ||
				!canonicalEqual(observation.Usage, copyKeys(mapAt(receipt, "measurement", "usage"),
					"input_tokens", "output_tokens", "total_tokens")) ||
				!canonicalEqual(observation.Cost, mapAt(receipt, "measurement", "provider_reported_cost")) ||
				observation.ProviderRequestID != stringAt(receipt, "measurement", "provider_request_id") {
				return fmt.Errorf("structured provider fields do not match pinned artifact parser")
			}
		}
	}
	if status == "failed" || status == "not_attempted" {
		errorValue := mapAt(receipt, "decision", "error")
		id := stringAt(errorValue, "sanitized_error_artifact_id")
		data, ok := artifacts[id]
		if !ok || digestBytes(data) != stringAt(errorValue, "error_detail_digest") {
			return fmt.Errorf("error artifact does not resolve")
		}
		found := false
		for _, raw := range sliceAt(receipt, "receipt_artifact_hashes") {
			if stringAt(raw.(map[string]any), "artifact_id") == id {
				found = true
			}
		}
		if !found {
			return fmt.Errorf("error artifact is not outcome-dependent")
		}
	}
	return nil
}

func artifactDigestPresent(entries []any, digest string) bool {
	for _, raw := range entries {
		entry, ok := raw.(map[string]any)
		if ok && stringAt(entry, "sha256") == digest {
			return true
		}
	}
	return false
}

func validateReceiptSourceAndPerturbation(receipt map[string]any, registry ReceiptRegistry) error {
	artifacts := registry.Artifacts
	sourceSet := mapAt(receipt, "source_set")
	manifest := stringsAt(sourceSet, "manifest_order")
	sources := sliceAt(sourceSet, "sources")
	if len(manifest) != len(sources) {
		return fmt.Errorf("source manifest cardinality mismatch")
	}
	ids := make(map[string]bool)
	paths := make(map[string]bool)
	projectedSources := make([]map[string]any, 0, len(sources))
	for _, raw := range sources {
		source := raw.(map[string]any)
		id := stringAt(source, "source_id")
		path := stringAt(source, "relative_path")
		if ids[id] || paths[path] {
			return fmt.Errorf("duplicate source ID or path")
		}
		ids[id], paths[path] = true, true
		data, ok := artifacts[id]
		if !ok || len(data) != intAt(source, "byte_length") || digestBytes(data) != stringAt(source, "content_digest") {
			return fmt.Errorf("source artifact %q does not resolve", id)
		}
		if !utf8.Valid(data) || stringAt(source, "normalized_content_digest") != digestText(normalizeText(string(data))) {
			return fmt.Errorf("source artifact %q normalized identity mismatch", id)
		}
		projectedSources = append(projectedSources, copyKeys(source,
			"source_id", "relative_path", "media_type", "byte_length", "content_digest",
			"normalized_content_digest", "evidence_role"))
	}
	for _, id := range manifest {
		if !ids[id] {
			return fmt.Errorf("source manifest references unknown ID")
		}
		delete(ids, id)
	}
	if len(ids) != 0 {
		return fmt.Errorf("source manifest omits source IDs")
	}
	sort.Slice(projectedSources, func(i, j int) bool {
		return projectedSources[i]["source_id"].(string) < projectedSources[j]["source_id"].(string)
	})
	sourceDigest, err := structuredDigest("source-set", map[string]any{
		"source_set_id": sourceSet["source_set_id"], "manifest_order": sourceSet["manifest_order"], "sources": projectedSources,
	})
	if err != nil || sourceDigest != stringAt(sourceSet, "source_set_digest") {
		return fmt.Errorf("receipt source-set digest mismatch")
	}
	resultDigest := stringAt(receipt, "perturbation", "result_source_set_digest")
	if resultDigest != sourceDigest || stringAt(receipt, "compiled_context", "input_source_set_digest") != sourceDigest {
		return fmt.Errorf("source/perturbation/context digest linkage mismatch")
	}
	perturbationType := stringAt(receipt, "perturbation", "type")
	if stringAt(receipt, "perturbation", "expected_equivalence") != equivalenceFor(perturbationType) {
		return fmt.Errorf("perturbation equivalence mismatch")
	}
	baseDigest := stringAt(receipt, "perturbation", "base_source_set_digest")
	if !registry.BaseSourceSetDigests[baseDigest] {
		return fmt.Errorf("base source set does not resolve in pre-execution registry")
	}
	if perturbationType == "none" && baseDigest != resultDigest {
		return fmt.Errorf("unperturbed source digest mismatch")
	}
	transformID := stringAt(receipt, "perturbation", "transformation_receipt_artifact_id")
	transformData, ok := artifacts[transformID]
	if !ok {
		return fmt.Errorf("transformation receipt is unresolved")
	}
	transformEvidenceFound := false
	for _, raw := range sliceAt(receipt, "evidence_hashes") {
		entry := raw.(map[string]any)
		if stringAt(entry, "artifact_id") == transformID && stringAt(entry, "sha256") == digestBytes(transformData) {
			transformEvidenceFound = true
		}
	}
	if !transformEvidenceFound {
		return fmt.Errorf("transformation receipt is not pre-execution evidence")
	}
	transformValue, err := decodeCanonicalValue(transformData)
	if err != nil {
		return fmt.Errorf("decode transformation receipt: %w", err)
	}
	transform, ok := transformValue.(map[string]any)
	if !ok {
		return fmt.Errorf("transformation receipt must be an object")
	}
	expectedTransform := map[string]any{
		"case_id":                  stringAt(receipt, "case", "case_id"),
		"perturbation_id":          stringAt(receipt, "perturbation", "perturbation_id"),
		"type":                     perturbationType,
		"transform_version":        stringAt(receipt, "perturbation", "transform_version"),
		"seed":                     valueAt(receipt, "perturbation", "seed"),
		"expected_equivalence":     stringAt(receipt, "perturbation", "expected_equivalence"),
		"base_source_set_digest":   stringAt(receipt, "perturbation", "base_source_set_digest"),
		"result_source_set_digest": resultDigest,
		"operations":               transform["operations"],
	}
	expectedBytes, expectedErr := canonicalJSON(expectedTransform)
	transformBytes, transformErr := canonicalJSON(transform)
	if expectedErr != nil || transformErr != nil || !bytes.Equal(expectedBytes, transformBytes) {
		return fmt.Errorf("transformation receipt fields do not bind receipt")
	}
	transformDigest, err := structuredDigest("transformation", transform)
	if err != nil || transformDigest != stringAt(receipt, "perturbation", "transformation_receipt_digest") {
		return fmt.Errorf("transformation receipt digest mismatch")
	}
	return nil
}

func decodeCanonicalValue(data []byte) (any, error) {
	if len(data) > maxArtifactBytes {
		return nil, fmt.Errorf("artifact exceeds size limit")
	}
	if err := validateJSONStructuralLimits(data); err != nil {
		return nil, err
	}
	canonical, err := jcs.Transform(data)
	if err != nil {
		return nil, err
	}
	if !bytes.Equal(canonical, data) {
		return nil, fmt.Errorf("artifact is not RFC 8785 canonical JSON")
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	var value any
	if err := decoder.Decode(&value); err != nil {
		return nil, err
	}
	var extra any
	if err := decoder.Decode(&extra); err == nil {
		return nil, fmt.Errorf("artifact has trailing JSON value")
	} else if err != io.EOF {
		return nil, fmt.Errorf("decode trailing JSON: %w", err)
	}
	if err := checkJSONValue(value, 0); err != nil {
		return nil, err
	}
	return value, nil
}

func canonicalEqual(left, right any) bool {
	leftBytes, leftErr := canonicalJSON(left)
	rightBytes, rightErr := canonicalJSON(right)
	return leftErr == nil && rightErr == nil && bytes.Equal(leftBytes, rightBytes)
}

func validateReceiptPolicy(receipt map[string]any, registry ReceiptRegistry) error {
	status := stringAt(receipt, "decision", "status")
	policy := mapAt(receipt, "policy")
	if status != "valid" {
		expected, _ := EvaluatePolicy(status, Evidence{})
		if stringAt(policy, "result") != expected.Result ||
			!equalStrings(stringsAt(policy, "reason_codes"), expected.ReasonCodes) ||
			valueAt(policy, "acceptance_score") != nil {
			return fmt.Errorf("failed/not-attempted policy mismatch")
		}
		return nil
	}
	outputs := sliceAt(receipt, "decision", "outputs")
	values := make(map[string]string, len(outputs))
	for _, raw := range outputs {
		output := raw.(map[string]any)
		values[stringAt(output, "question_id")] = stringAt(output, "selected_label")
	}
	evidence := Evidence{
		EvidenceComplete:        values["evidence_complete"],
		ObservedTestsSupport:    values["observed_tests_support"],
		VerifierSupport:         values["verifier_support"],
		CleanupComplete:         values["cleanup_complete"],
		ExternalEffectsResolved: values["external_effects_resolved"],
		PatchRisk:               values["patch_risk"],
		RecommendedDisposition:  values["recommended_disposition"],
		Contradiction:           registry.Contradiction,
		Stale:                   registry.Stale,
		Applicability:           registry.Applicability,
	}
	expected, err := EvaluatePolicy("valid", evidence)
	if err != nil || stringAt(policy, "result") != expected.Result ||
		!equalStrings(stringsAt(policy, "reason_codes"), expected.ReasonCodes) {
		return fmt.Errorf("policy result or ordered reason codes do not recompute")
	}
	probabilistic := stringAt(receipt, "decision_system", "kind") != "deterministic_policy"
	if !probabilistic {
		if valueAt(policy, "acceptance_score") != nil {
			return fmt.Errorf("deterministic policy acceptance score must be null")
		}
		return nil
	}
	expectedScore, err := acceptanceScore(outputs, registry.Contradiction, registry.Stale, registry.Applicability)
	if err != nil {
		return err
	}
	actualScore, ok := valueAt(policy, "acceptance_score").(float64)
	if !ok || math.Abs(actualScore-expectedScore) > 1e-12 {
		return fmt.Errorf("acceptance score does not recompute")
	}
	return nil
}

func acceptanceScore(outputs []any, contradiction, stale bool, applicability Applicability) (float64, error) {
	if contradiction || stale {
		return 0, nil
	}
	safeLabels := map[string][]string{
		"evidence_complete": {"yes"}, "observed_tests_support": applicableSafeLabels(applicability.ObservedTests),
		"verifier_support": applicableSafeLabels(applicability.Verifier), "cleanup_complete": applicableSafeLabels(applicability.Cleanup),
		"external_effects_resolved": applicableSafeLabels(applicability.ExternalEffects), "patch_risk": {"low"},
		"recommended_disposition": {"accept"},
	}
	score := 1.0
	for _, raw := range outputs {
		output := raw.(map[string]any)
		probabilities, ok := output["probabilities"].(map[string]any)
		if !ok {
			return 0, fmt.Errorf("probabilistic score lacks probabilities")
		}
		safeProbability := 0.0
		for _, label := range safeLabels[stringAt(output, "question_id")] {
			safeProbability += probabilities[label].(float64)
		}
		if safeProbability < score {
			score = safeProbability
		}
	}
	return score, nil
}

func applicableSafeLabels(applicable bool) []string {
	if applicable {
		return []string{"yes"}
	}
	return []string{"not_applicable"}
}

// ValidateReceiptStageCollection requires exactly one terminal receipt for
// every registered call in one record stage, with no duplicates or extras.
func ValidateReceiptStageCollection(receipts [][]byte, registries map[string]ReceiptRegistry, stage string) error {
	if stage != "runner" && stage != "analysis" {
		return fmt.Errorf("unknown receipt stage %q", stage)
	}
	if len(receipts) != len(registries) {
		return fmt.Errorf("receipt collection cardinality mismatch")
	}
	seen := make(map[string]bool, len(receipts))
	for _, data := range receipts {
		value, err := decodeCanonicalValue(data)
		if err != nil {
			return err
		}
		receipt, ok := value.(map[string]any)
		if !ok {
			return fmt.Errorf("receipt must be an object")
		}
		if stringAt(receipt, "record_stage") != stage {
			return fmt.Errorf("receipt stage mismatch")
		}
		callID := stringAt(receipt, "measurement", "scheduled_call_id")
		registry, exists := registries[callID]
		if !exists || seen[callID] {
			return fmt.Errorf("duplicate or extra scheduled call %q", callID)
		}
		if err := ValidateReceipt(data, registry); err != nil {
			return fmt.Errorf("%s: %w", callID, err)
		}
		seen[callID] = true
	}
	if len(seen) != len(registries) {
		return fmt.Errorf("receipt collection omits scheduled calls")
	}
	return nil
}

// ValidateReceiptCollection enforces the frozen per-stage bijection and the
// one-to-one runner/analysis pairing by scheduled call and condition identity.
func ValidateReceiptCollection(runners, analyses [][]byte, registries map[string]ReceiptRegistry) error {
	if err := ValidateReceiptStageCollection(runners, registries, "runner"); err != nil {
		return err
	}
	if err := ValidateReceiptStageCollection(analyses, registries, "analysis"); err != nil {
		return err
	}
	runnerIdentities, err := collectionIdentities(runners)
	if err != nil {
		return err
	}
	analysisIdentities, err := collectionIdentities(analyses)
	if err != nil {
		return err
	}
	for callID, identity := range runnerIdentities {
		if analysisIdentities[callID] != identity {
			return fmt.Errorf("runner/analysis identity mismatch for %q", callID)
		}
	}
	return nil
}

func collectionIdentities(receipts [][]byte) (map[string]string, error) {
	identities := make(map[string]string, len(receipts))
	for _, data := range receipts {
		value, err := decodeCanonicalValue(data)
		if err != nil {
			return nil, err
		}
		receipt := value.(map[string]any)
		callID := stringAt(receipt, "measurement", "scheduled_call_id")
		if _, exists := identities[callID]; exists {
			return nil, fmt.Errorf("duplicate scheduled call %q", callID)
		}
		identities[callID] = stringAt(receipt, "identity_digest")
	}
	return identities, nil
}

func validateReceiptDigests(receipt map[string]any) error {
	identityProjection := receiptIdentityProjection(receipt)
	identity, err := structuredDigest("condition", identityProjection)
	if err != nil || identity != stringAt(receipt, "identity_digest") {
		return fmt.Errorf("identity digest mismatch")
	}
	outcomeProjection := receiptOutcomeProjection(receipt)
	outcome, err := structuredDigest("outcome", outcomeProjection)
	if err != nil || outcome != stringAt(receipt, "outcome_digest") {
		return fmt.Errorf("outcome digest mismatch")
	}
	full := make(map[string]any, len(receipt)-1)
	for key, value := range receipt {
		if key != "receipt_digest" {
			full[key] = value
		}
	}
	receiptDigest, err := structuredDigest("receipt", full)
	if err != nil || receiptDigest != stringAt(receipt, "receipt_digest") {
		return fmt.Errorf("receipt digest mismatch")
	}
	return nil
}

func receiptIdentityProjection(receipt map[string]any) map[string]any {
	return map[string]any{
		"schema_version": receipt["schema_version"], "receipt_schema_digest": receipt["receipt_schema_digest"],
		"study_phase": receipt["study_phase"], "run_schedule_digest": receipt["run_schedule_digest"],
		"case": receipt["case"], "source_set": receipt["source_set"], "perturbation": receipt["perturbation"],
		"ground_truth_commitment_digest": receipt["ground_truth_commitment_digest"],
		"custody":                        copyKeys(mapAt(receipt, "custody"), "labels_available_to_runner", "commitment_published_digest"),
		"compiled_context":               receipt["compiled_context"], "question_schema": receipt["question_schema"],
		"decision_system": receipt["decision_system"], "execution_authorization": receipt["execution_authorization"],
		"policy": copyKeys(mapAt(receipt, "policy"), "policy_id", "policy_version", "policy_digest",
			"threshold_set_digest", "acceptance_score_definition_digest"),
		"validator": receipt["validator"], "evidence_hashes": sortedEvidence(sliceAt(receipt, "evidence_hashes")),
	}
}

func receiptOutcomeProjection(receipt map[string]any) map[string]any {
	return map[string]any{
		"identity_digest":                receipt["identity_digest"],
		"ground_truth_commitment_digest": receipt["ground_truth_commitment_digest"],
		"ground_truth":                   receipt["ground_truth"], "decision": receipt["decision"],
		"policy":       copyKeys(mapAt(receipt, "policy"), "result", "reason_codes", "acceptance_score"),
		"run_validity": receipt["run_validity"],
	}
}

func questionIDList() []string {
	ids := make([]string, len(Questions))
	for index := range Questions {
		ids[index] = Questions[index].ID
	}
	return ids
}

func questionByID(id string) (Question, bool) {
	for _, question := range Questions {
		if question.ID == id {
			return question, true
		}
	}
	return Question{}, false
}

func valueAt(root map[string]any, path ...string) any {
	var current any = root
	for _, part := range path {
		object, ok := current.(map[string]any)
		if !ok {
			return nil
		}
		current = object[part]
	}
	return current
}

func mapAt(root map[string]any, path ...string) map[string]any {
	value, _ := valueAt(root, path...).(map[string]any)
	return value
}

func sliceAt(root map[string]any, path ...string) []any {
	value, _ := valueAt(root, path...).([]any)
	return value
}

func stringAt(root map[string]any, path ...string) string {
	value, _ := valueAt(root, path...).(string)
	return value
}

func boolAt(root map[string]any, path ...string) bool {
	value, _ := valueAt(root, path...).(bool)
	return value
}

func intAt(root map[string]any, path ...string) int {
	value, _ := valueAt(root, path...).(float64)
	return int(value)
}

func stringsAt(root map[string]any, path ...string) []string {
	values := sliceAt(root, path...)
	result := make([]string, 0, len(values))
	for _, value := range values {
		text, ok := value.(string)
		if !ok {
			return nil
		}
		result = append(result, text)
	}
	return result
}

func copyKeys(source map[string]any, keys ...string) map[string]any {
	result := make(map[string]any, len(keys))
	for _, key := range keys {
		if value, exists := source[key]; exists {
			result[key] = value
		}
	}
	return result
}

func sortedEvidence(values []any) []any {
	result := append([]any(nil), values...)
	sort.Slice(result, func(i, j int) bool {
		left := result[i].(map[string]any)
		right := result[j].(map[string]any)
		if stringAt(left, "artifact_id") == stringAt(right, "artifact_id") {
			return stringAt(left, "sha256") < stringAt(right, "sha256")
		}
		return stringAt(left, "artifact_id") < stringAt(right, "artifact_id")
	})
	return result
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func equalStringSets(left, right []string) bool {
	leftCopy, rightCopy := append([]string(nil), left...), append([]string(nil), right...)
	sort.Strings(leftCopy)
	sort.Strings(rightCopy)
	return equalStrings(leftCopy, rightCopy)
}

func contains(values []string, candidate string) bool {
	for _, value := range values {
		if value == candidate {
			return true
		}
	}
	return false
}

func decodeNonce(value string) ([]byte, error) {
	decoded, err := hex.DecodeString(value)
	if err != nil || len(decoded) != 32 {
		return nil, fmt.Errorf("invalid ground-truth reveal nonce")
	}
	return decoded, nil
}
