package studylocalv2

import (
	"fmt"
	"sort"
	"strings"
)

var distillBaseIDs = []string{
	"distill-lock-golden-v0",
	"distill-lock-budget-whole-chunk",
	"distill-lock-source-drift",
	"distill-lock-fs-trust-boundary",
	"distill-lock-config-path-safety",
	"distill-lock-atomic-publication",
	"distill-lock-trusted-verification",
}

var llmTraceBaseIDs = []string{
	"KV-COMPLETE-01",
	"KV-INTEGRITY-02",
	"KV-CONTRADICTION-03",
	"COMPARABILITY-IDENTITY-04",
	"ABSENCE-TTFT-05",
	"REFUSAL-TEARDOWN-06",
	"CATALOG-LINEAGE-07",
}

var distillFamilies = []string{
	"base", "source_reorder", "exact_duplicate", "line_endings", "metadata_noise",
	"path_rename", "irrelevant_append", "relevant_fact_change", "missing_evidence",
	"contradiction", "stale_baseline",
}

var llmTraceFamilies = map[string][]string{
	"KV-COMPLETE-01":            {"complete", "null_timing", "null_memory", "missing_timing", "unsupported_timing", "complete_verified"},
	"KV-INTEGRITY-02":           {"missing_file", "extra_file", "tampered", "resealed", "checksum_mismatch", "closed_registry_invalid"},
	"KV-CONTRADICTION-03":       {"expected_attested", "prompt_work", "output_evaluator", "honest_unknown", "affirmative_unsupported", "schema_conflict"},
	"COMPARABILITY-IDENTITY-04": {"model_mismatch", "runtime_mismatch", "workload_mismatch", "evaluator_mismatch", "explicit_noncomparability", "claimed_pooling"},
	"ABSENCE-TTFT-05":           {"missing", "null", "measured_zero", "buffered_ttft", "first_content", "stream_incomplete"},
	"REFUSAL-TEARDOWN-06":       {"safe_refusal", "budget_preflight", "cleanup_complete", "local_shutdown", "provider_unverified", "provider_verified"},
	"CATALOG-LINEAGE-07":        {"closed_claim", "registry_drift", "dangling_lineage", "cyclic_lineage", "privacy_checksum"},
}

var repeatConditionIDs = []string{
	"ABSENCE-TTFT-05::buffered_ttft",
	"ABSENCE-TTFT-05::null",
	"CATALOG-LINEAGE-07::privacy_checksum",
	"COMPARABILITY-IDENTITY-04::claimed_pooling",
	"COMPARABILITY-IDENTITY-04::explicit_noncomparability",
	"KV-COMPLETE-01::complete",
	"KV-COMPLETE-01::missing_timing",
	"KV-CONTRADICTION-03::affirmative_unsupported",
	"KV-CONTRADICTION-03::honest_unknown",
	"KV-INTEGRITY-02::missing_file",
	"KV-INTEGRITY-02::tampered",
	"REFUSAL-TEARDOWN-06::safe_refusal",
	"distill-lock-atomic-publication::irrelevant_append",
	"distill-lock-budget-whole-chunk::exact_duplicate",
	"distill-lock-budget-whole-chunk::missing_evidence",
	"distill-lock-config-path-safety::path_rename",
	"distill-lock-config-path-safety::stale_baseline",
	"distill-lock-fs-trust-boundary::contradiction",
	"distill-lock-fs-trust-boundary::metadata_noise",
	"distill-lock-golden-v0::base",
	"distill-lock-golden-v0::source_reorder",
	"distill-lock-source-drift::line_endings",
	"distill-lock-source-drift::relevant_fact_change",
	"distill-lock-trusted-verification::base",
}

var sourceRegistry = []SourceAnchor{
	{Dataset: "distill", BaseID: "distill-lock-golden-v0", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit, License: "MIT", LicenseSHA256: "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca", Path: "pkg/lock/lock_test.go", SHA256: "8025a7f25aa1411c4a1a60e8fd3f615dd7bb1248dd6dddabbaa8f4d9e8d48d60"},
	{Dataset: "distill", BaseID: "distill-lock-budget-whole-chunk", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit, License: "MIT", LicenseSHA256: "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca", Path: "pkg/lock/build.go", SHA256: "b2b9d61d0fc3f7764aa0b39ccd66bbf26d97981dd542db4b5cbe9cda1aac4716"},
	{Dataset: "distill", BaseID: "distill-lock-source-drift", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit, License: "MIT", LicenseSHA256: "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca", Path: "testdata/distill-lock-v0/config.json", SHA256: "690fd254e697c5b4a0c542690bce1c733376b16b1f3da7b94d8c2ee3ab7ea713"},
	{Dataset: "distill", BaseID: "distill-lock-fs-trust-boundary", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit, License: "MIT", LicenseSHA256: "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca", Path: "pkg/lock/filesystem.go", SHA256: "304043a4f030f54f147822533fedf5307505824f10cfa38eaa01b852542b8a95"},
	{Dataset: "distill", BaseID: "distill-lock-config-path-safety", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit, License: "MIT", LicenseSHA256: "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca", Path: "pkg/lock/config.go", SHA256: "32d9373e09f8c1fb3e021abe48a90eaed7369b10be1bb95031aa7c942dde5762"},
	{Dataset: "distill", BaseID: "distill-lock-atomic-publication", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit, License: "MIT", LicenseSHA256: "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca", Path: "pkg/lock/lock.go", SHA256: "76eb54f79a3853cf83d327de491d9a526006c87665cd55a2e020ccd34eac7159"},
	{Dataset: "distill", BaseID: "distill-lock-trusted-verification", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit, License: "MIT", LicenseSHA256: "a669ee1a7ee2fd4e1a4024d6a2579da845ad577e14b7c177778270df210b02ca", Path: "pkg/lock/verify.go", SHA256: "6262432245ffd63b078006a5b82152bf9187c7637aaeb5ddd34f48ae3e0f09f8"},
	{Dataset: "llmtracefx", BaseID: "KV-COMPLETE-01", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit, License: "Apache-2.0", LicenseSHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", Path: "llmtracefx/cache_audit/demo.py", SHA256: "3b25a5a05af63c6290b0b2afd8286206d9c8dd906162033bbf45fb6194566813"},
	{Dataset: "llmtracefx", BaseID: "KV-INTEGRITY-02", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit, License: "Apache-2.0", LicenseSHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", Path: "tests/cache_audit/test_reference_bundle.py", SHA256: "d79752bda77b03c1f86b2be8fba6a6e3cf2160263fba32ce98969a1a286e938a"},
	{Dataset: "llmtracefx", BaseID: "KV-CONTRADICTION-03", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit, License: "Apache-2.0", LicenseSHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", Path: "tests/cache_audit/test_verdicts.py", SHA256: "0ea122a8a8717bdb3b8727b79d3909a5a302ceaf59d1fb1dd8c451e38a2951e4"},
	{Dataset: "llmtracefx", BaseID: "COMPARABILITY-IDENTITY-04", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit, License: "Apache-2.0", LicenseSHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", Path: "tests/optimizer/test_compare_hardening.py", SHA256: "0d73d0c265e49a19fd9df4571b08c3185af3c40cbf0103a047b06a6fa7313251"},
	{Dataset: "llmtracefx", BaseID: "ABSENCE-TTFT-05", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit, License: "Apache-2.0", LicenseSHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", Path: "tests/optimizer/test_openai_api_collector.py", SHA256: "ad3f1d29cd81ed2d67e92fbb1e556f7a3d868bf53d76d9b9f9da632f261ea00b"},
	{Dataset: "llmtracefx", BaseID: "REFUSAL-TEARDOWN-06", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit, License: "Apache-2.0", LicenseSHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", Path: "tests/deploy/test_vllm_kv_truth_lifecycle.py", SHA256: "18de1c9e93ef404ce32586d3cbfda41c3ca1b62e912afca248909d3ebca1b8e2"},
	{Dataset: "llmtracefx", BaseID: "CATALOG-LINEAGE-07", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit, License: "Apache-2.0", LicenseSHA256: "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4", Path: "tests/test_evidence_catalog.py", SHA256: "ee6661b96a65216afe0e5a6f453a08f192169cbabacebe9ea23cd2d4431a8b39"},
}

func RepeatConditionIDs() []string {
	return append([]string(nil), repeatConditionIDs...)
}

func BuildCorpus() (Corpus, error) {
	corpus := Corpus{
		SchemaVersion: SchemaVersion, StudyKind: "prospective_local_development_calibration",
		DevelopmentCalibration: true, FinalStudyEligible: false, HeldOutEvidenceIncluded: false,
		ProviderCalls:      0,
		ExclusionStatement: "This local development/calibration study is separate from the frozen TypeSafe final study. It contains no held-out AgentTrace record, label, outcome, or evidence byte and cannot update or repair the frozen protocol from its outcomes.",
		SourceRegistry:     append([]SourceAnchor(nil), sourceRegistry...),
	}
	for _, id := range distillBaseIDs {
		corpus.Bases = append(corpus.Bases, Base{ID: id, Dataset: "distill", Role: "threshold_development", Repository: "Siddhant-K-code/distill", Commit: DistillSourceCommit})
		for ordinal, family := range distillFamilies {
			condition, err := buildCondition(id, "distill", "threshold_development", family, ordinal+1)
			if err != nil {
				return Corpus{}, err
			}
			corpus.Conditions = append(corpus.Conditions, condition)
		}
	}
	for _, id := range llmTraceBaseIDs {
		corpus.Bases = append(corpus.Bases, Base{ID: id, Dataset: "llmtracefx", Role: "safety_calibration", Repository: "Siddhant-K-code/LLMTraceFX", Commit: LLMTraceSourceCommit})
		for ordinal, family := range llmTraceFamilies[id] {
			condition, err := buildCondition(id, "llmtracefx", "safety_calibration", family, ordinal+1)
			if err != nil {
				return Corpus{}, err
			}
			corpus.Conditions = append(corpus.Conditions, condition)
		}
	}
	sort.Slice(corpus.Conditions, func(i, j int) bool { return corpus.Conditions[i].ID < corpus.Conditions[j].ID })
	corpus.CorpusSHA256, _ = DigestDomain("corpus", struct {
		SchemaVersion          string
		StudyKind              string
		DevelopmentCalibration bool
		FinalStudyEligible     bool
		Bases                  []Base
		Conditions             []Condition
		SourceRegistry         []SourceAnchor
	}{corpus.SchemaVersion, corpus.StudyKind, corpus.DevelopmentCalibration, corpus.FinalStudyEligible, corpus.Bases, corpus.Conditions, corpus.SourceRegistry})
	if err := ValidateCorpus(corpus); err != nil {
		return Corpus{}, err
	}
	return corpus, nil
}

func buildCondition(baseID, dataset, role, family string, ordinal int) (Condition, error) {
	paths, contents := conditionSources(baseID, dataset, family, ordinal)
	sources := make([]EvidenceSource, len(paths))
	for i := range paths {
		sources[i] = EvidenceSource{
			ID: fmt.Sprintf("E%02d", i+1), Path: paths[i], Bytes: contents[i],
			SHA256: DigestBytes([]byte(contents[i])),
		}
	}
	sourceSetDigest, err := DigestDomain("source-set", sources)
	if err != nil {
		return Condition{}, err
	}
	seed := DigestBytes([]byte(CorpusSeed + "\x00" + baseID + "\x00" + family))
	condition := Condition{
		ID: baseID + "::" + family, BaseID: baseID, Dataset: dataset, Role: role,
		Family: family, Ordinal: ordinal, ExpectedEquivalent: equivalentFamily(family),
		TransformVersion: "local-control-transform/v2", Seed: seed, Sources: sources,
		SourceSetSHA256: sourceSetDigest,
	}
	condition.TransformSHA256, err = DigestDomain("transform", struct {
		ID, Version, Seed, SourceSetSHA256 string
		ExpectedEquivalent                 bool
	}{condition.ID, condition.TransformVersion, condition.Seed, condition.SourceSetSHA256, condition.ExpectedEquivalent})
	if err != nil {
		return Condition{}, err
	}
	condition.Adjudication, err = DeriveAdjudication(condition.Sources)
	if err != nil {
		return Condition{}, err
	}
	return condition, nil
}

func conditionSources(baseID, dataset, family string, ordinal int) ([]string, []string) {
	root := "local-evidence/" + strings.ToLower(baseID) + "/" + fmt.Sprintf("%02d-%s", ordinal, family)
	anchor := anchorForBase(baseID)
	summary := fmt.Sprintf("evidence_id=E01\ndataset=%s\nbase=%s\ncondition=%s\nsource_anchor_path=%s\nsource_anchor_sha256=%s\nrequired_evidence=present\n", dataset, baseID, family, anchor.Path, anchor.SHA256)
	execution := baseExecution(baseID)
	paths := []string{root + "/summary.txt", root + "/execution.txt"}
	contents := []string{summary, execution}
	switch family {
	case "source_reorder":
		paths[0], paths[1] = paths[1], paths[0]
		contents[0], contents[1] = contents[1], contents[0]
	case "exact_duplicate":
		paths = append(paths, root+"/execution-copy.txt")
		contents = append(contents, execution)
	case "line_endings":
		contents[0] = strings.ReplaceAll(summary, "\n", "\r\n")
	case "metadata_noise":
		paths = append(paths, root+"/metadata.txt")
		contents = append(contents, "evidence_id=E03\nmetadata_only=true\ncatalog_order=17\n")
	case "path_rename":
		paths[0] = root + "/renamed-summary.txt"
	case "irrelevant_append":
		contents[0] += "irrelevant_control=a_square_has_four_equal_sides\n"
	case "relevant_fact_change":
		contents[1] = replaceEvidenceValue(contents[1], "risk", "medium")
	case "missing_evidence":
		contents[0] += "evidence_state=incomplete\n"
		paths, contents = paths[:1], contents[:1]
	case "contradiction":
		paths = append(paths, root+"/contradiction.txt")
		contents = append(contents, "evidence_id=E03\ntests=fail\ncontradiction=true\n")
	case "stale_baseline":
		contents[1] += "baseline=stale\n"
	default:
		contents[0] += semanticEvidence(family)
	}
	if baseID == "distill-lock-fs-trust-boundary" && family == "relevant_fact_change" {
		contents[1] = replaceEvidenceValue(contents[1], "risk", "high")
	}
	if dataset == "distill" && family == "base" {
		switch baseID {
		case "distill-lock-golden-v0":
			contents[0] += deterministicPadding("chunking", 1800)
		case "distill-lock-budget-whole-chunk":
			contents[0] += deterministicPadding("budget", 40_000)
		}
	}
	return paths, contents
}

func anchorForBase(baseID string) SourceAnchor {
	for _, anchor := range sourceRegistry {
		if anchor.BaseID == baseID {
			return anchor
		}
	}
	panic("missing frozen source anchor for " + baseID)
}

func baseExecution(baseID string) string {
	lines := "evidence_id=E02\ntests=pass\nverifier=pass\ncleanup=complete\nexternal_effects=resolved\nrisk=low\n"
	switch baseID {
	case "distill-lock-source-drift":
		lines = replaceEvidenceValue(lines, "risk", "medium")
	case "distill-lock-fs-trust-boundary":
		lines += "teardown=verified\n"
	case "distill-lock-config-path-safety":
		lines += "comparison=not_comparable\n"
	case "distill-lock-atomic-publication":
		lines = replaceEvidenceValue(lines, "cleanup", "incomplete")
	case "distill-lock-trusted-verification":
		lines = replaceEvidenceValue(lines, "verifier", "fail")
	}
	return lines
}

func replaceEvidenceValue(content, key, value string) string {
	lines := strings.Split(content, "\n")
	for i, line := range lines {
		if strings.HasPrefix(line, key+"=") {
			lines[i] = key + "=" + value
		}
	}
	return strings.Join(lines, "\n")
}

func deterministicPadding(prefix string, minimumBytes int) string {
	var out strings.Builder
	for index := 0; out.Len() < minimumBytes; index++ {
		fmt.Fprintf(&out, "%s_%04d=%064x\n", prefix, index, index)
	}
	return out.String()
}

func semanticEvidence(family string) string {
	switch family {
	case "expected_attested", "prompt_work", "output_evaluator", "schema_conflict":
		return "schema_integrity=fail\n"
	case "checksum_mismatch", "tampered":
		return "artifact_integrity=fail\n"
	case "privacy_checksum":
		return "privacy_integrity=fail\n"
	case "affirmative_unsupported", "closed_claim":
		return "claim_support=unsupported\n"
	case "missing_file", "missing", "missing_timing", "stream_incomplete", "honest_unknown":
		return "evidence_state=incomplete\n"
	case "unsupported_timing":
		return "evidence_state=unsupported\n"
	case "safe_refusal", "budget_preflight":
		return "evidence_state=refusal\n"
	case "explicit_noncomparability", "model_mismatch", "runtime_mismatch", "workload_mismatch", "evaluator_mismatch":
		return "comparison=not_comparable\n"
	case "claimed_pooling":
		return "comparison=identity_mismatch\n"
	case "local_shutdown", "provider_unverified":
		return "teardown=unverified\n"
	case "provider_verified":
		return "teardown=verified\n"
	case "buffered_ttft":
		return "timing_claim=buffered_as_ttft\n"
	case "closed_registry_invalid", "resealed", "registry_drift", "dangling_lineage", "cyclic_lineage":
		return "registry_integrity=fail\n"
	default:
		return "variant_state=observed\n"
	}
}

func equivalentFamily(family string) bool {
	switch family {
	case "base", "source_reorder", "exact_duplicate", "line_endings", "metadata_noise", "path_rename", "irrelevant_append",
		"complete", "complete_verified", "null_timing", "null_memory", "explicit_noncomparability", "safe_refusal",
		"provider_verified", "first_content":
		return true
	default:
		return false
	}
}

func DeriveAdjudication(sources []EvidenceSource) (Adjudication, error) {
	if len(sources) == 0 {
		return Adjudication{}, fmt.Errorf("adjudication requires evidence")
	}
	values := map[string][]string{}
	valuesBySource := map[string]map[string][]string{}
	evidenceIDs := make([]string, 0, len(sources))
	evidenceDigests := make([]string, 0, len(sources))
	seenIDs := map[string]bool{}
	for _, source := range sources {
		if source.ID == "" || seenIDs[source.ID] || !safeRelative(source.Path) ||
			DigestBytes([]byte(source.Bytes)) != source.SHA256 {
			return Adjudication{}, fmt.Errorf("invalid digest-verified evidence %q", source.ID)
		}
		seenIDs[source.ID] = true
		valuesBySource[source.ID] = map[string][]string{}
		evidenceIDs = append(evidenceIDs, source.ID)
		evidenceDigests = append(evidenceDigests, source.SHA256)
		for _, line := range strings.Split(strings.ReplaceAll(strings.ReplaceAll(source.Bytes, "\r\n", "\n"), "\r", "\n"), "\n") {
			if line == "" {
				continue
			}
			key, value, ok := strings.Cut(line, "=")
			if !ok || key == "" || value == "" {
				return Adjudication{}, fmt.Errorf("malformed evidence line in %s", source.ID)
			}
			values[key] = append(values[key], value)
			valuesBySource[source.ID][key] = append(valuesBySource[source.ID][key], value)
		}
	}
	decision := "accept"
	rejectKeys := map[string]string{
		"schema_integrity": "fail", "artifact_integrity": "fail", "privacy_integrity": "fail",
		"claim_support": "unsupported", "comparison": "identity_mismatch",
		"timing_claim": "buffered_as_ttft", "registry_integrity": "fail",
	}
	for key, value := range rejectKeys {
		if contains(values[key], value) {
			decision = "reject"
		}
	}
	if contains(values["tests"], "fail") || contains(values["verifier"], "fail") || contains(values["risk"], "high") {
		decision = "reject"
	}
	if decision != "reject" {
		review := len(sources) < 2 || !contains(values["required_evidence"], "present") ||
			contains(values["risk"], "medium") || contains(values["baseline"], "stale") ||
			contains(values["evidence_state"], "incomplete") || contains(values["evidence_state"], "unsupported") ||
			contains(values["evidence_state"], "refusal") || contains(values["comparison"], "not_comparable") ||
			contains(values["teardown"], "unverified") || contains(values["cleanup"], "incomplete")
		if review {
			decision = "review"
		}
	}
	var supporting []string
	for _, sourceID := range evidenceIDs {
		sourceValues := valuesBySource[sourceID]
		supports := false
		switch decision {
		case "reject":
			supports = contains(sourceValues["schema_integrity"], "fail") ||
				contains(sourceValues["artifact_integrity"], "fail") ||
				contains(sourceValues["privacy_integrity"], "fail") ||
				contains(sourceValues["claim_support"], "unsupported") ||
				contains(sourceValues["comparison"], "identity_mismatch") ||
				contains(sourceValues["timing_claim"], "buffered_as_ttft") ||
				contains(sourceValues["registry_integrity"], "fail") ||
				contains(sourceValues["tests"], "fail") ||
				contains(sourceValues["verifier"], "fail") ||
				contains(sourceValues["risk"], "high")
		case "review":
			supports = contains(sourceValues["risk"], "medium") ||
				contains(sourceValues["baseline"], "stale") ||
				contains(sourceValues["evidence_state"], "incomplete") ||
				contains(sourceValues["evidence_state"], "unsupported") ||
				contains(sourceValues["evidence_state"], "refusal") ||
				contains(sourceValues["comparison"], "not_comparable") ||
				contains(sourceValues["teardown"], "unverified") ||
				contains(sourceValues["cleanup"], "incomplete")
		case "accept":
			supports = contains(sourceValues["required_evidence"], "present") ||
				contains(sourceValues["tests"], "pass") ||
				contains(sourceValues["verifier"], "pass") ||
				contains(sourceValues["risk"], "low")
		}
		if supports {
			supporting = append(supporting, sourceID)
		}
	}
	return Adjudication{
		Decision: decision, SourceIDs: evidenceIDs, SourceSHA256: evidenceDigests,
		SupportingEvidenceIDs: supporting,
	}, nil
}

func ValidateCorpus(corpus Corpus) error {
	if corpus.SchemaVersion != SchemaVersion || corpus.StudyKind != "prospective_local_development_calibration" ||
		!corpus.DevelopmentCalibration || corpus.FinalStudyEligible || corpus.HeldOutEvidenceIncluded ||
		corpus.ProviderCalls != 0 ||
		corpus.ExclusionStatement != "This local development/calibration study is separate from the frozen TypeSafe final study. It contains no held-out AgentTrace record, label, outcome, or evidence byte and cannot update or repair the frozen protocol from its outcomes." {
		return fmt.Errorf("invalid local-study corpus envelope")
	}
	if len(corpus.Bases) != 14 || len(corpus.Conditions) != 118 || len(corpus.SourceRegistry) != 14 {
		return fmt.Errorf("expected 14 bases, 118 conditions, and 14 source anchors")
	}
	allowedBases := map[string]Base{}
	for _, base := range corpus.Bases {
		if base.Dataset != "distill" && base.Dataset != "llmtracefx" {
			return fmt.Errorf("unapproved dataset %q", base.Dataset)
		}
		if base.ID == "" || allowedBases[base.ID].ID != "" {
			return fmt.Errorf("duplicate or empty base")
		}
		allowedBases[base.ID] = base
	}
	counts := map[string]int{}
	seen := map[string]bool{}
	for _, condition := range corpus.Conditions {
		base, ok := allowedBases[condition.BaseID]
		families := llmTraceFamilies[condition.BaseID]
		if condition.Dataset == "distill" {
			families = distillFamilies
		}
		if !ok || seen[condition.ID] || condition.Dataset != base.Dataset || condition.Role != base.Role ||
			condition.ID != condition.BaseID+"::"+condition.Family || condition.TransformVersion != "local-control-transform/v2" ||
			condition.Ordinal < 1 || condition.Ordinal > len(families) || families[condition.Ordinal-1] != condition.Family {
			return fmt.Errorf("invalid condition identity %q", condition.ID)
		}
		if strings.HasPrefix(condition.ID, "AT-") {
			return fmt.Errorf("held-out record identifier rejected")
		}
		for _, source := range condition.Sources {
			lower := strings.ToLower(source.Path)
			if strings.Contains(lower, "held-out") || strings.Contains(lower, "held_out") ||
				strings.Contains(lower, "agenttrace") || strings.Contains(lower, "agent-trace") {
				return fmt.Errorf("held-out evidence path rejected")
			}
		}
		sourceSetDigest, _ := DigestDomain("source-set", condition.Sources)
		if sourceSetDigest != condition.SourceSetSHA256 {
			return fmt.Errorf("source-set digest mismatch for %s", condition.ID)
		}
		transformDigest, _ := DigestDomain("transform", struct {
			ID, Version, Seed, SourceSetSHA256 string
			ExpectedEquivalent                 bool
		}{condition.ID, condition.TransformVersion, condition.Seed, condition.SourceSetSHA256, condition.ExpectedEquivalent})
		if transformDigest != condition.TransformSHA256 {
			return fmt.Errorf("transform digest mismatch for %s", condition.ID)
		}
		truth, err := DeriveAdjudication(condition.Sources)
		if err != nil || fmt.Sprint(truth) != fmt.Sprint(condition.Adjudication) {
			return fmt.Errorf("evidence-derived adjudication mismatch for %s", condition.ID)
		}
		seen[condition.ID], counts[condition.BaseID] = true, counts[condition.BaseID]+1
	}
	for _, id := range distillBaseIDs {
		if counts[id] != 11 {
			return fmt.Errorf("distill allocation mismatch for %s", id)
		}
	}
	for _, id := range llmTraceBaseIDs {
		if counts[id] != len(llmTraceFamilies[id]) {
			return fmt.Errorf("LLMTraceFX allocation mismatch for %s", id)
		}
	}
	for i, anchor := range corpus.SourceRegistry {
		base, ok := allowedBases[anchor.BaseID]
		if !ok || anchor.Dataset != base.Dataset || anchor.Repository != base.Repository ||
			anchor.Commit != base.Commit || !safeRelative(anchor.Path) ||
			len(anchor.SHA256) != 64 || len(anchor.LicenseSHA256) != 64 {
			return fmt.Errorf("invalid source anchor for %s", anchor.BaseID)
		}
		if anchor != sourceRegistry[i] {
			return fmt.Errorf("source anchor drift for %s", anchor.BaseID)
		}
	}
	recomputed, _ := DigestDomain("corpus", struct {
		SchemaVersion          string
		StudyKind              string
		DevelopmentCalibration bool
		FinalStudyEligible     bool
		Bases                  []Base
		Conditions             []Condition
		SourceRegistry         []SourceAnchor
	}{corpus.SchemaVersion, corpus.StudyKind, corpus.DevelopmentCalibration, corpus.FinalStudyEligible, corpus.Bases, corpus.Conditions, corpus.SourceRegistry})
	if recomputed != corpus.CorpusSHA256 {
		return fmt.Errorf("corpus digest mismatch")
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
