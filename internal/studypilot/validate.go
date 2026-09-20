package studypilot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

// Validate verifies a complete offline pilot artifact directory without network access.
func Validate(directory string) (Summary, error) {
	absolute, err := filepath.Abs(directory)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve artifact directory: %w", err)
	}
	directory = absolute
	info, err := os.Lstat(directory)
	if err != nil {
		return Summary{}, fmt.Errorf("inspect artifact directory: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return Summary{}, fmt.Errorf("artifact path must be a directory, not a symlink")
	}
	if err := validateExistingSafeDirectory(directory); err != nil {
		return Summary{}, fmt.Errorf("unsafe artifact directory: %w", err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return Summary{}, err
	}
	expected := make(map[string]bool, len(artifactFileNames))
	for _, name := range artifactFileNames {
		expected[name] = true
	}
	if len(entries) != len(expected) {
		return Summary{}, fmt.Errorf("artifact file set mismatch: got %d entries, want %d", len(entries), len(expected))
	}
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		if !expected[entry.Name()] {
			return Summary{}, fmt.Errorf("unexpected artifact %q", entry.Name())
		}
		path := filepath.Join(directory, entry.Name())
		fileInfo, err := os.Lstat(path)
		if err != nil {
			return Summary{}, err
		}
		if fileInfo.Mode()&os.ModeSymlink != 0 || !fileInfo.Mode().IsRegular() {
			return Summary{}, fmt.Errorf("artifact %q must be a regular file, not a symlink", entry.Name())
		}
		if err := validateSafeInfo(path, fileInfo, false); err != nil {
			return Summary{}, fmt.Errorf("unsafe artifact %q: %w", entry.Name(), err)
		}
		if fileInfo.Size() > maxArtifactBytes {
			return Summary{}, fmt.Errorf("artifact %q exceeds size limit", entry.Name())
		}
		data, err := readValidatedArtifact(path, fileInfo)
		if err != nil {
			return Summary{}, err
		}
		files[entry.Name()] = data
	}
	expectedFiles, err := BuildArtifacts(nil)
	if err != nil {
		return Summary{}, fmt.Errorf("regenerate expected artifact set: %w", err)
	}
	for _, name := range artifactFileNames {
		if !bytes.Equal(files[name], expectedFiles[name]) {
			return Summary{}, fmt.Errorf("artifact %q differs from deterministic registered corpus", name)
		}
	}
	if !bytes.Equal(files["receipt-schema.json"], contextartifact.PilotSchema) {
		return Summary{}, fmt.Errorf("receipt-schema.json is not the exact authoritative bytes")
	}
	if digestBytes(files["receipt-schema.json"]) != contextartifact.PilotSchemaSHA256 {
		return Summary{}, fmt.Errorf("receipt schema digest mismatch")
	}
	if err := validateChecksums(files); err != nil {
		return Summary{}, err
	}

	var manifest Manifest
	if err := validateCanonicalJSONFile(files["pilot-manifest.json"], &manifest); err != nil {
		return Summary{}, fmt.Errorf("pilot-manifest.json: %w", err)
	}
	if err := validateEnvelope(manifest.RecordEnvelope); err != nil {
		return Summary{}, fmt.Errorf("manifest: %w", err)
	}
	if manifest.ArtifactSetID != "context-build-artifact/excluded-offline-pilot/v1" ||
		manifest.ProtocolClarification != "research/context-is-a-build-artifact/pilot-protocol-v1.md" ||
		manifest.ProviderCalls != 0 || manifest.LabelsAvailableToRequests ||
		manifest.CaseCount != 18 {
		return Summary{}, fmt.Errorf("manifest pilot identity or exclusion fields mismatch")
	}
	frozen, err := frozenIdentities()
	if err != nil {
		return Summary{}, err
	}
	if !reflect.DeepEqual(manifest.Frozen, frozen) {
		return Summary{}, fmt.Errorf("manifest frozen identities mismatch")
	}
	if err := validateManifestFiles(manifest.Files, files); err != nil {
		return Summary{}, err
	}

	cases, err := parseJSONL[CaseRecord](files["cases.jsonl"])
	if err != nil {
		return Summary{}, fmt.Errorf("cases.jsonl: %w", err)
	}
	labels, err := parseJSONL[LabelRecord](files["labels.jsonl"])
	if err != nil {
		return Summary{}, fmt.Errorf("labels.jsonl: %w", err)
	}
	requests, err := parseJSONL[RequestRecord](files["requests.jsonl"])
	if err != nil {
		return Summary{}, fmt.Errorf("requests.jsonl: %w", err)
	}
	policies, err := parseJSONL[PolicyRecord](files["policy-baseline.jsonl"])
	if err != nil {
		return Summary{}, fmt.Errorf("policy-baseline.jsonl: %w", err)
	}
	schedule, err := parseJSONL[ScheduleRecord](files["schedule.jsonl"])
	if err != nil {
		return Summary{}, fmt.Errorf("schedule.jsonl: %w", err)
	}
	if len(cases) < 15 || len(cases) > 20 || len(labels) != len(cases) ||
		len(requests) != len(cases) || len(policies) != len(cases) || len(schedule) != len(cases) {
		return Summary{}, fmt.Errorf("pilot count/bijection mismatch")
	}
	if err := validateCollections(cases, labels, requests, policies, schedule, frozen); err != nil {
		return Summary{}, err
	}
	if err := privacyScan(files); err != nil {
		return Summary{}, err
	}

	summary := Summary{
		CaseCount:             len(cases),
		RequestCount:          len(requests),
		ProviderCalls:         0,
		ManifestSHA256:        digestBytes(files["pilot-manifest.json"]),
		ChecksumsSHA256:       digestBytes(files["SHA256SUMS"]),
		DatasetSnapshotDigest: manifest.Frozen.DatasetSnapshotDigest,
	}
	for _, record := range policies {
		switch record.Result.Result {
		case "accept":
			summary.PolicyAccept++
		case "review":
			summary.PolicyReview++
		case "reject":
			summary.PolicyReject++
		}
	}
	return summary, nil
}

func validateCanonicalJSONFile(data []byte, target any) error {
	if len(data) == 0 || data[len(data)-1] != '\n' || bytes.Contains(data, []byte{'\r'}) {
		return fmt.Errorf("canonical JSON file must end in one LF and contain no CR")
	}
	body := data[:len(data)-1]
	if bytes.Contains(body, []byte{'\n'}) {
		return fmt.Errorf("JCS file must contain a single JSON line")
	}
	if err := validateCanonicalLine(body); err != nil {
		return err
	}
	return decodeStrict(body, target)
}

func validateChecksums(files map[string][]byte) error {
	expected := renderChecksums(mapWithoutChecksums(files))
	if !bytes.Equal(files["SHA256SUMS"], expected) {
		return fmt.Errorf("SHA256SUMS mismatch")
	}
	lines := strings.Split(strings.TrimSuffix(string(files["SHA256SUMS"]), "\n"), "\n")
	if len(lines) != len(files)-1 {
		return fmt.Errorf("SHA256SUMS does not bind every non-self file")
	}
	return nil
}

func mapWithoutChecksums(files map[string][]byte) map[string][]byte {
	copyMap := make(map[string][]byte, len(files)-1)
	for name, data := range files {
		if name != "SHA256SUMS" {
			copyMap[name] = data
		}
	}
	return copyMap
}

func validateManifestFiles(identities []FileIdentity, files map[string][]byte) error {
	expectedNames := []string{"cases.jsonl", "labels.jsonl", "policy-baseline.jsonl", "receipt-schema.json", "requests.jsonl", "schedule.jsonl"}
	if len(identities) != len(expectedNames) {
		return fmt.Errorf("manifest file inventory length mismatch")
	}
	for index, identity := range identities {
		if identity.Path != expectedNames[index] {
			return fmt.Errorf("manifest file order mismatch")
		}
		data := files[identity.Path]
		if identity.Bytes != len(data) || identity.SHA256 != digestBytes(data) {
			return fmt.Errorf("manifest identity mismatch for %q", identity.Path)
		}
	}
	return nil
}

func validateCollections(cases []CaseRecord, labels []LabelRecord, requests []RequestRecord, policies []PolicyRecord, schedule []ScheduleRecord, frozen FrozenIdentities) error {
	requiredCategories := map[string]bool{
		"source_reorder": false, "exact_duplicate_insertion": false,
		"line_endings_lf": false, "line_endings_crlf": false, "line_endings_cr": false,
		"metadata_noise": false, "identical_content_path_rename": false,
		"irrelevant_append": false, "one_relevant_fact_change": false,
		"missing_observed_test_evidence": false, "claimed_tests_not_observed": false,
		"verifier_failure": false, "stale_baseline": false, "missing_cleanup_evidence": false,
		"unresolved_external_effects": false, "contradictory_summary_execution": false,
		"fully_complete_verified": false, "generic_missing_required_evidence": false,
	}
	labelByCase := make(map[string]LabelRecord)
	requestByCase := make(map[string]RequestRecord)
	policyByCase := make(map[string]PolicyRecord)
	scheduleByCase := make(map[string]ScheduleRecord)
	for _, record := range labels {
		if _, exists := labelByCase[record.CaseID]; exists {
			return fmt.Errorf("duplicate label case %q", record.CaseID)
		}
		labelByCase[record.CaseID] = record
	}
	for _, record := range requests {
		if _, exists := requestByCase[record.CaseID]; exists {
			return fmt.Errorf("duplicate request case %q", record.CaseID)
		}
		requestByCase[record.CaseID] = record
	}
	for _, record := range policies {
		if _, exists := policyByCase[record.CaseID]; exists {
			return fmt.Errorf("duplicate policy case %q", record.CaseID)
		}
		policyByCase[record.CaseID] = record
	}
	for _, record := range schedule {
		if _, exists := scheduleByCase[record.CaseID]; exists {
			return fmt.Errorf("duplicate schedule case %q", record.CaseID)
		}
		scheduleByCase[record.CaseID] = record
	}

	seenCases := make(map[string]bool)
	for index, record := range cases {
		if err := validateEnvelope(record.RecordEnvelope); err != nil {
			return fmt.Errorf("%s: %w", record.CaseID, err)
		}
		if seenCases[record.CaseID] || record.CaseID != fmt.Sprintf("pilot-case-%03d", index+1) {
			return fmt.Errorf("case IDs are duplicate or unstable at index %d", index)
		}
		seenCases[record.CaseID] = true
		if _, known := requiredCategories[record.Category]; !known {
			return fmt.Errorf("%s has unknown category %q", record.CaseID, record.Category)
		}
		requiredCategories[record.Category] = true
		if record.RepositoryURL != "https://example.org/public-domain/synthetic-fixtures" ||
			record.RepositoryRole != "pilot_development" || record.Split != "pilot" ||
			!reflect.DeepEqual(record.Frozen, frozen) {
			return fmt.Errorf("%s has non-pilot or unfrozen identity", record.CaseID)
		}
		if err := validateSourceSet(record.SourceSet); err != nil {
			return fmt.Errorf("%s: %w", record.CaseID, err)
		}
		if err := validateSourceSet(record.BaseSourceSet); err != nil {
			return fmt.Errorf("%s base: %w", record.CaseID, err)
		}
		if record.Perturbation.ResultSourceSetDigest != record.SourceSet.SourceSetDigest ||
			record.Perturbation.BaseSourceSetDigest != record.BaseSourceSet.SourceSetDigest ||
			record.Perturbation.ExpectedEquivalence != equivalenceFor(record.Perturbation.Type) ||
			record.Perturbation.TransformVersion != TransformVersion ||
			record.Perturbation.Seed != caseSeed(record.CaseID) ||
			record.Perturbation.TransformationReceiptArtifactID != record.CaseID+"-transformation-receipt" {
			return fmt.Errorf("%s perturbation mapping mismatch", record.CaseID)
		}
		recomputedTransformation, err := transformationDigest(record.CaseID, record.Perturbation)
		if err != nil || recomputedTransformation != record.Perturbation.TransformationReceiptDigest {
			return fmt.Errorf("%s transformation digest mismatch", record.CaseID)
		}
		expectedPolicy, err := EvaluatePolicy("valid", record.Evidence)
		if err != nil || expectedPolicy.Result != record.ExpectedBaselineAction {
			return fmt.Errorf("%s expected baseline action mismatch", record.CaseID)
		}

		label, labelOK := labelByCase[record.CaseID]
		request, requestOK := requestByCase[record.CaseID]
		policy, policyOK := policyByCase[record.CaseID]
		scheduled, scheduleOK := scheduleByCase[record.CaseID]
		if !labelOK || !requestOK || !policyOK || !scheduleOK {
			return fmt.Errorf("%s lacks a bijective label/request/policy/schedule record", record.CaseID)
		}
		if err := validateLabel(record, label); err != nil {
			return fmt.Errorf("%s label: %w", record.CaseID, err)
		}
		if err := validateRequest(record, request, frozen); err != nil {
			return fmt.Errorf("%s request: %w", record.CaseID, err)
		}
		if err := validatePolicyRecord(record, policy, frozen); err != nil {
			return fmt.Errorf("%s policy: %w", record.CaseID, err)
		}
		if err := validateSchedule(record, request, scheduled, index, frozen); err != nil {
			return fmt.Errorf("%s schedule: %w", record.CaseID, err)
		}
	}
	for category, covered := range requiredCategories {
		if !covered {
			return fmt.Errorf("missing required category %q", category)
		}
	}
	if len(seenCases) != len(labelByCase) || len(seenCases) != len(requestByCase) ||
		len(seenCases) != len(policyByCase) || len(seenCases) != len(scheduleByCase) {
		return fmt.Errorf("collection contains extra records")
	}
	return nil
}

func readValidatedArtifact(path string, before os.FileInfo) ([]byte, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() {
		_ = file.Close()
	}()
	opened, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !opened.Mode().IsRegular() || !os.SameFile(before, opened) {
		return nil, fmt.Errorf("artifact %q changed while opening", filepath.Base(path))
	}
	data, err := io.ReadAll(io.LimitReader(file, maxArtifactBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxArtifactBytes {
		return nil, fmt.Errorf("artifact %q exceeds size limit", filepath.Base(path))
	}
	after, err := file.Stat()
	if err != nil {
		return nil, err
	}
	if !os.SameFile(opened, after) || opened.Size() != after.Size() {
		return nil, fmt.Errorf("artifact %q changed while reading", filepath.Base(path))
	}
	return data, nil
}

func validateEnvelope(record RecordEnvelope) error {
	if record.SchemaVersion != ArtifactSchemaVersion || record.Phase != "pilot" ||
		!record.Excluded || record.FinalStudyEligible {
		return fmt.Errorf("record is not permanently excluded pilot data")
	}
	return nil
}

func validateSourceSet(set SourceSet) error {
	if len(set.Sources) == 0 || len(set.ManifestOrder) != len(set.Sources) {
		return fmt.Errorf("source manifest cardinality mismatch")
	}
	ids := make(map[string]bool)
	paths := make(map[string]bool)
	for _, source := range set.Sources {
		if ids[source.SourceID] || paths[source.RelativePath] {
			return fmt.Errorf("duplicate source ID or path")
		}
		if !strings.HasPrefix(source.SourceID, "pilot-case-") ||
			!validPortablePath(source.RelativePath) ||
			source.MediaType != "text/plain" {
			return fmt.Errorf("source identity, path, or media type is unsafe")
		}
		ids[source.SourceID] = true
		paths[source.RelativePath] = true
		if source.ByteLength != len([]byte(source.Content)) ||
			source.ContentDigest != digestText(source.Content) ||
			source.NormalizedContentDigest != digestText(normalizeText(source.Content)) {
			return fmt.Errorf("source hash/length mismatch")
		}
	}
	manifestSeen := make(map[string]bool)
	for _, id := range set.ManifestOrder {
		if !ids[id] || manifestSeen[id] {
			return fmt.Errorf("source manifest is not a bijection")
		}
		manifestSeen[id] = true
	}
	digest, err := sourceSetDigest(set)
	if err != nil || digest != set.SourceSetDigest {
		return fmt.Errorf("source-set digest mismatch")
	}
	return nil
}

func validPortablePath(value string) bool {
	if value == "" || strings.Contains(value, "\\") || strings.ContainsRune(value, 0) ||
		strings.HasPrefix(value, "/") || path.Clean(value) != value {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}

func validateLabel(caseRecord CaseRecord, label LabelRecord) error {
	if err := validateEnvelope(label.RecordEnvelope); err != nil {
		return err
	}
	if !label.SealedFromRequests || label.LabelSealDigest != caseRecord.LabelSealDigest {
		return fmt.Errorf("label sealing/commitment mismatch")
	}
	if label.SourceSetDigest != caseRecord.SourceSet.SourceSetDigest ||
		!reflect.DeepEqual(label.Frozen, caseRecord.Frozen) {
		return fmt.Errorf("label source or frozen identity mismatch")
	}
	labelSealDigest, err := labelSeal(caseRecord.CaseID, label.GroundTruth)
	if err != nil || labelSealDigest != label.LabelSealDigest {
		return fmt.Errorf("ground-truth commitment does not recompute")
	}
	if len(label.GroundTruth.Answers) != len(Questions) ||
		label.GroundTruth.PolicyEvidence.Contradiction != caseRecord.Evidence.Contradiction ||
		label.GroundTruth.PolicyEvidence.Stale != caseRecord.Evidence.Stale ||
		len(label.GroundTruth.DerivationReasons) == 0 {
		return fmt.Errorf("ground truth is incomplete")
	}
	values := evidenceValues(caseRecord.Evidence)
	for index, answer := range label.GroundTruth.Answers {
		if answer.QuestionID != Questions[index].ID || answer.Label != values[answer.QuestionID] ||
			len(answer.SupportingEvidenceDigests) != 1 ||
			answer.SupportingEvidenceDigests[0] != caseRecord.SourceSet.SourceSetDigest {
			return fmt.Errorf("ground-truth question set or derivation mismatch")
		}
	}
	return nil
}

func validateRequest(caseRecord CaseRecord, request RequestRecord, frozen FrozenIdentities) error {
	if err := validateEnvelope(request.RecordEnvelope); err != nil {
		return err
	}
	if request.ProtocolVersion != RequestProtocol || request.RequestID != strings.Replace(caseRecord.CaseID, "case", "request", 1) ||
		request.SourceSetDigest != caseRecord.SourceSet.SourceSetDigest ||
		!reflect.DeepEqual(request.Frozen, frozen) ||
		request.State.MediaType != "text/plain" ||
		request.State.Content != string(compiledContext(caseRecord.SourceSet)) ||
		request.State.SHA256 != compiledContextDigest(caseRecord.SourceSet) ||
		request.QuestionSchemaID != QuestionSchemaID || request.QuestionSchemaVersion != QuestionSchemaVersion ||
		request.QuestionSchemaDigest != frozen.QuestionSchemaDigest || len(request.Questions) != len(Questions) {
		return fmt.Errorf("request identity mismatch")
	}
	for index, question := range request.Questions {
		expected := Questions[index]
		if question.Field != expected.ID || question.Kind != "choice" || question.Question != expected.Text ||
			!reflect.DeepEqual(question.AllowedChoices, expected.Choices) ||
			question.AnswerContract.SelectedLabel != "exactly one allowed choice" ||
			!reflect.DeepEqual(question.AnswerContract.Probabilities, expected.Choices) ||
			question.AnswerContract.Confidence != "selected_label_probability" ||
			question.AnswerContract.SumTolerance != 1e-9 {
			return fmt.Errorf("question %d does not match frozen atomic contract", index)
		}
	}
	return nil
}

func validatePolicyRecord(caseRecord CaseRecord, policy PolicyRecord, frozen FrozenIdentities) error {
	if err := validateEnvelope(policy.RecordEnvelope); err != nil {
		return err
	}
	recomputed, err := EvaluatePolicy(policy.DecisionStatus, policy.Evidence)
	if err != nil {
		return err
	}
	if policy.PolicyID != PolicyID || policy.PolicyVersion != PolicyVersion ||
		policy.SourceSetDigest != caseRecord.SourceSet.SourceSetDigest ||
		!reflect.DeepEqual(policy.Frozen, frozen) ||
		policy.PolicyDigest != frozen.PolicyDigest || !reflect.DeepEqual(policy.Evidence, caseRecord.Evidence) ||
		!reflect.DeepEqual(policy.Result, recomputed) || policy.Result.Result != caseRecord.ExpectedBaselineAction {
		return fmt.Errorf("policy identity or recomputation mismatch")
	}
	return nil
}

func validateSchedule(caseRecord CaseRecord, request RequestRecord, schedule ScheduleRecord, index int, frozen FrozenIdentities) error {
	if err := validateEnvelope(schedule.RecordEnvelope); err != nil {
		return err
	}
	if schedule.ScheduledCallID != strings.Replace(caseRecord.CaseID, "case", "call", 1) ||
		schedule.ScheduleIndex != index || schedule.RecordedExecutionOrder != index ||
		schedule.RequestID != request.RequestID || schedule.CaseID != caseRecord.CaseID ||
		schedule.SourceSetDigest != caseRecord.SourceSet.SourceSetDigest ||
		!reflect.DeepEqual(schedule.Frozen, frozen) ||
		schedule.PerturbationDigest != caseRecord.Perturbation.TransformationReceiptDigest ||
		schedule.ContextArm != "raw_deterministic_concatenation" ||
		schedule.CompilerDigest != rawCompilerDigest() ||
		schedule.QuestionSchemaDigest != frozen.QuestionSchemaDigest ||
		schedule.DecisionSystemDigest != digestText(PolicyID+"\n"+PolicyVersion) ||
		schedule.PolicyDigest != frozen.PolicyDigest ||
		schedule.ThresholdSetDigest != digestText("context-build-artifact/not-applicable-threshold-set/v1") ||
		schedule.RepositoryRole != "pilot_development" || schedule.ReplicateIndex != 1 {
		return fmt.Errorf("schedule binding mismatch")
	}
	return nil
}

func evidenceValues(e Evidence) map[string]string {
	return map[string]string{
		"evidence_complete":         e.EvidenceComplete,
		"observed_tests_support":    e.ObservedTestsSupport,
		"verifier_support":          e.VerifierSupport,
		"cleanup_complete":          e.CleanupComplete,
		"external_effects_resolved": e.ExternalEffectsResolved,
		"patch_risk":                e.PatchRisk,
		"recommended_disposition":   e.RecommendedDisposition,
	}
}

func privacyScan(files map[string][]byte) error {
	needles := []string{
		"authorization: bearer", "private_key", "private key-----", "secret_access_key",
		"sk_live_", "ghp_", "xoxb-", "/users/", "/home/", `c:\users\`,
		"siddhant-git-ai",
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		lower := strings.ToLower(string(files[name]))
		for _, needle := range needles {
			if strings.Contains(lower, needle) {
				return fmt.Errorf("privacy scan rejected %q: matched prohibited credential pattern", name)
			}
		}
		if name == "receipt-schema.json" || name == "SHA256SUMS" {
			continue
		}
		if err := scanSensitiveJSONFields(name, files[name]); err != nil {
			return err
		}
	}
	return nil
}

func scanSensitiveJSONFields(name string, data []byte) error {
	for _, line := range bytes.Split(bytes.TrimSuffix(data, []byte{'\n'}), []byte{'\n'}) {
		var value any
		decoder := json.NewDecoder(bytes.NewReader(line))
		if err := decoder.Decode(&value); err != nil {
			return fmt.Errorf("privacy scan decode %q: %w", name, err)
		}
		if err := rejectSensitiveKeys(value); err != nil {
			return fmt.Errorf("privacy scan rejected %q: %w", name, err)
		}
	}
	return nil
}

func rejectSensitiveKeys(value any) error {
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			normalized := strings.ToLower(strings.ReplaceAll(key, "-", "_"))
			switch normalized {
			case "api_key", "access_token", "secret", "password", "environment",
				"environment_variables", "authorization_header":
				return fmt.Errorf("prohibited sensitive field %q", key)
			}
			if err := rejectSensitiveKeys(child); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := rejectSensitiveKeys(child); err != nil {
				return err
			}
		}
	}
	return nil
}

// Summarize validates before reporting; partial or tampered output is never summarized.
func Summarize(directory string) (string, error) {
	summary, err := Validate(directory)
	if err != nil {
		return "", err
	}
	lines := []string{
		"phase=pilot",
		"excluded=true",
		"final_study_eligible=false",
		"cases=" + strconv.Itoa(summary.CaseCount),
		"requests=" + strconv.Itoa(summary.RequestCount),
		"policy_accept=" + strconv.Itoa(summary.PolicyAccept),
		"policy_review=" + strconv.Itoa(summary.PolicyReview),
		"policy_reject=" + strconv.Itoa(summary.PolicyReject),
		"provider_calls=0",
		"dataset_snapshot_sha256=" + summary.DatasetSnapshotDigest,
		"pilot_manifest_sha256=" + summary.ManifestSHA256,
		"checksums_sha256=" + summary.ChecksumsSHA256,
	}
	return strings.Join(lines, "\n") + "\n", nil
}
