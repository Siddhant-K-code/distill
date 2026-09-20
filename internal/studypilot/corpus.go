package studypilot

import (
	"bytes"
	"crypto/sha256"
	"fmt"
	"math/rand"
	"slices"
	"sort"
	"strings"

	"golang.org/x/text/unicode/norm"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

type caseDefinition struct {
	ID           string   `json:"case_id"`
	Category     string   `json:"category"`
	Perturbation string   `json:"perturbation"`
	Variant      string   `json:"variant"`
	Evidence     Evidence `json:"evidence"`
	Reasons      []string `json:"derivation_reasons"`
}

func safeEvidence() Evidence {
	return Evidence{
		EvidenceComplete:        "yes",
		ObservedTestsSupport:    "yes",
		VerifierSupport:         "yes",
		CleanupComplete:         "yes",
		ExternalEffectsResolved: "yes",
		PatchRisk:               "low",
		RecommendedDisposition:  "accept",
		Applicability: Applicability{
			ObservedTests: true, Verifier: true, Cleanup: true, ExternalEffects: true,
		},
	}
}

func corpusDefinitions() []caseDefinition {
	safe := safeEvidence()
	definitions := []caseDefinition{
		{ID: "pilot-case-001", Category: "source_reorder", Perturbation: "source_reorder", Evidence: safe, Reasons: []string{"Manifest order changed while paths and bytes remained fixed."}},
		{ID: "pilot-case-002", Category: "exact_duplicate_insertion", Perturbation: "exact_duplicate_insertion", Evidence: safe, Reasons: []string{"An exact duplicate was added at a fresh path without changing evidence meaning."}},
		{ID: "pilot-case-003", Category: "line_endings_lf", Perturbation: "line_ending_change", Variant: "lf", Evidence: safe, Reasons: []string{"Line separators use LF and preserve the registered facts."}},
		{ID: "pilot-case-004", Category: "line_endings_crlf", Perturbation: "line_ending_change", Variant: "crlf", Evidence: safe, Reasons: []string{"Line separators use CRLF and normalize to the same evidence."}},
		{ID: "pilot-case-005", Category: "line_endings_cr", Perturbation: "line_ending_change", Variant: "cr", Evidence: safe, Reasons: []string{"Line separators use CR and normalize to the same evidence."}},
		{ID: "pilot-case-006", Category: "metadata_noise", Perturbation: "metadata_noise", Evidence: with(safe, func(e *Evidence) {
			e.CleanupComplete = "not_applicable"
			e.ExternalEffectsResolved = "not_applicable"
			e.Applicability.Cleanup = false
			e.Applicability.ExternalEffects = false
		}), Reasons: []string{"Non-semantic metadata was added outside the registered evidence.", "The registered task has no cleanup obligation or external effect."}},
		{ID: "pilot-case-007", Category: "identical_content_path_rename", Perturbation: "path_rename_identical_content", Evidence: safe, Reasons: []string{"A path changed while its content and evidence role remained fixed."}},
		{ID: "pilot-case-008", Category: "irrelevant_append", Perturbation: "irrelevant_appended_text", Evidence: safe, Reasons: []string{"The appended public-domain-style control text is unrelated to every question."}},
		{ID: "pilot-case-009", Category: "one_relevant_fact_change", Perturbation: "one_relevant_fact_change", Evidence: with(safe, func(e *Evidence) { e.PatchRisk = "medium"; e.RecommendedDisposition = "review" }), Reasons: []string{"One registered risk fact changed from low to medium."}},
		{ID: "pilot-case-010", Category: "missing_observed_test_evidence", Perturbation: "missing_required_evidence", Evidence: with(safe, func(e *Evidence) {
			e.EvidenceComplete = "no"
			e.ObservedTestsSupport = "unknown"
			e.RecommendedDisposition = "review"
		}), Reasons: []string{"The observed test artifact required by the rubric is absent."}},
		{ID: "pilot-case-011", Category: "claimed_tests_not_observed", Perturbation: "one_relevant_fact_change", Evidence: with(safe, func(e *Evidence) { e.ObservedTestsSupport = "no"; e.RecommendedDisposition = "reject" }), Reasons: []string{"A prose claim says tests pass, but the preserved execution record says they were not observed."}},
		{ID: "pilot-case-012", Category: "verifier_failure", Perturbation: "one_relevant_fact_change", Evidence: with(safe, func(e *Evidence) { e.VerifierSupport = "no"; e.RecommendedDisposition = "reject" }), Reasons: []string{"The registered verifier completed with a failing outcome."}},
		{ID: "pilot-case-013", Category: "stale_baseline", Perturbation: "stale_baseline_or_evidence", Evidence: with(safe, func(e *Evidence) { e.Stale = true; e.RecommendedDisposition = "review" }), Reasons: []string{"The evidence baseline predates an incompatible lifecycle state.", "Staleness is independently derived policy evidence, not an eighth provider question."}},
		{ID: "pilot-case-014", Category: "missing_cleanup_evidence", Perturbation: "missing_required_evidence", Evidence: with(safe, func(e *Evidence) {
			e.EvidenceComplete = "no"
			e.CleanupComplete = "unknown"
			e.RecommendedDisposition = "review"
		}), Reasons: []string{"A registered cleanup obligation has no completion evidence."}},
		{ID: "pilot-case-015", Category: "unresolved_external_effects", Perturbation: "one_relevant_fact_change", Evidence: with(safe, func(e *Evidence) { e.ExternalEffectsResolved = "no"; e.RecommendedDisposition = "reject" }), Reasons: []string{"A registered external effect remains unresolved."}},
		{ID: "pilot-case-016", Category: "contradictory_summary_execution", Perturbation: "contradictory_evidence", Evidence: with(safe, func(e *Evidence) { e.Contradiction = true; e.RecommendedDisposition = "reject" }), Reasons: []string{"The summary conflicts directly with the preserved execution record.", "Contradiction is independently derived policy evidence, not an eighth provider question."}},
		{ID: "pilot-case-017", Category: "fully_complete_verified", Perturbation: "none", Evidence: safe, Reasons: []string{"All required evidence is present, current, mutually consistent, verified, and low risk."}},
		{ID: "pilot-case-018", Category: "generic_missing_required_evidence", Perturbation: "missing_required_evidence", Evidence: with(safe, func(e *Evidence) { e.EvidenceComplete = "unknown"; e.RecommendedDisposition = "review" }), Reasons: []string{"A generic required rubric item is unavailable, so completeness is unknown."}},
	}
	return definitions
}

func with(base Evidence, mutate func(*Evidence)) Evidence {
	mutate(&base)
	return base
}

func frozenIdentities() (FrozenIdentities, error) {
	config := ConfigIdentity{
		SchemaVersion:   "distill-lock/config/v0",
		ChunkBytes:      1024,
		TokenBudget:     8192,
		MetadataRemoval: "none",
	}
	configValue := struct {
		SchemaVersion   string `json:"schema_version"`
		ChunkBytes      int    `json:"chunk_bytes"`
		TokenBudget     int    `json:"token_budget"`
		MetadataRemoval string `json:"metadata_removal"`
	}{config.SchemaVersion, config.ChunkBytes, config.TokenBudget, config.MetadataRemoval}
	configBytes, err := canonicalJSON(configValue)
	if err != nil {
		return FrozenIdentities{}, err
	}
	config.Digest = digestBytes(configBytes)

	questionBytes, err := canonicalJSON(Questions)
	if err != nil {
		return FrozenIdentities{}, err
	}
	policyDigest, err := policyDigest()
	if err != nil {
		return FrozenIdentities{}, err
	}
	snapshotDigest, err := datasetSnapshotDigest()
	if err != nil {
		return FrozenIdentities{}, err
	}
	return FrozenIdentities{
		DatasetSnapshotDigest: snapshotDigest,
		Config:                config,
		DistillLock: DistillLockIdentity{
			Tool:                  LockToolIdentity,
			Runtime:               LockRuntimeIdentity,
			MergeCommit:           LockMergeCommit,
			Canonicalization:      "utf8-nfc-lf-v1",
			Chunking:              "utf8-fixed-bytes-v1",
			TokenEstimation:       "utf8-bytes-div4-ceil-v1",
			ExactDeduplication:    "sha256-normalized-exact-v1",
			Selection:             "lexical-budget-v1",
			BundleRendering:       "markdown-bundle-v1",
			CanonicalJSON:         "go-struct-json-indent-v1",
			ContextBundleSHA256:   "3096d7b4492124df4e893d27b15a9d59ffe8a9fad80d24cd8265262c2251866a",
			ContextLockSHA256:     "a4d44357e284f359c970af70aea744346703ecd37e8058bf7b8d3bd30f11842b",
			ContextManifestSHA256: "7f6849eb7f22a1de00e851bfd42ccef32dea1d82751bca9d7c592286ec6929fd",
			ChecksumsSHA256:       "ad4d593b7ded2a53023a24e767351b21570839f0b4fea9348d8e600252b8b0ee",
		},
		QuestionSchemaDigest: digestBytes(questionBytes),
		PolicyDigest:         policyDigest,
		ReceiptSchemaDigest:  contextartifact.PilotSchemaSHA256,
	}, nil
}

func datasetSnapshotDigest() (string, error) {
	return structuredDigest("dataset-snapshot", map[string]any{
		"dataset_snapshot_id": DatasetSnapshotID,
		"transform_version":   TransformVersion,
		"cases":               corpusDefinitions(),
	})
}

func policyDigest() (string, error) {
	table := []map[string]string{
		{"result": "accept", "rule": "only_complete_current_consistent_low_risk_accept_recommendation"},
		{"result": "invalid", "rule": "not_applicable_only_when_pre_execution_applicability_is_false"},
		{"result": "reject", "rule": "observed_tests_no_or_verifier_no_or_external_effects_no_or_contradiction_or_recommended_reject_or_high_risk"},
		{"result": "review", "rule": "unknown_incomplete_stale_cleanup_missing_medium_or_unknown_risk_or_recommended_review"},
		{"result": "review", "rule": "failed_or_not_attempted_decision"},
	}
	value := map[string]any{"policy_id": PolicyID, "policy_version": PolicyVersion, "rules": table}
	data, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	return digestBytes(data), nil
}

func normalizeText(value string) string {
	value = norm.NFC.String(value)
	value = strings.ReplaceAll(value, "\r\n", "\n")
	return strings.ReplaceAll(value, "\r", "\n")
}

func makeSource(id, path, role, content string) Source {
	normalized := normalizeText(content)
	return Source{
		SourceID:                id,
		RelativePath:            path,
		MediaType:               "text/plain",
		ByteLength:              len([]byte(content)),
		ContentDigest:           digestText(content),
		NormalizedContentDigest: digestText(normalized),
		EvidenceRole:            role,
		Content:                 content,
	}
}

func sourceSetDigest(set SourceSet) (string, error) {
	sources := append([]Source(nil), set.Sources...)
	sort.Slice(sources, func(i, j int) bool { return sources[i].SourceID < sources[j].SourceID })
	projected := make([]map[string]any, 0, len(sources))
	for _, source := range sources {
		projected = append(projected, map[string]any{
			"source_id": source.SourceID, "relative_path": source.RelativePath,
			"media_type": source.MediaType, "byte_length": source.ByteLength,
			"content_digest": source.ContentDigest, "normalized_content_digest": source.NormalizedContentDigest,
			"evidence_role": source.EvidenceRole,
		})
	}
	return structuredDigest("source-set", map[string]any{
		"source_set_id": set.SourceSetID, "manifest_order": set.ManifestOrder, "sources": projected,
	})
}

func transformSourceSet(def caseDefinition) (SourceSet, SourceSet, []Operation, error) {
	cleanup := "Cleanup: complete.\n"
	if !def.Evidence.Applicability.Cleanup {
		cleanup = "Cleanup: not applicable; no obligation exists.\n"
	}
	external := "External effects: resolved.\n"
	if !def.Evidence.Applicability.ExternalEffects {
		external = "External effects: not applicable; none exist.\n"
	}
	base := SourceSet{
		SourceSetID:   def.ID + "-base",
		ManifestOrder: []string{def.ID + "-summary", def.ID + "-execution"},
		Sources: []Source{
			makeSource(def.ID+"-summary", "evidence/summary.txt", "required", "Synthetic public pilot handoff.\nTests: observed pass.\nRisk: low.\n"),
			makeSource(def.ID+"-execution", "evidence/execution.txt", "supporting", "Verifier: pass.\n"+cleanup+external),
		},
	}
	var err error
	base.SourceSetDigest, err = sourceSetDigest(base)
	if err != nil {
		return SourceSet{}, SourceSet{}, nil, err
	}
	result := base
	result.SourceSetID = def.ID + "-result"
	result.ManifestOrder = append([]string(nil), base.ManifestOrder...)
	result.Sources = append([]Source(nil), base.Sources...)
	operations := []Operation{{Kind: def.Perturbation, Target: "source-set", Detail: "deterministic synthetic pilot transformation"}}

	switch def.Perturbation {
	case "none":
		result.SourceSetID = base.SourceSetID
		operations = []Operation{{Kind: "none", Target: "source-set", Detail: "unperturbed baseline"}}
	case "source_reorder":
		random := rand.New(rand.NewSource(int64(caseSeed(def.ID))))
		random.Shuffle(len(result.ManifestOrder), func(i, j int) {
			result.ManifestOrder[i], result.ManifestOrder[j] = result.ManifestOrder[j], result.ManifestOrder[i]
		})
		if slices.Equal(result.ManifestOrder, base.ManifestOrder) {
			result.ManifestOrder = append(result.ManifestOrder[1:], result.ManifestOrder[0])
		}
		operations[0].Detail = fmt.Sprintf("seeded Fisher-Yates manifest permutation with seed %d", caseSeed(def.ID))
	case "exact_duplicate_insertion":
		duplicate := result.Sources[0]
		duplicate.SourceID = def.ID + "-duplicate"
		duplicate.RelativePath = "evidence/summary-copy.txt"
		result.Sources = append(result.Sources, duplicate)
		result.ManifestOrder = append(result.ManifestOrder, duplicate.SourceID)
	case "line_ending_change":
		normalized := normalizeText(result.Sources[0].Content)
		switch def.Variant {
		case "lf":
			result.Sources[0] = makeSource(result.Sources[0].SourceID, result.Sources[0].RelativePath, result.Sources[0].EvidenceRole, normalized)
		case "crlf":
			result.Sources[0] = makeSource(result.Sources[0].SourceID, result.Sources[0].RelativePath, result.Sources[0].EvidenceRole, strings.ReplaceAll(normalized, "\n", "\r\n"))
		case "cr":
			result.Sources[0] = makeSource(result.Sources[0].SourceID, result.Sources[0].RelativePath, result.Sources[0].EvidenceRole, strings.ReplaceAll(normalized, "\n", "\r"))
		default:
			return SourceSet{}, SourceSet{}, nil, fmt.Errorf("unknown line-ending variant %q", def.Variant)
		}
		operations[0].Detail = "replace line separators with " + def.Variant
	case "metadata_noise":
		meta := makeSource(def.ID+"-metadata", "metadata/note.txt", "metadata", "Synthetic fixture note: catalog order 17.\n")
		result.Sources = append(result.Sources, meta)
		result.ManifestOrder = append(result.ManifestOrder, meta.SourceID)
	case "path_rename_identical_content":
		result.Sources[0].RelativePath = "evidence/renamed-summary.txt"
	case "irrelevant_appended_text":
		source := result.Sources[0]
		source.Content += "Irrelevant control: a square has four equal sides.\n"
		result.Sources[0] = makeSource(source.SourceID, source.RelativePath, source.EvidenceRole, source.Content)
		operations[0].Target = source.RelativePath
		operations[0].Detail = "append registered irrelevant control text"
	case "one_relevant_fact_change":
		switch def.Category {
		case "claimed_tests_not_observed":
			result.Sources[0] = makeSource(result.Sources[0].SourceID, result.Sources[0].RelativePath, "required", "Synthetic public pilot handoff.\nClaim: tests pass.\n")
			result.Sources[1] = makeSource(result.Sources[1].SourceID, result.Sources[1].RelativePath, "supporting", "Test execution: not observed.\nVerifier: pass.\nCleanup: complete.\nExternal effects: resolved.\n")
		case "verifier_failure":
			result.Sources[1] = makeSource(result.Sources[1].SourceID, result.Sources[1].RelativePath, "required", "Verifier: fail.\nCleanup: complete.\nExternal effects: resolved.\n")
		case "unresolved_external_effects":
			result.Sources[1] = makeSource(result.Sources[1].SourceID, result.Sources[1].RelativePath, "required", "Verifier: pass.\nCleanup: complete.\nExternal effects: unresolved.\n")
		default:
			result.Sources[0] = makeSource(result.Sources[0].SourceID, result.Sources[0].RelativePath, "required", "Synthetic public pilot handoff.\nTests: observed pass.\nRisk: medium.\n")
		}
	case "missing_required_evidence":
		if def.Category == "missing_cleanup_evidence" {
			result.Sources[1] = makeSource(result.Sources[1].SourceID, result.Sources[1].RelativePath, "required", "Verifier: pass.\nExternal effects: resolved.\nCleanup evidence: absent.\n")
		} else {
			result.Sources = result.Sources[1:]
			result.ManifestOrder = result.ManifestOrder[1:]
		}
	case "contradictory_evidence":
		conflict := makeSource(def.ID+"-contradiction", "evidence/conflict.txt", "contradictory", "Execution record: tests did not pass.\n")
		result.Sources = append(result.Sources, conflict)
		result.ManifestOrder = append(result.ManifestOrder, conflict.SourceID)
	case "stale_baseline_or_evidence":
		stale := makeSource(def.ID+"-lifecycle", "evidence/lifecycle.txt", "required", "Lifecycle baseline superseded after evidence capture.\n")
		result.Sources = append(result.Sources, stale)
		result.ManifestOrder = append(result.ManifestOrder, stale.SourceID)
	default:
		return SourceSet{}, SourceSet{}, nil, fmt.Errorf("unknown perturbation %q", def.Perturbation)
	}
	result.SourceSetDigest, err = sourceSetDigest(result)
	return base, result, operations, err
}

func equivalenceFor(perturbationType string) string {
	switch perturbationType {
	case "none", "source_reorder", "exact_duplicate_insertion", "line_ending_change",
		"metadata_noise", "path_rename_identical_content", "irrelevant_appended_text":
		return "equivalent"
	default:
		return "meaning_changing"
	}
}

func caseSeed(caseID string) uint64 {
	sum := sha256.Sum256([]byte(caseID + "\n" + TransformVersion))
	var seed uint64
	for _, value := range sum[:8] {
		seed = seed<<8 | uint64(value)
	}
	return seed & ((1 << 53) - 1)
}

func groundTruthFor(def caseDefinition, sources SourceSet) GroundTruth {
	evidenceDigest := sources.SourceSetDigest
	answers := []GroundTruthAnswer{
		{QuestionID: Questions[0].ID, Label: def.Evidence.EvidenceComplete, SupportingEvidenceDigests: []string{evidenceDigest}},
		{QuestionID: Questions[1].ID, Label: def.Evidence.ObservedTestsSupport, SupportingEvidenceDigests: []string{evidenceDigest}},
		{QuestionID: Questions[2].ID, Label: def.Evidence.VerifierSupport, SupportingEvidenceDigests: []string{evidenceDigest}},
		{QuestionID: Questions[3].ID, Label: def.Evidence.CleanupComplete, SupportingEvidenceDigests: []string{evidenceDigest}},
		{QuestionID: Questions[4].ID, Label: def.Evidence.ExternalEffectsResolved, SupportingEvidenceDigests: []string{evidenceDigest}},
		{QuestionID: Questions[5].ID, Label: def.Evidence.PatchRisk, SupportingEvidenceDigests: []string{evidenceDigest}},
		{QuestionID: Questions[6].ID, Label: def.Evidence.RecommendedDisposition, SupportingEvidenceDigests: []string{evidenceDigest}},
	}
	return GroundTruth{
		Answers:           answers,
		PolicyEvidence:    PolicyEvidence{Contradiction: def.Evidence.Contradiction, Stale: def.Evidence.Stale},
		DerivationReasons: append([]string(nil), def.Reasons...),
	}
}

func labelSeal(caseID string, truth GroundTruth) (string, error) {
	return structuredDigest("pilot-label-seal", map[string]any{
		"case_id":      caseID,
		"ground_truth": truth,
	})
}

func transformationDigest(caseID string, perturbation Perturbation) (string, error) {
	return structuredDigest("transformation", map[string]any{
		"case_id": caseID, "perturbation_id": perturbation.PerturbationID,
		"type": perturbation.Type, "transform_version": perturbation.TransformVersion,
		"seed": perturbation.Seed, "expected_equivalence": perturbation.ExpectedEquivalence,
		"base_source_set_digest":   perturbation.BaseSourceSetDigest,
		"result_source_set_digest": perturbation.ResultSourceSetDigest,
		"operations":               perturbation.Operations,
	})
}

func compiledContext(set SourceSet) []byte {
	byID := make(map[string]Source, len(set.Sources))
	for _, source := range set.Sources {
		byID[source.SourceID] = source
	}
	var buffer bytes.Buffer
	for _, sourceID := range set.ManifestOrder {
		source := byID[sourceID]
		fmt.Fprintf(&buffer, "--- BEGIN SOURCE: %s ---\n%s", source.RelativePath, source.Content)
		if !strings.HasSuffix(source.Content, "\n") {
			buffer.WriteByte('\n')
		}
		fmt.Fprintf(&buffer, "--- END SOURCE: %s ---\n", source.RelativePath)
	}
	return buffer.Bytes()
}

func compiledContextDigest(set SourceSet) string {
	return digestBytes(compiledContext(set))
}

func rawCompilerDigest() string {
	return digestText("context-build-artifact/raw-deterministic-concatenation/v1")
}
