package studypilot

import (
	"encoding/hex"
	"encoding/json"
	"math"
	"testing"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

func TestValidateReceiptValidSynthetic(t *testing.T) {
	receipt, registry := validReceiptFixture(t)
	data, err := canonicalJSON(receipt)
	if err != nil {
		t.Fatal(err)
	}

	if err := ValidateReceipt(data, registry); err != nil {
		t.Fatal(err)
	}
}

func TestStructuredDigestKnownAnswers(t *testing.T) {
	tests := []struct {
		domain string
		value  map[string]any
		want   string
	}{
		{"condition", map[string]any{"a": "NFC", "z": 1}, "8a4cf865911e197bdd0e84893e746844c4affd7d10df25216dab7a26afc3906b"},
		{"outcome", map[string]any{"identity_digest": strings64("0"), "result": "review"}, "4b791030da3696eed600d0dfa282dabb9a9184394ba68a5467fc0b271633dfef"},
		{"receipt", map[string]any{"attempt_count": 1, "timestamp": "2026-09-20T00:00:00Z"}, "56f80512bdbd5f9fb99d7fc55c17134752ea1c624f7ebf399885178e7dbcbf0d"},
	}
	for _, test := range tests {
		got, err := structuredDigest(test.domain, test.value)
		if err != nil {
			t.Fatal(err)
		}
		if got != test.want {
			t.Fatalf("%s digest = %s, want %s", test.domain, got, test.want)
		}
	}
}

func TestValidateReceiptAdversarialOutputs(t *testing.T) {
	tests := map[string]func(map[string]any, *ReceiptRegistry){
		"moving alias": func(receipt map[string]any, registry *ReceiptRegistry) {
			mapAt(receipt, "decision_system", "model")["model_id"] = "jev-latest"
			registry.ModelID = "jev-latest"
		},
		"stable alias": func(receipt map[string]any, registry *ReceiptRegistry) {
			mapAt(receipt, "decision_system", "model")["model_version"] = "stable"
			registry.ModelVersion = "stable"
		},
		"preview alias": func(receipt map[string]any, registry *ReceiptRegistry) {
			mapAt(receipt, "decision_system", "model")["model_id"] = "jev/preview"
			registry.ModelID = "jev/preview"
		},
		"newest alias": func(receipt map[string]any, registry *ReceiptRegistry) {
			mapAt(receipt, "decision_system", "model")["model_id"] = "jev.newest"
			registry.ModelID = "jev.newest"
		},
		"missing probability": func(receipt map[string]any, _ *ReceiptRegistry) {
			delete(sliceAt(receipt, "decision", "outputs")[0].(map[string]any)["probabilities"].(map[string]any), "unknown")
		},
		"confidence mismatch": func(receipt map[string]any, _ *ReceiptRegistry) {
			sliceAt(receipt, "decision", "outputs")[0].(map[string]any)["confidence"] = 0.8
		},
		"probability sum": func(receipt map[string]any, _ *ReceiptRegistry) {
			sliceAt(receipt, "decision", "outputs")[0].(map[string]any)["probabilities"].(map[string]any)["yes"] = 0.8
		},
		"probability type": func(receipt map[string]any, _ *ReceiptRegistry) {
			sliceAt(receipt, "decision", "outputs")[0].(map[string]any)["probabilities"].(map[string]any)["yes"] = "0.9"
		},
		"probability out of range": func(receipt map[string]any, _ *ReceiptRegistry) {
			probabilities := sliceAt(receipt, "decision", "outputs")[0].(map[string]any)["probabilities"].(map[string]any)
			probabilities["yes"] = 1.1
			probabilities["no"] = -0.1
		},
		"missing usage": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "measurement")["usage"] = nil
		},
		"missing request ID": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "measurement")["provider_request_id"] = nil
		},
		"schedule index mismatch": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "measurement")["schedule_index"] = 1
		},
		"replicate mismatch": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "measurement")["replicate_index"] = 2
		},
		"provider mismatch": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "decision_system", "model")["provider"] = "different-provider"
		},
		"system version mismatch": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "decision_system")["system_version"] = "v2"
		},
		"confidence semantics version mismatch": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "decision_system", "model")["confidence_semantics_version"] = "v2"
		},
		"not applicable on applicable field": func(receipt map[string]any, _ *ReceiptRegistry) {
			output := sliceAt(receipt, "decision", "outputs")[1].(map[string]any)
			output["selected_label"] = "not_applicable"
			output["confidence"] = 0.05
		},
		"invalid URI format": func(receipt map[string]any, registry *ReceiptRegistry) {
			mapAt(receipt, "case")["repository_url"] = "not a URI"
			registry.RepositoryURL = "not a URI"
		},
		"invalid timestamp format": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "measurement")["attempt_started_at"] = "20 September"
		},
		"wrong policy": func(receipt map[string]any, _ *ReceiptRegistry) {
			mapAt(receipt, "policy")["result"] = "review"
		},
		"structured raw output mismatch": func(receipt map[string]any, _ *ReceiptRegistry) {
			output := sliceAt(receipt, "decision", "outputs")[0].(map[string]any)
			output["selected_label"] = "no"
			output["confidence"] = 0.05
		},
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			receipt, registry := validReceiptFixture(t)
			mutate(receipt, &registry)
			if err := SealReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			data, err := canonicalJSON(receipt)
			if err != nil {
				t.Fatal(err)
			}
			if err := ValidateReceipt(data, registry); err == nil {
				t.Fatal("adversarial receipt unexpectedly validated")
			}
		})
	}
}

func TestValidateReceiptRejectsNoncanonicalTransformationArtifact(t *testing.T) {
	receipt, registry := validReceiptFixture(t)
	transform := registry.Artifacts["pilot-transform"]
	transform = append([]byte(`{"case_id":"ambiguous",`), transform[1:]...)
	registry.Artifacts["pilot-transform"] = transform
	for _, raw := range sliceAt(receipt, "evidence_hashes") {
		entry := raw.(map[string]any)
		if stringAt(entry, "artifact_id") == "pilot-transform" {
			entry["byte_length"] = len(transform)
			entry["sha256"] = digestBytes(transform)
		}
	}
	if err := SealReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	data, err := canonicalJSON(receipt)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceipt(data, registry); err == nil {
		t.Fatal("noncanonical transformation artifact unexpectedly validated")
	}
}

func TestValidateReceiptStageCollectionBijection(t *testing.T) {
	receipt, registry := validReceiptFixture(t)
	data, err := canonicalJSON(receipt)
	if err != nil {
		t.Fatal(err)
	}
	registries := map[string]ReceiptRegistry{registry.ScheduledCallID: registry}
	if err := ValidateReceiptStageCollection([][]byte{data}, registries, "runner"); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptStageCollection(nil, registries, "runner"); err == nil {
		t.Fatal("missing receipt unexpectedly validated")
	}
	if err := ValidateReceiptStageCollection([][]byte{data, data}, registries, "runner"); err == nil {
		t.Fatal("duplicate receipt unexpectedly validated")
	}
}

func TestValidateReceiptCollectionPairsRunnerAndAnalysis(t *testing.T) {
	runner, registry := validReceiptFixture(t)
	answers := make([]any, 0, len(Questions))
	for _, outputRaw := range sliceAt(runner, "decision", "outputs") {
		output := outputRaw.(map[string]any)
		answers = append(answers, map[string]any{
			"question_id": stringAt(output, "question_id"),
			"label":       stringAt(output, "selected_label"),
			"supporting_evidence_digests": []any{
				registry.SourceSetDigest,
			},
			"disagreement_code": nil,
		})
	}
	truth := map[string]any{
		"rubric_version": "synthetic-rubric/v1", "rubric_digest": digestText("synthetic-rubric"),
		"adjudication_status": "agreed", "answers": answers,
		"adjudication_receipt_digest": digestText("synthetic-adjudication"),
	}
	nonceText := strings64("1")
	nonce, err := hex.DecodeString(nonceText)
	if err != nil {
		t.Fatal(err)
	}
	truthBytes, err := canonicalJSON(truth)
	if err != nil {
		t.Fatal(err)
	}
	preimage := append([]byte("context-build-artifact/ground-truth-commitment/v1\n"), nonce...)
	preimage = append(preimage, truthBytes...)
	commitment := digestBytes(preimage)
	runner["ground_truth_commitment_digest"] = commitment
	mapAt(runner, "custody")["commitment_published_digest"] = commitment
	registry.GroundTruthCommitmentDigest = commitment
	if err := SealReceipt(runner); err != nil {
		t.Fatal(err)
	}

	analysis := cloneMap(t, runner)
	analysis["record_stage"] = "analysis"
	analysis["ground_truth"] = truth
	mapAt(analysis, "custody")["unsealed_at"] = "2026-09-20T01:00:00Z"
	mapAt(analysis, "custody")["reveal_nonce"] = nonceText
	if err := SealReceipt(analysis); err != nil {
		t.Fatal(err)
	}
	runnerData, _ := canonicalJSON(runner)
	analysisData, _ := canonicalJSON(analysis)
	registries := map[string]ReceiptRegistry{registry.ScheduledCallID: registry}
	if err := ValidateReceiptCollection([][]byte{runnerData}, [][]byte{analysisData}, registries); err != nil {
		t.Fatal(err)
	}
	mapAt(analysis, "case")["case_id"] = "pilot-case-receipt-other"
	if err := SealReceipt(analysis); err != nil {
		t.Fatal(err)
	}
	analysisData, _ = canonicalJSON(analysis)
	if err := ValidateReceiptCollection([][]byte{runnerData}, [][]byte{analysisData}, registries); err == nil {
		t.Fatal("mismatched runner/analysis pair unexpectedly validated")
	}
}

func TestValidateReceiptRejectsNonFiniteAndMissingRegistry(t *testing.T) {
	receipt, registry := validReceiptFixture(t)
	data, _ := canonicalJSON(receipt)
	if err := ValidateReceipt(data, ReceiptRegistry{}); err == nil {
		t.Fatal("missing external registry unexpectedly accepted")
	}

	output := sliceAt(receipt, "decision", "outputs")[0].(map[string]any)
	output["confidence"] = math.Inf(1)
	if _, err := canonicalJSON(receipt); err == nil {
		t.Fatal("non-finite receipt unexpectedly canonicalized")
	}
	_ = registry
}

func TestValidateReceiptFailedAndNotAttemptedTerminals(t *testing.T) {
	for _, status := range []string{"failed", "not_attempted"} {
		t.Run(status, func(t *testing.T) {
			receipt, registry := validReceiptFixture(t)
			errorBytes := []byte("sanitized synthetic failure")
			errorID := "pilot-error-" + status
			registry.Artifacts[errorID] = errorBytes
			decision := mapAt(receipt, "decision")
			decision["status"] = status
			decision["outputs"] = []any{}
			decision["raw_output_digest"] = nil
			stage := "provider"
			reason := "decision_failed"
			if status == "not_attempted" {
				stage = "scheduler"
				reason = "not_attempted"
				measurement := mapAt(receipt, "measurement")
				measurement["attempt_count"] = 0
				measurement["attempt_started_at"] = nil
				measurement["attempt_finished_at"] = nil
				measurement["latency_ms"] = nil
				measurement["usage"] = nil
				measurement["provider_reported_cost"] = nil
				measurement["provider_request_id"] = nil
			}
			decision["error"] = map[string]any{
				"stage": stage, "code": "synthetic_failure", "retry_count": 0,
				"sanitized_error_artifact_id": errorID, "error_detail_digest": digestBytes(errorBytes),
			}
			policy := mapAt(receipt, "policy")
			policy["acceptance_score"] = nil
			policy["result"] = "review"
			policy["reason_codes"] = []any{reason}
			receipt["receipt_artifact_hashes"] = append(sliceAt(receipt, "receipt_artifact_hashes"),
				map[string]any{"artifact_id": errorID, "media_type": "text/plain", "byte_length": len(errorBytes), "sha256": digestBytes(errorBytes)})
			if err := SealReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			data, _ := canonicalJSON(receipt)
			if err := ValidateReceipt(data, registry); err != nil {
				t.Fatal(err)
			}
			delete(mapAt(receipt, "decision"), "error")
			if err := SealReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			data, _ = canonicalJSON(receipt)
			if err := ValidateReceipt(data, registry); err == nil {
				t.Fatal("terminal receipt without error unexpectedly validated")
			}
		})
	}
}

func TestReceiptTimestampChangesOnlyReceiptDigest(t *testing.T) {
	first, registry := validReceiptFixture(t)
	firstIdentity := stringAt(first, "identity_digest")
	firstOutcome := stringAt(first, "outcome_digest")
	firstReceipt := stringAt(first, "receipt_digest")

	second := cloneMap(t, first)
	measurement := mapAt(second, "measurement")
	measurement["attempt_started_at"] = "2026-09-20T00:00:01Z"
	measurement["attempt_finished_at"] = "2026-09-20T00:00:01.010Z"
	if err := SealReceipt(second); err != nil {
		t.Fatal(err)
	}
	if stringAt(second, "identity_digest") != firstIdentity || stringAt(second, "outcome_digest") != firstOutcome {
		t.Fatal("timestamps changed condition or outcome identity")
	}
	if stringAt(second, "receipt_digest") == firstReceipt {
		t.Fatal("timestamps did not change full receipt digest")
	}
	data, _ := canonicalJSON(second)
	if err := ValidateReceipt(data, registry); err != nil {
		t.Fatal(err)
	}
}

func validReceiptFixture(t *testing.T) (map[string]any, ReceiptRegistry) {
	t.Helper()
	sha := func(value string) string { return digestText(value) }
	sourceA := []byte("Synthetic handoff evidence.\n")
	sourceB := []byte("Verifier pass.\n")
	sourceObjects := []any{
		map[string]any{
			"source_id": "pilot-source-a", "relative_path": "evidence/a.txt", "media_type": "text/plain",
			"byte_length": len(sourceA), "content_digest": digestBytes(sourceA),
			"normalized_content_digest": digestBytes(sourceA), "evidence_role": "required",
		},
		map[string]any{
			"source_id": "pilot-source-b", "relative_path": "evidence/b.txt", "media_type": "text/plain",
			"byte_length": len(sourceB), "content_digest": digestBytes(sourceB),
			"normalized_content_digest": digestBytes(sourceB), "evidence_role": "supporting",
		},
	}
	sourceSet := map[string]any{
		"source_set_id":  "pilot-source-set-001",
		"manifest_order": []any{"pilot-source-a", "pilot-source-b"},
		"sources":        sourceObjects,
	}
	sourceProjection := []map[string]any{
		copyKeys(sourceObjects[0].(map[string]any), "source_id", "relative_path", "media_type", "byte_length", "content_digest", "normalized_content_digest", "evidence_role"),
		copyKeys(sourceObjects[1].(map[string]any), "source_id", "relative_path", "media_type", "byte_length", "content_digest", "normalized_content_digest", "evidence_role"),
	}
	sourceDigest, err := structuredDigest("source-set", map[string]any{
		"source_set_id": sourceSet["source_set_id"], "manifest_order": sourceSet["manifest_order"], "sources": sourceProjection,
	})
	if err != nil {
		t.Fatal(err)
	}
	sourceSet["source_set_digest"] = sourceDigest
	transform := map[string]any{
		"case_id": "pilot-case-receipt-001", "perturbation_id": "pilot-perturbation-none",
		"type": "none", "transform_version": TransformVersion, "seed": 1,
		"expected_equivalence": "equivalent", "base_source_set_digest": sourceDigest,
		"result_source_set_digest": sourceDigest,
		"operations":               []any{map[string]any{"kind": "none", "target": "source-set", "detail": "unperturbed baseline"}},
	}
	transformBytes, _ := canonicalJSON(transform)
	transformDigest, _ := structuredDigest("transformation", transform)
	contextBytes := []byte("compiled synthetic context")
	artifact := func(id, media string, data []byte) map[string]any {
		return map[string]any{"artifact_id": id, "media_type": media, "byte_length": len(data), "sha256": digestBytes(data)}
	}
	outputs := make([]any, 0, len(Questions))
	for _, question := range Questions {
		selected := question.Choices[0]
		probabilities := make(map[string]any, len(question.Choices))
		switch len(question.Choices) {
		case 3:
			probabilities[question.Choices[0]] = 0.9
			probabilities[question.Choices[1]] = 0.05
			probabilities[question.Choices[2]] = 0.05
		case 4:
			probabilities[question.Choices[0]] = 0.85
			probabilities[question.Choices[1]] = 0.05
			probabilities[question.Choices[2]] = 0.05
			probabilities[question.Choices[3]] = 0.05
		}
		outputs = append(outputs, map[string]any{
			"question_id": question.ID, "selected_label": selected, "probabilities": probabilities,
			"confidence": probabilities[selected], "confidence_semantics": "selected_label_probability",
		})
	}
	cost := map[string]any{"amount": 0.01, "currency": "USD", "pricing_version": "synthetic-v1"}
	rawOutput, err := canonicalJSON(map[string]any{
		"outputs":                outputs,
		"provider_request_id":    "synthetic-request-001",
		"provider_reported_cost": cost,
	})
	if err != nil {
		t.Fatal(err)
	}
	usageValue := map[string]any{"input_tokens": 10, "output_tokens": 7, "total_tokens": 17}
	usageBytes, err := canonicalJSON(usageValue)
	if err != nil {
		t.Fatal(err)
	}
	questionBytes, err := canonicalJSON(Questions)
	if err != nil {
		t.Fatal(err)
	}
	questionDigest := digestBytes(questionBytes)
	expectedPolicyDigest, err := policyDigest()
	if err != nil {
		t.Fatal(err)
	}
	thresholdDigest := digestText("context-build-artifact/not-applicable-threshold-set/v1")
	acceptanceDigest := digestText(AcceptanceScoreVersion)
	validatorConfig := sha("validator-config")
	semanticValidator := sha("semantic-validator-source")
	repositoryURL := "https://example.org/public/synthetic"
	repositoryCommit := "0000000000000000000000000000000000000001"
	datasetDigest := sha("dataset")
	splitDigest := sha("split")
	caseManifestDigest := sha("case-manifest")
	groundTruthCommitment := sha("sealed-ground-truth")
	accessLogDigest := sha("access-log")
	compilerDigest := sha("compiler")
	registry := ReceiptRegistry{
		CaseID: "pilot-case-receipt-001", ScheduledCallID: "pilot-call-receipt-001",
		ScheduleIndex: 0, ReplicateIndex: 1, ContextArm: "raw_deterministic_concatenation",
		RepositoryRole: "pilot_development",
		RepositoryURL:  repositoryURL, RepositoryCommit: repositoryCommit,
		DatasetSnapshotDigest: datasetDigest, SplitAssignmentDigest: splitDigest,
		CaseManifestDigest: caseManifestDigest, GroundTruthCommitmentDigest: groundTruthCommitment,
		AccessLogDigest: accessLogDigest, SourceSetDigest: sourceDigest,
		BaseSourceSetDigests: map[string]bool{sourceDigest: true},
		TransformationDigest: transformDigest, CompilerDigest: compilerDigest,
		CompiledContextDigest: digestBytes(contextBytes),
		RunScheduleDigest:     sha("schedule"), QuestionSchemaDigest: questionDigest,
		DecisionSystemDigest: sha("decision-system"), DecisionSystemVersion: "v1",
		Provider: "synthetic-offline-provider", ModelID: "jev-2026-09-immutable",
		ModelVersion: "2026-09-01", ModelSnapshotDigest: sha("model-snapshot"),
		ParametersDigest: sha("parameters"), APIVersion: "2026-09-01",
		SDKName: "synthetic-offline-sdk", SDKVersion: "1.0.0", SDKPackageDigest: sha("sdk"),
		ConfidenceSemanticsVersion: "v1",
		PolicyID:                   PolicyID, PolicyVersion: PolicyVersion, PolicyDigest: expectedPolicyDigest,
		ThresholdSetDigest: thresholdDigest, AcceptanceDefinitionDigest: acceptanceDigest,
		ValidatorConfigDigest: validatorConfig, SemanticValidatorDigest: semanticValidator,
		Applicability: Applicability{ObservedTests: true, Verifier: true, Cleanup: true, ExternalEffects: true},
		Artifacts: map[string][]byte{
			"pilot-source-a": sourceA, "pilot-source-b": sourceB,
			"pilot-transform": transformBytes, "pilot-context": contextBytes,
			"pilot-raw-output": rawOutput, "pilot-usage": usageBytes,
		},
	}
	registry.ParseProviderArtifacts = func(artifacts map[string][]byte) (ProviderObservation, error) {
		rawValue, err := decodeCanonicalValue(artifacts["pilot-raw-output"])
		if err != nil {
			return ProviderObservation{}, err
		}
		raw := rawValue.(map[string]any)
		usageRaw, err := decodeCanonicalValue(artifacts["pilot-usage"])
		if err != nil {
			return ProviderObservation{}, err
		}
		return ProviderObservation{
			Outputs:           raw["outputs"].([]any),
			Usage:             usageRaw.(map[string]any),
			Cost:              raw["provider_reported_cost"].(map[string]any),
			ProviderRequestID: raw["provider_request_id"].(string),
		}, nil
	}
	receipt := map[string]any{
		"schema_version":        "context-build-artifact/receipt/v1",
		"receipt_schema_digest": contextartifact.PilotSchemaSHA256,
		"study_phase":           "pilot", "record_stage": "runner", "run_schedule_digest": registry.RunScheduleDigest,
		"case": map[string]any{
			"case_id": registry.CaseID, "repository_url": repositoryURL,
			"repository_commit": repositoryCommit, "split": "pilot",
			"repository_role": "pilot_development", "task_kind": "evidence_handoff",
			"dataset_snapshot_digest": datasetDigest, "split_assignment_digest": splitDigest,
			"case_manifest_digest": caseManifestDigest,
		},
		"source_set": sourceSet,
		"perturbation": map[string]any{
			"perturbation_id": "pilot-perturbation-none", "type": "none",
			"transform_version": TransformVersion, "seed": 1, "expected_equivalence": "equivalent",
			"base_source_set_digest": sourceDigest, "result_source_set_digest": sourceDigest,
			"transformation_receipt_artifact_id": "pilot-transform",
			"transformation_receipt_digest":      transformDigest,
		},
		"ground_truth_commitment_digest": groundTruthCommitment,
		"ground_truth":                   nil,
		"custody": map[string]any{
			"labels_available_to_runner": false, "commitment_published_digest": groundTruthCommitment,
			"access_log_digest": accessLogDigest, "unsealed_at": nil, "reveal_nonce": nil,
		},
		"compiled_context": map[string]any{
			"arm": "raw_deterministic_concatenation", "compiler_version": "context-build-artifact/raw-deterministic-concatenation/v1",
			"compiler_digest": compilerDigest, "input_source_set_digest": sourceDigest,
			"context_byte_length": len(contextBytes), "compiled_context_digest": digestBytes(contextBytes),
			"artifact_hashes": []any{artifact("pilot-context", "text/plain", contextBytes)},
		},
		"question_schema": map[string]any{
			"schema_id": QuestionSchemaID, "schema_version": QuestionSchemaVersion,
			"question_schema_digest": questionDigest, "question_ids": toInterfaceStrings(questionIDList()),
		},
		"decision_system": map[string]any{
			"kind": "jev_pinned", "system_version": "v1", "system_digest": registry.DecisionSystemDigest,
			"model": map[string]any{
				"provider": "synthetic-offline-provider", "model_id": registry.ModelID,
				"model_version": registry.ModelVersion, "model_snapshot_digest": registry.ModelSnapshotDigest,
				"parameters_digest": registry.ParametersDigest, "confidence_semantics_version": "v1",
			},
			"api_version": registry.APIVersion,
			"sdk":         map[string]any{"name": registry.SDKName, "version": registry.SDKVersion, "package_digest": registry.SDKPackageDigest},
		},
		"execution_authorization": map[string]any{
			"stage": "excluded_provider_pilot", "authorization_record_digest": sha("authorization"),
			"publication_independence_digest": sha("publication"), "credential_policy_digest": sha("credential-policy"),
			"authorized_budget":  map[string]any{"amount": 1.0, "currency": "USD", "pricing_version": "synthetic-v1"},
			"provider_spend_cap": map[string]any{"amount": 1.0, "currency": "USD", "pricing_version": "synthetic-v1"},
		},
		"decision": map[string]any{
			"status": "valid", "outputs": outputs, "raw_output_digest": digestBytes(rawOutput), "error": nil,
		},
		"policy": map[string]any{
			"policy_id": PolicyID, "policy_version": PolicyVersion, "policy_digest": expectedPolicyDigest,
			"threshold_set_digest": thresholdDigest, "acceptance_score": 0.85,
			"acceptance_score_definition_digest": acceptanceDigest,
			"result":                             "accept", "reason_codes": []any{"all_required_evidence_safe"},
		},
		"run_validity": map[string]any{"status": "valid", "reason_codes": []any{}},
		"validator": map[string]any{
			"json_schema_validator":         "github.com/santhosh-tekuri/jsonschema/v6",
			"json_schema_validator_version": "v6.0.3", "format_assertion_enabled": true,
			"validator_config_digest":    validatorConfig,
			"semantic_validator_version": "context-build-artifact/semantic-validator/v1",
			"semantic_validator_digest":  semanticValidator,
		},
		"measurement": map[string]any{
			"scheduled_call_id": registry.ScheduledCallID, "schedule_index": 0, "replicate_index": 1,
			"attempt_count": 1, "retry_count": 0, "rate_limit_waits": []any{},
			"attempt_started_at": "2026-09-20T00:00:00Z", "attempt_finished_at": "2026-09-20T00:00:00.010Z",
			"latency_ms":             10.0,
			"usage":                  map[string]any{"input_tokens": 10, "output_tokens": 7, "total_tokens": 17, "provider_usage_json_digest": digestBytes(usageBytes)},
			"provider_reported_cost": cost,
			"provider_request_id":    "synthetic-request-001",
		},
		"evidence_hashes": []any{
			artifact("pilot-context", "text/plain", contextBytes),
			artifact("pilot-transform", "application/json", transformBytes),
		},
		"receipt_artifact_hashes": []any{
			artifact("pilot-raw-output", "application/json", rawOutput),
			artifact("pilot-usage", "application/json", usageBytes),
		},
		"identity_digest": "", "outcome_digest": "", "receipt_digest": "",
	}
	authorizationBytes, err := canonicalJSON(receipt["execution_authorization"])
	if err != nil {
		t.Fatal(err)
	}
	registry.ExecutionAuthorizationDigest = digestBytes(authorizationBytes)
	if err := SealReceipt(receipt); err != nil {
		t.Fatal(err)
	}
	return receipt, registry
}

func cloneMap(t *testing.T, source map[string]any) map[string]any {
	t.Helper()
	data, err := json.Marshal(source)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(data, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func toInterfaceStrings(values []string) []any {
	result := make([]any, len(values))
	for index := range values {
		result[index] = values[index]
	}

	return result
}

func strings64(value string) string {
	result := ""
	for len(result) < 64 {
		result += value
	}
	return result[:64]
}
