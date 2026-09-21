package studyfinal

import (
	"bytes"
	"encoding/json"
	"fmt"
	"path"
	"sort"
	"strconv"
	"strings"
	"unicode/utf8"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
	"golang.org/x/text/unicode/norm"
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

var agentTraceBaseIDs = []string{
	"AT-COV-01",
	"AT-CLAIM-01",
	"AT-REC-01",
	"AT-DIFF-01",
	"AT-PRIV-01",
	"AT-INT-01",
	"AT-USAGE-01",
}

type sourceAnchor struct {
	Repository string
	Path       string
	SHA256     string
}

type reviewedRegistry struct {
	Repositories []struct {
		ID         string `json:"id"`
		Repository string `json:"repository"`
		Commit     string `json:"commit"`
		Role       string `json:"role"`
		Anchors    []struct {
			BaseID string `json:"base_id"`
			Path   string `json:"path"`
			SHA256 string `json:"sha256"`
			Paths  []struct {
				Path   string `json:"path"`
				SHA256 string `json:"sha256"`
			} `json:"paths"`
		} `json:"anchors"`
	} `json:"repositories"`
}

func reviewedSourceAnchors() (map[string][]sourceAnchor, error) {
	var registry reviewedRegistry
	if _, err := decodeNormalizedJSON(contextartifact.FinalSourceRegistry); err != nil {
		return nil, err
	}
	if err := json.Unmarshal(contextartifact.FinalSourceRegistry, &registry); err != nil {
		return nil, err
	}
	out := map[string][]sourceAnchor{}
	seen := map[string]bool{}
	for _, repository := range registry.Repositories {
		name := map[string]string{"distill": "Distill", "llmtracefx": "LLMTraceFX", "agenttrace": "AgentTrace"}[repository.ID]
		if name == "" || repository.Repository != strings.TrimPrefix(repositoryURL(name), "https://github.com/") {
			return nil, fmt.Errorf("invalid reviewed source registry repository")
		}
		if repository.Role == "held_out_repository" {
			return nil, fmt.Errorf("legacy repository role in reviewed source registry")
		}
		for _, anchor := range repository.Anchors {
			paths := anchor.Paths
			if anchor.Path != "" {
				paths = append(paths, struct {
					Path   string `json:"path"`
					SHA256 string `json:"sha256"`
				}{anchor.Path, anchor.SHA256})
			}
			for _, item := range paths {
				value := sourceAnchor{name, item.Path, item.SHA256}
				key := value.Repository + "\x00" + anchor.BaseID + "\x00" + value.Path
				if seen[key] || !safeRelative(value.Path) || len(value.SHA256) != 64 {
					return nil, fmt.Errorf("invalid or duplicate reviewed source anchor")
				}
				seen[key] = true
				out[anchor.BaseID] = append(out[anchor.BaseID], value)
			}
		}
	}
	return out, nil
}

var sourceAnchors = func() map[string]sourceAnchor {
	all, err := reviewedSourceAnchors()
	if err != nil {
		panic(err)
	}
	out := map[string]sourceAnchor{}
	for id, anchors := range all {
		if len(anchors) == 1 && anchors[0].Repository != "Distill" {
			out[id] = anchors[0]
		}
	}
	return out
}()

var distillFamilies = []string{
	"base", "source_reorder", "exact_duplicate", "line_endings", "metadata_noise",
	"path_rename", "irrelevant_append", "relevant_fact_change", "missing_evidence",
	"contradiction", "stale_baseline",
}

var specializedFamilies = map[string][]string{
	"KV-COMPLETE-01":            {"complete", "null_timing", "null_memory", "missing_timing", "unsupported_timing", "complete_verified"},
	"KV-INTEGRITY-02":           {"missing_file", "extra_file", "tampered", "resealed", "checksum_mismatch", "closed_registry_invalid"},
	"KV-CONTRADICTION-03":       {"expected_attested", "prompt_work", "output_evaluator", "honest_unknown", "affirmative_unsupported", "schema_conflict"},
	"COMPARABILITY-IDENTITY-04": {"model_mismatch", "runtime_mismatch", "workload_mismatch", "evaluator_mismatch", "explicit_noncomparability", "claimed_pooling"},
	"ABSENCE-TTFT-05":           {"missing", "null", "measured_zero", "buffered_ttft", "first_content", "stream_incomplete"},
	"REFUSAL-TEARDOWN-06":       {"safe_refusal", "budget_preflight", "cleanup_complete", "local_shutdown", "provider_unverified", "provider_verified"},
	"CATALOG-LINEAGE-07":        {"closed_claim", "registry_drift", "dangling_lineage", "cyclic_lineage", "privacy_checksum"},
	"AT-COV-01":                 {"complete", "missing", "cross_window", "before_call", "duplicate_result"},
	"AT-CLAIM-01":               {"observed_success", "claimed_only", "command_mismatch", "dirty_end", "missing_outcome"},
	"AT-REC-01":                 {"failed_unrecovered", "successful_retry", "unfinished", "retry_no_clean_end", "recovered_clean_end"},
	"AT-DIFF-01":                {"normalized_identity", "command_drift", "file_drift", "failure_drift", "html_only"},
	"AT-PRIV-01":                {"all_member_redaction", "local_path_redacted", "secret_leak", "partial_redaction"},
	"AT-INT-01":                 {"checksum", "schema", "sequence", "duplicate_id"},
	"AT-USAGE-01":               {"missing", "explicit_zero", "partial", "unknown_not_zero"},
}

func BuildCorpus(cfg Config) (Dataset, error) {
	return buildCorpus(cfg, productionAuthorizationEnvironment())
}

func buildCorpus(cfg Config, env authorizationEnvironment) (Dataset, error) {
	if cfg.LLMTraceFXCommit == "" {
		cfg.LLMTraceFXCommit = LLMTraceFXCommit
	}
	if cfg.LLMTraceFXCommit != LLMTraceFXCommit && cfg.LLMTraceFXCommit != OfflineUnresolved {
		return Dataset{}, fmt.Errorf("LLMTraceFX commit must match the frozen registry")
	}
	if cfg.Seed == "" {
		cfg.Seed = DefaultCorpusSeed
	}
	if cfg.TokenBound == 0 {
		cfg.TokenBound = DefaultMaxInputToken
	}
	if cfg.TokenBound < 1 {
		return Dataset{}, fmt.Errorf("token bound must be positive")
	}
	agentStatus, agentEvidence, agentVerified := AgentTraceDowngraded, "", false
	var contaminationArtifact []byte
	if cfg.AgentTraceContamination != nil {
		validated, err := validateAgentTraceArtifact(cfg.AgentTraceContamination.raw, cfg.AgentTraceContamination.digest, env)
		if err != nil || validated.digest != cfg.AgentTraceContamination.digest ||
			validated.decision != cfg.AgentTraceContamination.decision {
			return Dataset{}, fmt.Errorf("AgentTrace contamination artifact is not validated")
		}
		contaminationArtifact = append([]byte(nil), validated.raw...)
		agentEvidence, agentVerified = validated.digest, true
		if validated.decision == "clean-held-out" {
			agentStatus = AgentTraceHeldOut
		}
	}
	agentRole, agentSplit := "confirmatory_secondary", "confirmatory_secondary"
	if agentStatus == AgentTraceHeldOut {
		agentRole, agentSplit = "held_out", "held_out_repository"
	}
	d := Dataset{
		SchemaVersion: SchemaVersion, GeneratedOffline: true, Executable: cfg.LLMTraceFXCommit != OfflineUnresolved,
		ArmB: ArmBOmission, ArmD: "not-implemented",
		DistillLock: DistillLockConfig{MergeCommit: DistillCommit, ChunkBytes: 1024, TokenBudget: 8192, Algorithm: "distill-lock/v0"},
		AgentTraceContamination: ContaminationDisposition{
			Status: agentStatus, EvidenceSHA256: agentEvidence, Verified: agentVerified,
			ArtifactPath: "research/context-is-a-build-artifact/final-contamination-ledger-v1.md",
		},
		AgentTraceContaminationArtifact: contaminationArtifact,
		SourceRegistry:                  append([]byte(nil), contextartifact.FinalSourceRegistry...),
		SourceRegistrySHA256:            contextartifact.FinalSourceRegistrySHA256,
	}
	sourceAnchors, err := reviewedSourceAnchors()
	if err != nil {
		return Dataset{}, err
	}
	specs := []struct {
		ids, counts                        []string
		repo, role, split, commit, license string
	}{
		{distillBaseIDs, []string{"11", "11", "11", "11", "11", "11", "11"}, "Distill", "threshold_development", "threshold-development", DistillCommit, "MIT"},
		{llmTraceBaseIDs, []string{"6", "6", "6", "6", "6", "6", "5"}, "LLMTraceFX", "safety_calibration", "safety-calibration", cfg.LLMTraceFXCommit, "Apache-2.0"},
		{agentTraceBaseIDs, []string{"5", "5", "5", "5", "4", "4", "4"}, "AgentTrace", agentRole, agentSplit, AgentTraceCommit, "MIT"},
	}
	for _, spec := range specs {
		for i, id := range spec.ids {
			evidencePath := "offline-evidence/" + strings.ToLower(id) + ".txt"
			evidence := fmt.Sprintf("Public synthetic final-study evidence\nrepository=%s\nbase=%s\nobservable_status=registered\n", spec.repo, id)
			base := BaseCase{ID: id, Repository: spec.repo, RepositoryRole: spec.role, Split: spec.split, EvidencePath: evidencePath, Evidence: evidence, EvidenceSHA256: digestBytes([]byte(evidence))}
			d.Bases = append(d.Bases, base)
			resolution := "resolved"
			if spec.commit == OfflineUnresolved {
				resolution = OfflineUnresolved
			}
			d.Registry = append(d.Registry, SourceRegistryEntry{
				Repository: spec.repo, RepositoryURL: repositoryURL(spec.repo), Kind: "generated-evidence", BaseID: id,
				Commit: spec.commit, License: spec.license, LicensePath: "LICENSE", LicenseSHA256: licenseDigest(spec.repo),
				Path: evidencePath, ContentSHA256: base.EvidenceSHA256, Resolution: resolution,
			})
			for _, anchor := range sourceAnchors[id] {
				d.Registry = append(d.Registry, SourceRegistryEntry{
					Repository: spec.repo, RepositoryURL: repositoryURL(spec.repo), Kind: "source-anchor", BaseID: id,
					Commit: spec.commit, License: spec.license, LicensePath: "LICENSE", LicenseSHA256: licenseDigest(spec.repo),
					Path: anchor.Path, ContentSHA256: anchor.SHA256, Resolution: resolution,
				})
			}
			count, err := strconv.Atoi(spec.counts[i])
			if err != nil {
				return Dataset{}, fmt.Errorf("invalid allocation count for %s: %w", id, err)
			}
			families := distillFamilies
			if spec.repo != "Distill" {
				families = specializedFamilies[id]
			}
			if len(families) != count {
				return Dataset{}, fmt.Errorf("internal allocation mismatch for %s", id)
			}
			for ordinal, family := range families {
				paths, contents := conditionSources(spec.repo, id, family, ordinal+1)
				digests := make([]string, len(contents))
				for i := range contents {
					digests[i] = digestBytes([]byte(contents[i]))
				}
				seed := digestBytes([]byte(cfg.Seed + "\x00" + id + "\x00" + family))
				condition := Condition{
					ID: id + "::" + family, BaseID: id, Repository: spec.repo, RepositoryRole: spec.role, Split: spec.split,
					Family: family, TransformVersion: "final-transform/v1", Ordinal: ordinal + 1, Seed: seed, Applicable: true,
					ExpectedEquivalent: isEquivalent(family), SourcePaths: paths,
					BaseEvidenceSHA256: base.EvidenceSHA256, SourceContents: contents, SourceDigests: digests,
				}
				condition.ConditionSHA256, _ = DigestDomain("condition", struct {
					ID, Seed       string
					Paths, Digests []string
				}{condition.ID, condition.Seed, condition.SourcePaths, condition.SourceDigests})
				condition.TransformReceiptSHA256, _ = DigestDomain("transformation", struct {
					Version, Family, Seed, BaseDigest, ResultDigest string
					Equivalent                                      bool
				}{condition.TransformVersion, condition.Family, condition.Seed, condition.BaseEvidenceSHA256, condition.ConditionSHA256, condition.ExpectedEquivalent})
				condition.LabelCommitmentSlot = digestBytes([]byte("label-commitment-slot-v1\x00" + condition.ID))
				condition.CustodyProtocolSHA256 = digestBytes([]byte("independent-random-256-bit-custody-v1"))
				d.Conditions = append(d.Conditions, condition)
				for sourceIndex, sourcePath := range paths {
					d.Registry = append(d.Registry, SourceRegistryEntry{
						Repository: spec.repo, RepositoryURL: repositoryURL(spec.repo), Kind: "generated-evidence", BaseID: id,
						Commit: spec.commit, License: spec.license, LicensePath: "LICENSE", LicenseSHA256: licenseDigest(spec.repo), Path: sourcePath,
						ContentSHA256: condition.SourceDigests[sourceIndex], Resolution: resolution,
					})
				}

				d.DeduplicationLedger = append(d.DeduplicationLedger, DedupEntry{ConditionID: condition.ID, GroupKey: condition.ConditionSHA256, Decision: "retain-lexically-unique"})
			}

		}
	}
	agentContamination := ContaminationEntry{Repository: "AgentTrace", Status: "repository-level-holdout", Reason: "no exact source, fixture, case, label, or output used in pilot; public pretraining exposure unknown"}
	if agentStatus == AgentTraceDowngraded {
		agentContamination = ContaminationEntry{Repository: "AgentTrace", Status: "confirmatory-secondary", Reason: "prospective contamination ledger downgrade; never represented as held-out"}
	}
	d.ContaminationLedger = []ContaminationEntry{
		{Repository: "Distill", Status: "development-only", Reason: "protocol and pilot informed; barred from held-out"},
		{Repository: "LLMTraceFX", Status: "calibration-only", Reason: "protocol evidence informed; barred from held-out"},
		agentContamination,
	}
	d.QuestionSchemaSHA256, _ = DigestDomain("question-schema", AtomicQuestions())
	d.SplitSHA256, _ = DigestDomain("split", struct {
		Bases  []BaseCase
		Ledger []ContaminationEntry
	}{d.Bases, d.ContaminationLedger})
	d.CorpusSHA256, _ = corpusDigest(d)
	if err := validateCorpus(d, env); err != nil {
		return Dataset{}, err
	}
	return d, nil
}

func corpusDigest(d Dataset) (string, error) {
	return DigestDomain("final-corpus", struct {
		Schema                       string
		GeneratedOffline, Executable bool
		Registry                     []SourceRegistryEntry
		Bases                        []BaseCase
		Conditions                   []Condition
		ArmB, ArmD, Questions, Split string
		Lock                         DistillLockConfig
		Contamination                ContaminationDisposition
		ContaminationArtifact        []byte
		SourceRegistry               []byte
		SourceRegistrySHA256         string
	}{d.SchemaVersion, d.GeneratedOffline, d.Executable, d.Registry, d.Bases, d.Conditions, d.ArmB, d.ArmD, d.QuestionSchemaSHA256, d.SplitSHA256, d.DistillLock, d.AgentTraceContamination, d.AgentTraceContaminationArtifact, d.SourceRegistry, d.SourceRegistrySHA256})
}

func repositoryURL(repository string) string {
	switch repository {
	case "Distill":
		return "https://github.com/Siddhant-K-code/distill"
	case "LLMTraceFX":
		return "https://github.com/Siddhant-K-code/LLMTraceFX"
	case "AgentTrace":
		return "https://github.com/Siddhant-K-code/agent-trace"
	default:
		return ""
	}
}

func licenseDigest(repository string) string {
	switch repository {
	case "Distill":
		return DistillLicenseSHA256
	case "LLMTraceFX":
		return LLMTraceLicenseSHA256
	case "AgentTrace":
		return AgentTraceLicenseSHA256
	default:
		return ""
	}
}

func conditionSources(repository, baseID, family string, ordinal int) ([]string, []string) {
	root := "offline-evidence/" + strings.ToLower(baseID) + "/" + fmt.Sprintf("%02d-%s", ordinal, family)
	summary := fmt.Sprintf("Public synthetic final-study evidence\nrepository=%s\nbase=%s\ncondition=%s\nrequired_facts=complete\n", repository, baseID, family)
	execution := "observed_tests=pass\nverifier=pass\ncleanup=complete\nexternal_effects=resolved\nrisk=low\n"
	paths := []string{root + "/summary.txt", root + "/execution.txt"}
	contents := []string{summary, execution}
	switch family {
	case "source_reorder":
		paths[0], paths[1] = paths[1], paths[0]
		contents[0], contents[1] = contents[1], contents[0]
	case "exact_duplicate":
		paths = append(paths, root+"/summary-copy.txt")
		contents = append(contents, summary)
	case "line_endings":
		contents[0] = strings.ReplaceAll(summary, "\n", "\r\n")
	case "metadata_noise":
		paths = append(paths, root+"/metadata.txt")
		contents = append(contents, "catalog_order=17\nnon_semantic=true\n")
	case "path_rename":
		paths[0] = root + "/renamed-summary.txt"
	case "irrelevant_append":
		contents[0] += "irrelevant_control=a square has four equal sides\n"
	case "relevant_fact_change":
		contents[1] = strings.Replace(execution, "risk=low", "risk=medium", 1)
	case "missing_evidence":
		paths, contents = paths[:1], contents[:1]
	case "contradiction":
		paths = append(paths, root+"/contradiction.txt")
		contents = append(contents, "observed_tests=fail\ncontradicts=execution.txt\n")
	case "stale_baseline":
		paths = append(paths, root+"/lifecycle.txt")
		contents = append(contents, "baseline=superseded_after_evidence\n")
	default:
		contents[0] += fmt.Sprintf("typed_variant=%s\nvalue_state=%s\n%s", family, measurementState(family), semanticEvidence(family))
	}

	return paths, contents
}

func semanticEvidence(family string) string {
	switch family {
	case "expected_attested", "prompt_work", "output_evaluator", "schema_conflict":
		return "semantic=schema_contradiction\n"
	case "checksum_mismatch", "tampered":
		return "semantic=crypto_contradiction\n"
	case "privacy_checksum", "secret_leak", "partial_redaction":
		return "semantic=privacy_contradiction\n"
	case "affirmative_unsupported", "claimed_only", "html_only":
		return "semantic=affirmative_unsupported\n"
	case "missing_file", "missing", "missing_timing", "missing_outcome", "unfinished", "partial", "stream_incomplete":
		return "semantic=honest_incomplete\n"
	case "unsupported_timing":
		return "semantic=honest_unsupported\n"
	case "safe_refusal", "budget_preflight":
		return "semantic=honest_refusal\n"
	case "explicit_noncomparability", "model_mismatch", "runtime_mismatch", "workload_mismatch", "evaluator_mismatch":
		return "semantic=explicit_noncomparability\n"
	case "claimed_pooling":
		return "semantic=claimed_pooling_identity_mismatch\n"
	case "local_shutdown", "provider_unverified":
		return "semantic=teardown_unverified\n"
	case "provider_verified":
		return "semantic=teardown_provider_verified\n"
	case "buffered_ttft":
		return "semantic=buffered_timing_claimed_ttft\n"
	case "closed_registry_invalid", "resealed":
		return "semantic=closed_registry_invalid_reseal\n"
	case "local_path_redacted":
		return "semantic=local_operational_redaction\n"
	default:
		return ""
	}
}

func measurementState(family string) string {
	switch family {
	case "missing", "missing_timing", "missing_file", "missing_outcome":
		return "missing"
	case "null_timing", "null_memory":
		return "null"
	case "measured_zero", "explicit_zero":
		return "measured_zero"
	default:
		return "present"
	}
}

func isEquivalent(family string) bool {
	switch family {
	case "base", "source_reorder", "exact_duplicate", "line_endings", "metadata_noise", "path_rename", "irrelevant_append",
		"complete", "complete_verified", "null_timing", "null_memory", "explicit_noncomparability", "safe_refusal",
		"provider_verified", "all_member_redaction", "normalized_identity", "observed_success", "first_content":
		return true
	default:
		return false
	}
}

func ValidateCorpus(d Dataset) error {
	return validateCorpus(d, productionAuthorizationEnvironment())
}

func validateCorpus(d Dataset, env authorizationEnvironment) error {
	if !bytes.Equal(d.SourceRegistry, contextartifact.FinalSourceRegistry) ||
		d.SourceRegistrySHA256 != contextartifact.FinalSourceRegistrySHA256 ||
		digestBytes(d.SourceRegistry) != d.SourceRegistrySHA256 {
		return fmt.Errorf("reviewed source registry bytes or raw digest mismatch")
	}
	sourceAnchors, err := reviewedSourceAnchors()
	if err != nil {
		return fmt.Errorf("invalid embedded reviewed source registry: %w", err)
	}
	if d.SchemaVersion != SchemaVersion || !d.GeneratedOffline {
		return fmt.Errorf("invalid final corpus envelope")
	}
	if d.ArmB != ArmBOmission || d.ArmD != "not-implemented" {
		return fmt.Errorf("only arms A and C are permitted")
	}
	if d.AgentTraceContamination.ArtifactPath != "research/context-is-a-build-artifact/final-contamination-ledger-v1.md" {
		return fmt.Errorf("unexpected AgentTrace contamination artifact path")
	}
	if d.DistillLock != (DistillLockConfig{MergeCommit: DistillCommit, ChunkBytes: 1024, TokenBudget: 8192, Algorithm: "distill-lock/v0"}) {
		return fmt.Errorf("distill Lock configuration drift")
	}
	if len(d.Bases) != 21 || len(d.Conditions) != 150 {
		return fmt.Errorf("expected 21 bases and 150 conditions, got %d and %d", len(d.Bases), len(d.Conditions))
	}
	expected := map[string]struct {
		repo, role, split string
		count             int
	}{}
	for _, id := range distillBaseIDs {
		expected[id] = struct {
			repo, role, split string
			count             int
		}{"Distill", "threshold_development", "threshold-development", 11}
	}
	for i, id := range llmTraceBaseIDs {
		n := 6
		if i == 6 {
			n = 5
		}
		expected[id] = struct {
			repo, role, split string
			count             int
		}{"LLMTraceFX", "safety_calibration", "safety-calibration", n}
	}
	agentRole, agentSplit := "held_out", "held_out_repository"
	switch d.AgentTraceContamination.Status {
	case AgentTraceDowngraded:
		agentRole, agentSplit = "confirmatory_secondary", "confirmatory_secondary"
		if len(d.AgentTraceContaminationArtifact) == 0 {
			if d.AgentTraceContamination.Verified || d.AgentTraceContamination.EvidenceSHA256 != "" {
				return fmt.Errorf("unverified downgrade has fabricated contamination evidence")
			}
		} else {
			validated, err := validateAgentTraceArtifact(d.AgentTraceContaminationArtifact, d.AgentTraceContamination.EvidenceSHA256, env)
			if err != nil || validated.decision != "contaminated-downgrade" ||
				!d.AgentTraceContamination.Verified || d.AgentTraceContamination.EvidenceSHA256 != validated.digest {
				return fmt.Errorf("invalid AgentTrace downgrade artifact")
			}
		}
	case AgentTraceHeldOut:
		validated, err := validateAgentTraceArtifact(d.AgentTraceContaminationArtifact, d.AgentTraceContamination.EvidenceSHA256, env)
		if err != nil || validated.decision != "clean-held-out" ||
			!d.AgentTraceContamination.Verified || d.AgentTraceContamination.EvidenceSHA256 != validated.digest {
			return fmt.Errorf("held-out AgentTrace requires a validated clean contamination artifact")
		}
	default:
		return fmt.Errorf("invalid AgentTrace contamination disposition")
	}
	for i, id := range agentTraceBaseIDs {
		n := 5
		if i >= 4 {
			n = 4
		}
		expected[id] = struct {
			repo, role, split string
			count             int
		}{"AgentTrace", agentRole, agentSplit, n}
	}
	seenBase, counts, seenCondition := map[string]bool{}, map[string]int{}, map[string]bool{}
	for _, b := range d.Bases {
		e, ok := expected[b.ID]
		if !ok || seenBase[b.ID] || b.Repository != e.repo || b.RepositoryRole != e.role || b.Split != e.split {
			return fmt.Errorf("invalid base %q", b.ID)
		}
		if !safeRelative(b.EvidencePath) || digestBytes([]byte(b.Evidence)) != b.EvidenceSHA256 {
			return fmt.Errorf("invalid evidence for %s", b.ID)
		}
		if strings.Contains(strings.ToLower(b.Evidence), "pilot") {
			return fmt.Errorf("pilot contamination in base %s", b.ID)
		}
		seenBase[b.ID] = true
	}
	for _, c := range d.Conditions {
		e, ok := expected[c.BaseID]
		if !ok || seenCondition[c.ID] || c.Repository != e.repo || c.RepositoryRole != e.role || c.Split != e.split || !c.Applicable {
			return fmt.Errorf("invalid condition %q", c.ID)
		}
		families := distillFamilies
		if c.Repository != "Distill" {
			families = specializedFamilies[c.BaseID]
		}
		if c.Ordinal < 1 || c.Ordinal > len(families) || c.Family != families[c.Ordinal-1] ||
			c.ID != c.BaseID+"::"+c.Family {
			return fmt.Errorf("condition family registry mismatch for %s", c.ID)
		}
		if strings.Contains(strings.ToLower(c.ID+"\x00"+strings.Join(c.SourceContents, "\x00")), "pilot") {
			return fmt.Errorf("pilot contamination in %s", c.ID)
		}
		if len(c.SourcePaths) == 0 || len(c.SourcePaths) != len(c.SourceContents) || len(c.SourcePaths) != len(c.SourceDigests) {
			return fmt.Errorf("invalid sources for %s", c.ID)
		}
		for i := range c.SourcePaths {
			if !safeRelative(c.SourcePaths[i]) || !utf8.ValidString(c.SourceContents[i]) || digestBytes([]byte(c.SourceContents[i])) != c.SourceDigests[i] {
				return fmt.Errorf("invalid source %d for %s", i, c.ID)
			}
		}
		if len(c.LabelCommitmentSlot) != 64 || len(c.CustodyProtocolSHA256) != 64 {
			return fmt.Errorf("invalid label custody for %s", c.ID)
		}
		expectedConditionDigest, _ := DigestDomain("condition", struct {
			ID, Seed       string
			Paths, Digests []string
		}{c.ID, c.Seed, c.SourcePaths, c.SourceDigests})
		if c.ConditionSHA256 != expectedConditionDigest {
			return fmt.Errorf("condition digest mismatch for %s", c.ID)
		}
		if IndependentFacts(c) != expectedFactsForFamily(c.Family) {
			return fmt.Errorf("independent evidence semantics disagree with family registry for %s", c.ID)
		}
		baseDigest := ""
		for _, base := range d.Bases {
			if base.ID == c.BaseID {
				baseDigest = base.EvidenceSHA256
				break
			}
		}
		if c.TransformVersion != "final-transform/v1" || c.BaseEvidenceSHA256 != baseDigest {
			return fmt.Errorf("transform base/version mismatch for %s", c.ID)
		}
		expectedTransformReceipt, _ := DigestDomain("transformation", struct {
			Version, Family, Seed, BaseDigest, ResultDigest string
			Equivalent                                      bool
		}{c.TransformVersion, c.Family, c.Seed, c.BaseEvidenceSHA256, c.ConditionSHA256, c.ExpectedEquivalent})
		if c.TransformReceiptSHA256 != expectedTransformReceipt {
			return fmt.Errorf("transform receipt mismatch for %s", c.ID)
		}
		expectedSlot := digestBytes([]byte("label-commitment-slot-v1\x00" + c.ID))
		if c.LabelCommitmentSlot != expectedSlot ||
			c.CustodyProtocolSHA256 != digestBytes([]byte("independent-random-256-bit-custody-v1")) {
			return fmt.Errorf("custody slot mismatch for %s", c.ID)
		}
		seenCondition[c.ID], counts[c.BaseID] = true, counts[c.BaseID]+1
	}
	for id, e := range expected {
		if !seenBase[id] || counts[id] != e.count {
			return fmt.Errorf("allocation mismatch for %s: %d", id, counts[id])
		}
	}
	reg := map[string]SourceRegistryEntry{}
	anchorCount := 0
	for _, anchors := range sourceAnchors {
		anchorCount += len(anchors)
	}
	expectedRegistryCount := len(d.Bases) + anchorCount
	for _, condition := range d.Conditions {
		expectedRegistryCount += len(condition.SourcePaths)
	}
	if len(d.Registry) != expectedRegistryCount {
		return fmt.Errorf("source registry count mismatch")
	}
	llmCommit := ""
	for _, r := range d.Registry {
		if !safeRelative(r.Path) || len(r.ContentSHA256) != 64 || r.Commit == "" || r.License == "" ||
			(r.Kind != "generated-evidence" && r.Kind != "source-anchor") || expected[r.BaseID].repo != r.Repository ||
			r.RepositoryURL != repositoryURL(r.Repository) {
			return fmt.Errorf("invalid registry entry for %s", r.Repository)
		}
		if r.Repository == "Distill" && r.Commit != DistillCommit {
			return fmt.Errorf("invalid frozen commit for Distill")
		}
		if r.Repository == "AgentTrace" && r.Commit != AgentTraceCommit {
			return fmt.Errorf("invalid frozen commit for AgentTrace")
		}
		if r.Repository == "LLMTraceFX" && r.Commit != LLMTraceFXCommit && r.Commit != OfflineUnresolved {
			return fmt.Errorf("invalid frozen commit for LLMTraceFX")
		}
		if r.Repository == "LLMTraceFX" {
			if llmCommit == "" {
				llmCommit = r.Commit
			} else if llmCommit != r.Commit {
				return fmt.Errorf("inconsistent LLMTraceFX commit")
			}
		}
		if (r.Commit == OfflineUnresolved) != (r.Resolution == OfflineUnresolved) {
			return fmt.Errorf("registry resolution mismatch for %s", r.Repository)
		}
		if r.Commit != OfflineUnresolved && r.Resolution != "resolved" {
			return fmt.Errorf("registry source is not resolved for %s", r.Repository)
		}
		wantLicense := map[string]string{"Distill": "MIT", "LLMTraceFX": "Apache-2.0", "AgentTrace": "MIT"}[r.Repository]
		if wantLicense == "" || r.License != wantLicense || r.LicensePath != "LICENSE" || r.LicenseSHA256 != licenseDigest(r.Repository) {
			return fmt.Errorf("license registry mismatch for %s", r.Repository)
		}
		if strings.Contains(strings.ToLower(r.Path), "pilot") {
			return fmt.Errorf("pilot registry path rejected")
		}
		if r.Repository != "LLMTraceFX" && len(r.Commit) != 40 {
			return fmt.Errorf("invalid frozen commit for %s", r.Repository)
		}
		key := r.Repository + "\x00" + r.BaseID + "\x00" + r.Kind + "\x00" + r.Path
		if _, exists := reg[key]; exists {
			return fmt.Errorf("duplicate registry path for %s", r.Repository)
		}
		reg[key] = r
	}
	if (llmCommit != OfflineUnresolved) != d.Executable {
		return fmt.Errorf("corpus executable status disagrees with source registry")
	}
	for id, anchors := range sourceAnchors {
		for _, anchor := range anchors {
			entry, ok := reg[anchor.Repository+"\x00"+id+"\x00source-anchor\x00"+anchor.Path]
			if !ok || entry.ContentSHA256 != anchor.SHA256 {
				return fmt.Errorf("source anchor mismatch for %s", id)
			}
		}
	}
	for _, b := range d.Bases {
		r, ok := reg[b.Repository+"\x00"+b.ID+"\x00generated-evidence\x00"+b.EvidencePath]
		if !ok || r.ContentSHA256 != b.EvidenceSHA256 {
			return fmt.Errorf("unregistered evidence for %s", b.ID)
		}
		for _, c := range d.Conditions {
			for i, p := range c.SourcePaths {
				r, ok := reg[c.Repository+"\x00"+c.BaseID+"\x00generated-evidence\x00"+p]
				if !ok || r.ContentSHA256 != c.SourceDigests[i] {
					return fmt.Errorf("unregistered condition evidence for %s", c.ID)
				}
			}
		}
	}
	q, _ := DigestDomain("question-schema", AtomicQuestions())
	if q != d.QuestionSchemaSHA256 {
		return fmt.Errorf("question schema digest mismatch")
	}
	agentLedger := ContaminationEntry{Repository: "AgentTrace", Status: "repository-level-holdout", Reason: "no exact source, fixture, case, label, or output used in pilot; public pretraining exposure unknown"}
	if d.AgentTraceContamination.Status == AgentTraceDowngraded {
		agentLedger = ContaminationEntry{Repository: "AgentTrace", Status: "confirmatory-secondary", Reason: "prospective contamination ledger downgrade; never represented as held-out"}
	}
	expectedLedger := []ContaminationEntry{
		{Repository: "Distill", Status: "development-only", Reason: "protocol and pilot informed; barred from held-out"},
		{Repository: "LLMTraceFX", Status: "calibration-only", Reason: "protocol evidence informed; barred from held-out"},
		agentLedger,
	}
	if fmt.Sprint(d.ContaminationLedger) != fmt.Sprint(expectedLedger) {
		return fmt.Errorf("contamination ledger mismatch")
	}
	splitDigest, _ := DigestDomain("split", struct {
		Bases  []BaseCase
		Ledger []ContaminationEntry
	}{d.Bases, d.ContaminationLedger})
	if d.SplitSHA256 != splitDigest {
		return fmt.Errorf("split digest mismatch")
	}
	if len(d.DeduplicationLedger) != len(d.Conditions) {
		return fmt.Errorf("deduplication ledger count mismatch")
	}
	for i, entry := range d.DeduplicationLedger {
		if entry.ConditionID != d.Conditions[i].ID || entry.GroupKey != d.Conditions[i].ConditionSHA256 || entry.Decision != "retain-lexically-unique" {
			return fmt.Errorf("deduplication ledger mismatch")
		}
	}
	recomputed, _ := corpusDigest(d)
	if recomputed != d.CorpusSHA256 {
		return fmt.Errorf("corpus digest mismatch")
	}
	return nil
}

func safeRelative(p string) bool {
	if p == "" || strings.ContainsRune(p, 0) || strings.Contains(p, "\\") || strings.HasPrefix(p, "/") || path.Clean(p) != p {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "." || part == ".." || part == "" {
			return false
		}
	}
	return true
}

func CompileContext(c Condition, arm string) (ContextArtifact, error) {
	if len(c.SourcePaths) != len(c.SourceContents) || len(c.SourcePaths) == 0 {
		return ContextArtifact{}, fmt.Errorf("invalid source set")
	}
	var b strings.Builder
	switch arm {
	case ArmRaw:
		for i, p := range c.SourcePaths {
			if !safeRelative(p) || !utf8.ValidString(c.SourceContents[i]) {
				return ContextArtifact{}, fmt.Errorf("invalid source")
			}
			fmt.Fprintf(&b, "--- BEGIN SOURCE: %s ---\n", p)
			b.WriteString(c.SourceContents[i])
			if !strings.HasSuffix(c.SourceContents[i], "\n") {
				b.WriteByte('\n')
			}
			fmt.Fprintf(&b, "--- END SOURCE: %s ---\n", p)
		}
	case ArmDistillLock:
		type source struct{ p, body string }
		sources := make([]source, 0, len(c.SourcePaths))
		paths := map[string]bool{}
		for i, p := range c.SourcePaths {
			if !safeRelative(p) || !utf8.ValidString(c.SourceContents[i]) {
				return ContextArtifact{}, fmt.Errorf("invalid source")
			}
			if paths[p] {
				return ContextArtifact{}, fmt.Errorf("duplicate source path")
			}
			paths[p] = true
			body := norm.NFC.String(strings.ReplaceAll(strings.ReplaceAll(c.SourceContents[i], "\r\n", "\n"), "\r", "\n"))
			sources = append(sources, source{p, body})
		}
		sort.Slice(sources, func(i, j int) bool {
			return sources[i].p < sources[j].p
		})
		type chunk struct {
			path       string
			start, end int
			body       string
			digest     string
			order      int
		}
		var selected []chunk
		seen, remaining := map[string]bool{}, 8192
		for _, s := range sources {
			for _, bounds := range chunkRanges([]byte(s.body), 1024) {
				body := s.body[bounds[0]:bounds[1]]
				digest := digestBytes([]byte(body))
				identity := fmt.Sprintf("%s:%d", digest, bounds[1]-bounds[0])
				if seen[identity] {
					continue
				}
				seen[identity] = true
				tokens := (len([]byte(body)) + 3) / 4
				if tokens > remaining {
					continue
				}
				remaining -= tokens
				selected = append(selected, chunk{s.p, bounds[0], bounds[1], body, digest, len(selected)})
			}
		}
		b.WriteString("# Distill Context Bundle\n\nSchema: distill-lock/v0\n\n")
		for _, chunk := range selected {
			pathJSON, _ := json.Marshal(chunk.path)
			fmt.Fprintf(&b, "<!-- distill-lock/v0 chunk=%d path=%s start=%d end=%d sha256=%s -->\n",
				chunk.order, pathJSON, chunk.start, chunk.end, chunk.digest)
			b.WriteString(chunk.body)
			b.WriteString("\n<!-- /distill-lock/v0 -->\n\n")
		}
	default:
		return ContextArtifact{}, fmt.Errorf("arm %q is not approved", arm)
	}

	content := b.String()
	return ContextArtifact{ConditionID: c.ID, Arm: arm, Content: content, SHA256: digestBytes([]byte(content))}, nil
}

func chunkRanges(content []byte, limit int) [][2]int {
	if len(content) == 0 {
		return [][2]int{{0, 0}}
	}
	ranges := make([][2]int, 0, (len(content)+limit-1)/limit)
	for start := 0; start < len(content); {
		end := start + limit
		if end >= len(content) {
			end = len(content)
		} else {
			for end > start && !utf8.RuneStart(content[end]) {
				end--
			}
			if end == start {
				_, size := utf8.DecodeRune(content[start:])
				end = start + size
			}
		}
		ranges = append(ranges, [2]int{start, end})
		start = end
	}
	return ranges
}
