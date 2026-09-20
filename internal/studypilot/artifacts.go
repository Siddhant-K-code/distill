package studypilot

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

func envelope() RecordEnvelope {
	return RecordEnvelope{
		SchemaVersion:      ArtifactSchemaVersion,
		Phase:              "pilot",
		Excluded:           true,
		FinalStudyEligible: false,
	}
}

// BuildArtifacts returns the complete artifact set before atomic publication.
// A supplied case order is accepted to test generation-order independence.
func BuildArtifacts(caseOrder []string) (map[string][]byte, error) {
	definitions := corpusDefinitions()
	if len(caseOrder) > 0 {
		if len(caseOrder) != len(definitions) {
			return nil, fmt.Errorf("case order has %d IDs, want %d", len(caseOrder), len(definitions))
		}
		byID := make(map[string]caseDefinition, len(definitions))
		for _, definition := range definitions {
			byID[definition.ID] = definition
		}
		reordered := make([]caseDefinition, 0, len(definitions))
		for _, id := range caseOrder {
			definition, ok := byID[id]
			if !ok {
				return nil, fmt.Errorf("case order contains unknown or duplicate ID %q", id)
			}
			delete(byID, id)
			reordered = append(reordered, definition)
		}
		if len(byID) != 0 {
			return nil, fmt.Errorf("case order omits IDs")
		}
		definitions = reordered
	}
	sort.Slice(definitions, func(i, j int) bool { return definitions[i].ID < definitions[j].ID })

	frozen, err := frozenIdentities()
	if err != nil {
		return nil, err
	}
	cases := make([]CaseRecord, 0, len(definitions))
	labels := make([]LabelRecord, 0, len(definitions))
	requests := make([]RequestRecord, 0, len(definitions))
	schedules := make([]ScheduleRecord, 0, len(definitions))
	policies := make([]PolicyRecord, 0, len(definitions))
	compilerDigest := rawCompilerDigest()
	decisionSystemDigest := digestText(PolicyID + "\n" + PolicyVersion)
	thresholdSetDigest := digestText("context-build-artifact/not-applicable-threshold-set/v1")

	for index, definition := range definitions {
		base, result, operations, err := transformSourceSet(definition)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", definition.ID, err)
		}
		perturbation := Perturbation{
			PerturbationID:                  definition.ID + "-perturbation",
			Type:                            definition.Perturbation,
			TransformVersion:                TransformVersion,
			Seed:                            caseSeed(definition.ID),
			ExpectedEquivalence:             equivalenceFor(definition.Perturbation),
			BaseSourceSetDigest:             base.SourceSetDigest,
			ResultSourceSetDigest:           result.SourceSetDigest,
			TransformationReceiptArtifactID: definition.ID + "-transformation-receipt",
			Operations:                      operations,
		}
		perturbation.TransformationReceiptDigest, err = transformationDigest(definition.ID, perturbation)
		if err != nil {
			return nil, err
		}
		truth := groundTruthFor(definition, result)
		labelSealDigest, err := labelSeal(definition.ID, truth)
		if err != nil {
			return nil, err
		}
		policyResult, err := EvaluatePolicy("valid", definition.Evidence)
		if err != nil {
			return nil, fmt.Errorf("%s policy: %w", definition.ID, err)
		}
		context := compiledContext(result)
		contextDigest := digestBytes(context)
		cases = append(cases, CaseRecord{
			RecordEnvelope:         envelope(),
			CaseID:                 definition.ID,
			Category:               definition.Category,
			RepositoryURL:          "https://example.org/public-domain/synthetic-fixtures",
			RepositoryCommit:       fmt.Sprintf("%040x", index+1),
			RepositoryRole:         "pilot_development",
			Split:                  "pilot",
			TaskKind:               taskKindFor(definition.Category),
			Frozen:                 frozen,
			BaseSourceSet:          base,
			SourceSet:              result,
			Perturbation:           perturbation,
			Evidence:               definition.Evidence,
			LabelSealDigest:        labelSealDigest,
			ExpectedBaselineAction: policyResult.Result,
		})
		labels = append(labels, LabelRecord{
			RecordEnvelope:     envelope(),
			CaseID:             definition.ID,
			SourceSetDigest:    result.SourceSetDigest,
			Frozen:             frozen,
			LabelSealDigest:    labelSealDigest,
			SealedFromRequests: true,
			GroundTruth:        truth,
		})
		requestQuestions := make([]RequestQuestion, 0, len(Questions))
		for _, question := range Questions {
			requestQuestions = append(requestQuestions, RequestQuestion{
				Field:          question.ID,
				Kind:           "choice",
				Question:       question.Text,
				AllowedChoices: append([]string(nil), question.Choices...),
				AnswerContract: AnswerContract{
					SelectedLabel: "exactly one allowed choice",
					Probabilities: append([]string(nil), question.Choices...),
					Confidence:    "selected_label_probability",
					SumTolerance:  1e-9,
				},
			})
		}
		requestID := strings.Replace(definition.ID, "case", "request", 1)
		requests = append(requests, RequestRecord{
			RecordEnvelope:  envelope(),
			ProtocolVersion: RequestProtocol,
			RequestID:       requestID,
			CaseID:          definition.ID,
			SourceSetDigest: result.SourceSetDigest,
			Frozen:          frozen,
			State: RequestState{
				MediaType: "text/plain",
				Content:   string(context),
				SHA256:    contextDigest,
			},
			QuestionSchemaID:      QuestionSchemaID,
			QuestionSchemaVersion: QuestionSchemaVersion,
			QuestionSchemaDigest:  frozen.QuestionSchemaDigest,
			Questions:             requestQuestions,
		})
		schedules = append(schedules, ScheduleRecord{
			RecordEnvelope:         envelope(),
			ScheduledCallID:        strings.Replace(definition.ID, "case", "call", 1),
			ScheduleIndex:          index,
			RecordedExecutionOrder: index,
			RequestID:              requestID,
			CaseID:                 definition.ID,
			SourceSetDigest:        result.SourceSetDigest,
			Frozen:                 frozen,
			PerturbationDigest:     perturbation.TransformationReceiptDigest,
			ContextArm:             "raw_deterministic_concatenation",
			CompilerDigest:         compilerDigest,
			QuestionSchemaDigest:   frozen.QuestionSchemaDigest,
			DecisionSystemDigest:   decisionSystemDigest,
			PolicyDigest:           frozen.PolicyDigest,
			ThresholdSetDigest:     thresholdSetDigest,
			RepositoryRole:         "pilot_development",
			ReplicateIndex:         1,
		})
		policies = append(policies, PolicyRecord{
			RecordEnvelope:  envelope(),
			CaseID:          definition.ID,
			SourceSetDigest: result.SourceSetDigest,
			Frozen:          frozen,
			DecisionStatus:  "valid",
			PolicyID:        PolicyID,
			PolicyVersion:   PolicyVersion,
			PolicyDigest:    frozen.PolicyDigest,
			Evidence:        definition.Evidence,
			Result:          policyResult,
		})
	}

	files := make(map[string][]byte)
	if files["cases.jsonl"], err = canonicalJSONL(toAny(cases)); err != nil {
		return nil, err
	}
	if files["labels.jsonl"], err = canonicalJSONL(toAny(labels)); err != nil {
		return nil, err
	}
	if files["requests.jsonl"], err = canonicalJSONL(toAny(requests)); err != nil {
		return nil, err
	}
	if files["policy-baseline.jsonl"], err = canonicalJSONL(toAny(policies)); err != nil {
		return nil, err
	}
	if files["schedule.jsonl"], err = canonicalJSONL(toAny(schedules)); err != nil {
		return nil, err
	}
	files["receipt-schema.json"] = append([]byte(nil), contextartifact.PilotSchema...)

	payloadNames := []string{"cases.jsonl", "labels.jsonl", "policy-baseline.jsonl", "receipt-schema.json", "requests.jsonl", "schedule.jsonl"}
	manifestFiles := make([]FileIdentity, 0, len(payloadNames))
	for _, name := range payloadNames {
		manifestFiles = append(manifestFiles, FileIdentity{Path: name, Bytes: len(files[name]), SHA256: digestBytes(files[name])})
	}
	manifest := Manifest{
		RecordEnvelope:            envelope(),
		ArtifactSetID:             "context-build-artifact/excluded-offline-pilot/v1",
		ProtocolClarification:     "research/context-is-a-build-artifact/pilot-protocol-v1.md",
		CaseCount:                 len(cases),
		ProviderCalls:             0,
		LabelsAvailableToRequests: false,
		Frozen:                    frozen,
		Files:                     manifestFiles,
	}
	files["pilot-manifest.json"], err = canonicalJSONFile(manifest)
	if err != nil {
		return nil, err
	}
	files["SHA256SUMS"] = renderChecksums(files)
	return files, nil
}

func taskKindFor(category string) string {
	switch category {
	case "missing_cleanup_evidence":
		return "cleanup_verification"
	case "unresolved_external_effects":
		return "external_effect_verification"
	case "verifier_failure", "fully_complete_verified":
		return "patch_verification"
	default:
		return "evidence_handoff"
	}
}

func renderChecksums(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "SHA256SUMS" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var output bytes.Buffer
	for _, name := range names {
		fmt.Fprintf(&output, "%s  %d  %s\n", digestBytes(files[name]), len(files[name]), name)
	}
	return output.Bytes()
}

// Prepare atomically publishes a validated artifact set to a nonexistent directory.
func Prepare(outputDirectory string) (Summary, error) {
	files, err := BuildArtifacts(nil)
	if err != nil {
		return Summary{}, err
	}
	absolute, err := filepath.Abs(outputDirectory)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve output: %w", err)
	}
	if filepath.Base(absolute) == "." || filepath.Base(absolute) == string(filepath.Separator) {
		return Summary{}, fmt.Errorf("unsafe output directory")
	}
	if _, err := os.Lstat(absolute); err == nil {
		return Summary{}, fmt.Errorf("output directory already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Summary{}, fmt.Errorf("inspect output: %w", err)
	}
	parent := filepath.Dir(absolute)
	if err := ensureSafeDirectory(parent); err != nil {
		return Summary{}, fmt.Errorf("prepare output parent: %w", err)
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil {
		return Summary{}, fmt.Errorf("inspect output parent: %w", err)
	}
	if parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return Summary{}, fmt.Errorf("output parent must be a real directory")
	}
	if parentInfo.Mode().Perm()&0o022 != 0 && parentInfo.Mode()&os.ModeSticky == 0 {
		return Summary{}, fmt.Errorf("output parent is group/world writable without sticky bit")
	}

	staging, err := os.MkdirTemp(parent, "."+filepath.Base(absolute)+".staging-")
	if err != nil {
		return Summary{}, fmt.Errorf("create staging directory: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(staging)
		}
	}()
	if err := os.Chmod(staging, 0o755); err != nil {
		return Summary{}, err
	}
	for _, name := range artifactFileNames {
		if err := writeSyncedFile(filepath.Join(staging, name), files[name]); err != nil {
			return Summary{}, err
		}
	}
	if err := syncDirectory(staging); err != nil {
		return Summary{}, err
	}
	summary, err := Validate(staging)
	if err != nil {
		return Summary{}, fmt.Errorf("validate staging output: %w", err)
	}
	if err := syncDirectory(parent); err != nil {
		return Summary{}, fmt.Errorf("sync parent before publication: %w", err)
	}
	if err := renameNoReplace(staging, absolute); err != nil {
		return Summary{}, fmt.Errorf("publish output directory: %w", err)
	}
	published = true
	if err := syncDirectory(parent); err != nil {
		return Summary{}, fmt.Errorf("published but durability unconfirmed for %q: %w", absolute, err)
	}
	return summary, nil
}

func toAny[T any](records []T) []any {
	values := make([]any, len(records))
	for index := range records {
		values[index] = records[index]
	}
	return values
}

func writeSyncedFile(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		return fmt.Errorf("create %q: %w", filepath.Base(path), err)
	}
	ok := false
	defer func() {
		_ = file.Close()
		if !ok {
			_ = os.Remove(path)
		}
	}()
	if _, err := file.Write(data); err != nil {
		return fmt.Errorf("write %q: %w", filepath.Base(path), err)
	}
	if err := file.Sync(); err != nil {
		return fmt.Errorf("sync %q: %w", filepath.Base(path), err)
	}
	if err := file.Close(); err != nil {
		return fmt.Errorf("close %q: %w", filepath.Base(path), err)
	}
	ok = true
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	defer func() {
		_ = directory.Close()
	}()
	return directory.Sync()
}
