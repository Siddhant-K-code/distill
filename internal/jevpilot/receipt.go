package jevpilot

import (
	"encoding/json"
	"fmt"
	"math"
	"sort"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

type receiptInput struct {
	Case               studypilot.CaseRecord
	Request            studypilot.RequestRecord
	BaseSchedule       studypilot.ScheduleRecord
	ExecutionSchedule  ExecutionScheduleEntry
	Authorization      Authorization
	AuthorizationBytes []byte
	ProviderRecord     ProviderRecord
	ProviderBytes      []byte
	ScheduleBytes      []byte
	ReservationBytes   []byte
	Result             callResult
}

func buildReceipt(input receiptInput) (receiptMaterial, error) {
	artifacts := make(map[string][]byte)
	evidence := make([]any, 0, len(input.Case.SourceSet.Sources)+6)
	addEvidence := func(id, media string, data []byte) {
		artifacts[id] = data
		evidence = append(evidence, artifactIdentity(id, media, data))
	}
	for _, source := range input.Case.SourceSet.Sources {
		addEvidence(source.SourceID, source.MediaType, []byte(source.Content))
	}
	transform := map[string]any{
		"case_id": input.Case.CaseID, "perturbation_id": input.Case.Perturbation.PerturbationID,
		"type": input.Case.Perturbation.Type, "transform_version": input.Case.Perturbation.TransformVersion,
		"seed": input.Case.Perturbation.Seed, "expected_equivalence": input.Case.Perturbation.ExpectedEquivalence,
		"base_source_set_digest":   input.Case.Perturbation.BaseSourceSetDigest,
		"result_source_set_digest": input.Case.Perturbation.ResultSourceSetDigest,
		"operations":               input.Case.Perturbation.Operations,
	}
	transformBytes, err := canonicalJSON(transform)
	if err != nil {
		return receiptMaterial{}, err
	}
	addEvidence(input.Case.Perturbation.TransformationReceiptArtifactID, "application/json", transformBytes)
	contextID := input.Case.CaseID + "-compiled-context"
	addEvidence(contextID, "text/plain", []byte(input.Request.State.Content))
	requestID := input.ExecutionSchedule.ScheduledCallID + "-provider-request"
	addEvidence(requestID, "application/json", input.Result.RequestBody)
	addEvidence("execution-authorization", "application/json", input.AuthorizationBytes)
	addEvidence("provider-record", "application/json", input.ProviderBytes)
	addEvidence("execution-schedule", "application/x-ndjson", input.ScheduleBytes)
	if len(input.ReservationBytes) > 0 {
		addEvidence(input.ExecutionSchedule.ScheduledCallID+"-attempt-reservation", "application/json", input.ReservationBytes)
	}

	receiptArtifacts := make([]any, 0, 4)
	addReceiptArtifact := func(id, media string, data []byte) {
		artifacts[id] = data
		receiptArtifacts = append(receiptArtifacts, artifactIdentity(id, media, data))
	}
	if len(input.Result.RawBody) > 0 {
		addReceiptArtifact(input.ExecutionSchedule.ScheduledCallID+"-raw-response", "application/json", input.Result.RawBody)
	}
	if !input.Result.NotAttempted {
		metadataBytes, err := canonicalJSON(input.Result.Metadata)
		if err != nil {
			return receiptMaterial{}, err
		}
		addReceiptArtifact(input.ExecutionSchedule.ScheduledCallID+"-response-metadata", "application/json", metadataBytes)
	}

	outputs := []any{}
	var usage map[string]any
	var rawOutputDigest any
	var providerRequestID any
	var errorValue any
	status := "valid"
	runValidity := map[string]any{"status": "valid", "reason_codes": []any{}}
	if input.Result.ObservedUsage != nil {
		usage = map[string]any{
			"input_tokens":  input.Result.ObservedUsage.InputTokens,
			"output_tokens": input.Result.ObservedUsage.OutputTokens,
			"total_tokens":  input.Result.ObservedUsage.InputTokens + input.Result.ObservedUsage.OutputTokens,
		}
		usageBytes, encodeErr := canonicalJSON(usage)
		if encodeErr != nil {
			return receiptMaterial{}, encodeErr
		}
		usageID := input.ExecutionSchedule.ScheduledCallID + "-usage"
		addReceiptArtifact(usageID, "application/json", usageBytes)
		usage["provider_usage_json_digest"] = digest(usageBytes)
	}
	if input.Result.NotAttempted {
		status = "not_attempted"
		runValidity = map[string]any{"status": "invalid", "reason_codes": []any{"not_attempted"}}
		errorArtifact := map[string]any{
			"stage": "scheduler", "code": input.Result.ErrorCode,
			"message": input.Result.Err.Error(), "http_status": 0, "raw_response_sha256": nil,
		}
		errorBytes, encodeErr := canonicalJSON(errorArtifact)
		if encodeErr != nil {
			return receiptMaterial{}, encodeErr
		}
		errorID := input.ExecutionSchedule.ScheduledCallID + "-sanitized-error"
		addReceiptArtifact(errorID, "application/json", errorBytes)
		errorValue = map[string]any{
			"stage": "scheduler", "code": input.Result.ErrorCode, "retry_count": 0,
			"sanitized_error_artifact_id": errorID, "error_detail_digest": digest(errorBytes),
		}
	} else if input.Result.Err == nil {
		outputs = providerOutputs(*input.Result.Response, input.Request)
		rawOutputDigest = digest(input.Result.RawBody)
		providerRequestID = input.Result.Metadata.ProviderRequestID
	} else {
		status = "failed"
		runValidity = map[string]any{"status": "invalid", "reason_codes": []any{"decision_failed"}}
		errorArtifact := map[string]any{
			"stage": input.Result.ErrorStage, "code": input.Result.ErrorCode,
			"message": input.Result.Err.Error(), "http_status": input.Result.Metadata.HTTPStatus,
			"raw_response_sha256": nullableDigest(input.Result.RawBody),
		}
		errorBytes, encodeErr := canonicalJSON(errorArtifact)
		if encodeErr != nil {
			return receiptMaterial{}, encodeErr
		}
		errorID := input.ExecutionSchedule.ScheduledCallID + "-sanitized-error"
		addReceiptArtifact(errorID, "application/json", errorBytes)
		errorValue = map[string]any{
			"stage": input.Result.ErrorStage, "code": input.Result.ErrorCode, "retry_count": 0,
			"sanitized_error_artifact_id": errorID, "error_detail_digest": digest(errorBytes),
		}
		if input.Result.Metadata.ProviderRequestID != "" {
			providerRequestID = input.Result.Metadata.ProviderRequestID
		}
	}

	policyEvidence := evidenceFromOutputs(outputs, input.Case.Evidence)
	policyResult, err := studypilot.EvaluatePolicy(status, policyEvidence)
	if err != nil {
		return receiptMaterial{}, err
	}
	var acceptance any
	if status == "valid" {
		score, scoreErr := acceptanceScore(outputs, input.Case.Evidence)
		if scoreErr != nil {
			return receiptMaterial{}, scoreErr
		}
		acceptance = score
	}
	executionAuthorization := executionAuthorizationValue(input.Authorization, input.AuthorizationBytes, input.ProviderRecord)
	executionAuthorizationBytes, err := canonicalJSON(executionAuthorization)
	if err != nil {
		return receiptMaterial{}, err
	}
	questionIDs := make([]any, 0, len(input.Request.Questions))
	for _, question := range input.Request.Questions {
		questionIDs = append(questionIDs, question.Field)
	}
	modelSnapshotDigest := modelSnapshotDigest(input.ProviderRecord)
	parametersDigest := parametersDigest(input.Request)
	decisionSystemDigest := digest(input.ProviderBytes)
	validatorConfigDigest := digest([]byte("context-build-artifact/typesafe-jev-validator-config/v1"))
	semanticValidatorDigest := digest([]byte("context-build-artifact/semantic-validator/v1\n" + contextartifact.PilotSchemaSHA256))
	thresholdDigest := digest([]byte("context-build-artifact/not-applicable-threshold-set/v1"))
	acceptanceDigest := digest([]byte(studypilot.AcceptanceScoreVersion))

	attemptCount := 1
	var attemptStartedAt any = input.Result.StartedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	var attemptFinishedAt any = input.Result.FinishedAt.UTC().Format("2006-01-02T15:04:05.999999999Z07:00")
	var latency any = float64(input.Result.FinishedAt.Sub(input.Result.StartedAt).Microseconds()) / 1000
	if input.Result.NotAttempted {
		attemptCount = 0
		attemptStartedAt = nil
		attemptFinishedAt = nil
		latency = nil
	}
	receipt := map[string]any{
		"schema_version": "context-build-artifact/receipt/v1", "receipt_schema_digest": contextartifact.PilotSchemaSHA256,
		"study_phase": "pilot", "record_stage": "runner", "run_schedule_digest": digest(input.ScheduleBytes),
		"case": map[string]any{
			"case_id": input.Case.CaseID, "repository_url": input.Case.RepositoryURL,
			"repository_commit": input.Case.RepositoryCommit, "split": input.Case.Split,
			"repository_role": input.Case.RepositoryRole, "task_kind": input.Case.TaskKind,
			"dataset_snapshot_digest": input.Case.Frozen.DatasetSnapshotDigest,
			"split_assignment_digest": digest([]byte("context-build-artifact/synthetic-public-pilot/split/v1")),
			"case_manifest_digest":    digest(input.AuthorizationBytes),
		},
		"source_set": sourceSetValue(input.Case.SourceSet),
		"perturbation": map[string]any{
			"perturbation_id": input.Case.Perturbation.PerturbationID, "type": input.Case.Perturbation.Type,
			"transform_version": input.Case.Perturbation.TransformVersion, "seed": input.Case.Perturbation.Seed,
			"expected_equivalence":               input.Case.Perturbation.ExpectedEquivalence,
			"base_source_set_digest":             input.Case.Perturbation.BaseSourceSetDigest,
			"result_source_set_digest":           input.Case.Perturbation.ResultSourceSetDigest,
			"transformation_receipt_artifact_id": input.Case.Perturbation.TransformationReceiptArtifactID,
			"transformation_receipt_digest":      input.Case.Perturbation.TransformationReceiptDigest,
		},
		"ground_truth_commitment_digest": input.Case.LabelSealDigest, "ground_truth": nil,
		"custody": map[string]any{
			"labels_available_to_runner": false, "commitment_published_digest": input.Case.LabelSealDigest,
			"access_log_digest": digest([]byte("context-build-artifact/excluded-pilot/no-label-access/v1")),
			"unsealed_at":       nil, "reveal_nonce": nil,
		},
		"compiled_context": map[string]any{
			"arm":                     input.BaseSchedule.ContextArm,
			"compiler_version":        "context-build-artifact/raw-deterministic-concatenation/v1",
			"compiler_digest":         input.BaseSchedule.CompilerDigest,
			"input_source_set_digest": input.Case.SourceSet.SourceSetDigest,
			"context_byte_length":     len(input.Request.State.Content),
			"compiled_context_digest": input.Request.State.SHA256,
			"artifact_hashes":         []any{artifactIdentity(contextID, "text/plain", []byte(input.Request.State.Content))},
		},
		"question_schema": map[string]any{
			"schema_id": input.Request.QuestionSchemaID, "schema_version": input.Request.QuestionSchemaVersion,
			"question_schema_digest": input.Request.QuestionSchemaDigest, "question_ids": questionIDs,
		},
		"decision_system": map[string]any{
			"kind": "jev_pinned", "system_version": "context-build-artifact/typesafe-jev-adapter/v1",
			"system_digest": decisionSystemDigest,
			"model": map[string]any{
				"provider": ProviderName, "model_id": ModelID, "model_version": ModelVersion,
				"model_snapshot_digest": modelSnapshotDigest, "parameters_digest": parametersDigest,
				"confidence_semantics_version": "context-build-artifact/selected-label-probability/v1; typesafe raw confidence preserved",
			},
			"api_version": APIVersion,
			"sdk": map[string]any{
				"name": "go-standard-library-net-http", "version": input.Authorization.ExecutionRuntime,
				"package_digest": digest([]byte("go-standard-library/net/http/" + input.Authorization.ExecutionRuntime)),
			},
		},
		"execution_authorization": executionAuthorization,
		"decision": map[string]any{
			"status": status, "outputs": outputs, "raw_output_digest": rawOutputDigest, "error": errorValue,
		},
		"policy": map[string]any{
			"policy_id": studypilot.PolicyID, "policy_version": studypilot.PolicyVersion,
			"policy_digest": input.Case.Frozen.PolicyDigest, "threshold_set_digest": thresholdDigest,
			"acceptance_score": acceptance, "acceptance_score_definition_digest": acceptanceDigest,
			"result": policyResult.Result, "reason_codes": stringSliceAny(policyResult.ReasonCodes),
		},
		"run_validity": runValidity,
		"validator": map[string]any{
			"json_schema_validator":         "github.com/santhosh-tekuri/jsonschema/v6",
			"json_schema_validator_version": "v6.0.3", "format_assertion_enabled": true,
			"validator_config_digest":    validatorConfigDigest,
			"semantic_validator_version": "context-build-artifact/semantic-validator/v1",
			"semantic_validator_digest":  semanticValidatorDigest,
		},
		"measurement": map[string]any{
			"scheduled_call_id": input.ExecutionSchedule.ScheduledCallID,
			"schedule_index":    input.ExecutionSchedule.ScheduleIndex,
			"replicate_index":   input.ExecutionSchedule.ReplicateIndex,
			"attempt_count":     attemptCount, "retry_count": 0, "rate_limit_waits": []any{},
			"attempt_started_at": attemptStartedAt, "attempt_finished_at": attemptFinishedAt,
			"latency_ms": latency,
			"usage":      usage, "provider_reported_cost": nil, "provider_request_id": providerRequestID,
		},
		"evidence_hashes": evidence, "receipt_artifact_hashes": receiptArtifacts,
		"identity_digest": "", "outcome_digest": "", "receipt_digest": "",
	}
	registry := studypilot.ReceiptRegistry{
		CaseID: input.Case.CaseID, RepositoryURL: input.Case.RepositoryURL,
		RepositoryCommit: input.Case.RepositoryCommit, DatasetSnapshotDigest: input.Case.Frozen.DatasetSnapshotDigest,
		SplitAssignmentDigest: digest([]byte("context-build-artifact/synthetic-public-pilot/split/v1")),
		CaseManifestDigest:    digest(input.AuthorizationBytes), GroundTruthCommitmentDigest: input.Case.LabelSealDigest,
		AccessLogDigest: digest([]byte("context-build-artifact/excluded-pilot/no-label-access/v1")),
		ScheduledCallID: input.ExecutionSchedule.ScheduledCallID, ScheduleIndex: input.ExecutionSchedule.ScheduleIndex,
		ReplicateIndex: input.ExecutionSchedule.ReplicateIndex, ContextArm: input.BaseSchedule.ContextArm,
		RepositoryRole: input.Case.RepositoryRole, RunScheduleDigest: digest(input.ScheduleBytes),
		SourceSetDigest:      input.Case.SourceSet.SourceSetDigest,
		BaseSourceSetDigests: map[string]bool{input.Case.Perturbation.BaseSourceSetDigest: true},
		TransformationDigest: input.Case.Perturbation.TransformationReceiptDigest,
		CompilerDigest:       input.BaseSchedule.CompilerDigest, CompiledContextDigest: input.Request.State.SHA256,
		QuestionSchemaDigest: input.Request.QuestionSchemaDigest, DecisionSystemDigest: decisionSystemDigest,
		DecisionSystemVersion: "context-build-artifact/typesafe-jev-adapter/v1",
		Provider:              ProviderName, ModelID: ModelID, ModelVersion: ModelVersion,
		ModelSnapshotDigest: modelSnapshotDigest, ParametersDigest: parametersDigest,
		APIVersion: APIVersion, SDKName: "go-standard-library-net-http", SDKVersion: input.Authorization.ExecutionRuntime,
		SDKPackageDigest:             digest([]byte("go-standard-library/net/http/" + input.Authorization.ExecutionRuntime)),
		ConfidenceSemanticsVersion:   "context-build-artifact/selected-label-probability/v1; typesafe raw confidence preserved",
		ExecutionAuthorizationDigest: digest(executionAuthorizationBytes),
		PolicyID:                     studypilot.PolicyID, PolicyVersion: studypilot.PolicyVersion,
		PolicyDigest: input.Case.Frozen.PolicyDigest, ThresholdSetDigest: thresholdDigest,
		AcceptanceDefinitionDigest: acceptanceDigest, ValidatorConfigDigest: validatorConfigDigest,
		SemanticValidatorDigest: semanticValidatorDigest, Contradiction: input.Case.Evidence.Contradiction,
		Stale: input.Case.Evidence.Stale, Applicability: input.Case.Evidence.Applicability,
		Artifacts: artifacts,
	}
	registry.ParseProviderArtifacts = providerArtifactParser(
		input.ExecutionSchedule.ScheduledCallID+"-raw-response",
		input.ExecutionSchedule.ScheduledCallID+"-response-metadata",
		input.Request,
	)
	if err := studypilot.SealReceipt(receipt); err != nil {
		return receiptMaterial{}, err
	}
	receiptBytes, err := canonicalJSON(receipt)
	if err != nil {
		return receiptMaterial{}, err
	}
	if err := studypilot.ValidateReceipt(receiptBytes, registry); err != nil {
		return receiptMaterial{}, fmt.Errorf("validate built receipt: %w", err)
	}
	return receiptMaterial{Receipt: receiptBytes, Registry: registry, Artifacts: artifacts}, nil
}

func providerArtifactParser(rawID, metadataID string, request studypilot.RequestRecord) func(map[string][]byte) (studypilot.ProviderObservation, error) {
	return func(artifacts map[string][]byte) (studypilot.ProviderObservation, error) {
		raw, ok := artifacts[rawID]
		if !ok {
			return studypilot.ProviderObservation{}, fmt.Errorf("missing raw response artifact")
		}
		response, err := parseAPIResponse(raw, request)
		if err != nil {
			return studypilot.ProviderObservation{}, err
		}
		var metadata ResponseMetadata
		if err := json.Unmarshal(artifacts[metadataID], &metadata); err != nil {
			return studypilot.ProviderObservation{}, err
		}
		if metadata.HTTPStatus != 200 || metadata.ProviderRequestID == "" {
			return studypilot.ProviderObservation{}, fmt.Errorf("provider metadata is not a successful identified response")
		}
		return studypilot.ProviderObservation{
			Outputs: providerOutputs(response, request),
			Usage: map[string]any{
				"input_tokens": response.Usage.InputTokens, "output_tokens": response.Usage.OutputTokens,
				"total_tokens": response.Usage.InputTokens + response.Usage.OutputTokens,
			},
			Cost: nil, ProviderRequestID: metadata.ProviderRequestID,
		}, nil
	}
}

func providerOutputs(response APIResponse, request studypilot.RequestRecord) []any {
	outputs := make([]any, 0, len(request.Questions))
	for _, question := range request.Questions {
		answer := response.Answers[question.Field]
		probabilities := make(map[string]any, len(answer.Probabilities))
		for key, value := range answer.Probabilities {
			probabilities[key] = value
		}
		outputs = append(outputs, map[string]any{
			"question_id": question.Field, "selected_label": answer.Choice,
			"probabilities": probabilities, "confidence": answer.Probabilities[answer.Choice],
			"confidence_semantics": "selected_label_probability",
		})
	}
	return outputs
}

func sourceSetValue(sourceSet studypilot.SourceSet) map[string]any {
	sources := make([]any, 0, len(sourceSet.Sources))
	for _, source := range sourceSet.Sources {
		sources = append(sources, map[string]any{
			"source_id": source.SourceID, "relative_path": source.RelativePath,
			"media_type": source.MediaType, "byte_length": source.ByteLength,
			"content_digest": source.ContentDigest, "normalized_content_digest": source.NormalizedContentDigest,
			"evidence_role": source.EvidenceRole,
		})
	}
	return map[string]any{
		"source_set_id":  sourceSet.SourceSetID,
		"manifest_order": stringSliceAny(sourceSet.ManifestOrder),
		"sources":        sources, "source_set_digest": sourceSet.SourceSetDigest,
	}
}

func evidenceFromOutputs(outputs []any, frozen studypilot.Evidence) studypilot.Evidence {
	if len(outputs) == 0 {
		return studypilot.Evidence{}
	}
	values := make(map[string]string, len(outputs))
	for _, raw := range outputs {
		output := raw.(map[string]any)
		values[output["question_id"].(string)] = output["selected_label"].(string)
	}
	return studypilot.Evidence{
		EvidenceComplete: values["evidence_complete"], ObservedTestsSupport: values["observed_tests_support"],
		VerifierSupport: values["verifier_support"], CleanupComplete: values["cleanup_complete"],
		ExternalEffectsResolved: values["external_effects_resolved"], PatchRisk: values["patch_risk"],
		RecommendedDisposition: values["recommended_disposition"], Contradiction: frozen.Contradiction,
		Stale: frozen.Stale, Applicability: frozen.Applicability,
	}
}

func acceptanceScore(outputs []any, frozen studypilot.Evidence) (float64, error) {
	if frozen.Contradiction || frozen.Stale {
		return 0, nil
	}
	safe := map[string][]string{
		"evidence_complete": {"yes"}, "observed_tests_support": safeLabels(frozen.Applicability.ObservedTests),
		"verifier_support":          safeLabels(frozen.Applicability.Verifier),
		"cleanup_complete":          safeLabels(frozen.Applicability.Cleanup),
		"external_effects_resolved": safeLabels(frozen.Applicability.ExternalEffects),
		"patch_risk":                {"low"}, "recommended_disposition": {"accept"},
	}
	score := 1.0
	for _, raw := range outputs {
		output := raw.(map[string]any)
		probabilities := output["probabilities"].(map[string]any)
		total := 0.0
		for _, label := range safe[output["question_id"].(string)] {
			value, ok := probabilities[label].(float64)
			if !ok || math.IsNaN(value) || math.IsInf(value, 0) {
				return 0, fmt.Errorf("missing safe-label probability")
			}
			total += value
		}
		score = math.Min(score, total)
	}
	return score, nil
}

func safeLabels(applicable bool) []string {
	if applicable {
		return []string{"yes"}
	}
	return []string{"not_applicable"}
}

func artifactIdentity(id, media string, data []byte) map[string]any {
	return map[string]any{"artifact_id": id, "media_type": media, "byte_length": len(data), "sha256": digest(data)}
}

func nullableDigest(data []byte) any {
	if len(data) == 0 {
		return nil
	}
	return digest(data)
}

func stringSliceAny(values []string) []any {
	result := make([]any, len(values))
	for index, value := range values {
		result[index] = value
	}
	return result
}

func executionAuthorizationValue(authorization Authorization, authorizationBytes []byte, provider ProviderRecord) map[string]any {
	return map[string]any{
		"stage":                           "excluded_provider_pilot",
		"authorization_record_digest":     digest(authorizationBytes),
		"publication_independence_digest": digest([]byte(provider.ExternalGates.PublicationIndependence)),
		"credential_policy_digest":        digest([]byte("macOS Keychain ai.typesafe.api; TYPESAFE_API_KEY injected only into one child process; no headers persisted")),
		"authorized_budget": map[string]any{
			"amount": 5.0, "currency": "USD", "pricing_version": PricingVersion,
		},
		"provider_spend_cap": map[string]any{
			"amount": 5.0, "currency": "USD", "pricing_version": PricingVersion + "/local-fail-closed",
		},
	}
}

func modelSnapshotDigest(provider ProviderRecord) string {
	value, _ := canonicalJSON(map[string]any{
		"model_id": provider.ModelID, "model_version": provider.ModelVersion,
		"model_list_response_sha256": provider.ModelListResponseSHA256,
		"models_document_sha256":     "9d20bb3c90a0147532d0b20ddc4c64579391be7842e965543b27bad684eeb4d6",
	})
	return digest(value)
}

func parametersDigest(request studypilot.RequestRecord) string {
	questions := make([]map[string]any, 0, len(request.Questions))
	for _, question := range request.Questions {
		questions = append(questions, map[string]any{
			"field": question.Field, "kind": question.Kind, "question": question.Question,
			"allowed_choices": question.AllowedChoices,
		})
	}
	sort.Slice(questions, func(i, j int) bool { return questions[i]["field"].(string) < questions[j]["field"].(string) })
	value, _ := canonicalJSON(map[string]any{
		"model": ModelID, "temperature": nil, "seed": nil, "retries": 0,
		"questions": questions,
	})
	return digest(value)
}
