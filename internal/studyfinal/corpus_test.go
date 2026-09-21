package studyfinal

import (
	"bytes"
	"strings"
	"testing"
)

func testCorpus(t *testing.T) Dataset {
	t.Helper()
	d, err := BuildCorpus(Config{Seed: "test-seed"})
	if err != nil {
		t.Fatal(err)
	}
	return d
}

func TestCorpusExactAllocationAndDeterminism(t *testing.T) {
	first := testCorpus(t)
	second := testCorpus(t)
	if first.CorpusSHA256 != second.CorpusSHA256 || len(first.Bases) != 21 || len(first.Conditions) != 150 {
		t.Fatalf("non-deterministic or wrong allocation")
	}
	repositories := map[string]int{}
	roles := map[string]int{}
	for _, c := range first.Conditions {
		repositories[c.Repository]++
		roles[c.RepositoryRole]++
	}
	if repositories["Distill"] != 77 || repositories["LLMTraceFX"] != 41 || repositories["AgentTrace"] != 32 {
		t.Fatalf("wrong repository allocation: %#v", repositories)
	}
	if roles["threshold_development"] != 77 || roles["safety_calibration"] != 41 || roles["confirmatory_secondary"] != 32 {
		t.Fatalf("wrong role allocation: %#v", roles)
	}
	reviewed, err := reviewedSourceAnchors()
	if err != nil {
		t.Fatal(err)
	}
	anchorCount := 0
	for _, anchors := range reviewed {
		anchorCount += len(anchors)
	}
	expectedRegistry := len(first.Bases) + anchorCount
	for _, condition := range first.Conditions {
		expectedRegistry += len(condition.SourcePaths)
	}
	if len(first.Registry) != expectedRegistry {
		t.Fatalf("expected %d source registry records, got %d", expectedRegistry, len(first.Registry))
	}

	for _, base := range first.Bases {
		if strings.Contains(strings.ToLower(base.ID), "pilot") || strings.Contains(strings.ToLower(base.Evidence), "pilot") {
			t.Fatalf("pilot contamination: %s", base.ID)
		}
	}
	if !first.Executable {
		t.Fatal("verified source registry must be executable")
	}
}

func TestCorpusRejectsSourceRegistryByteAndRehashTampering(t *testing.T) {
	d := testCorpus(t)
	d.SourceRegistry = append(append([]byte(nil), d.SourceRegistry...), ' ')
	if err := ValidateCorpus(d); err == nil {
		t.Fatal("source registry byte tampering accepted")
	}
	d.SourceRegistrySHA256 = digestBytes(d.SourceRegistry)
	d.CorpusSHA256, _ = corpusDigest(d)
	if err := ValidateCorpus(d); err == nil {
		t.Fatal("self-consistently rehashed source registry accepted")
	}
}

func TestCorpusRejectsLegacyAgentTraceVocabulary(t *testing.T) {
	d := testCorpus(t)
	for i := range d.Bases {
		if d.Bases[i].Repository == "AgentTrace" {
			d.Bases[i].RepositoryRole = "held_out_repository"
			d.Bases[i].Split = "held-out"
			break
		}
	}
	if err := ValidateCorpus(d); err == nil {
		t.Fatal("legacy AgentTrace role/split aliases accepted")
	}
}

func TestDistillTransformsAreMaterial(t *testing.T) {
	d := testCorpus(t)
	byFamily := map[string]Condition{}
	for _, condition := range d.Conditions {
		if condition.BaseID == distillBaseIDs[0] {
			byFamily[condition.Family] = condition
		}
	}
	if len(byFamily["exact_duplicate"].SourceContents) != 3 ||
		byFamily["exact_duplicate"].SourceContents[0] != byFamily["exact_duplicate"].SourceContents[2] {
		t.Fatal("exact duplicate transform is not material")
	}
	if !strings.Contains(byFamily["line_endings"].SourceContents[0], "\r\n") {
		t.Fatal("line-ending transform missing")
	}
	if len(byFamily["missing_evidence"].SourceContents) != 1 {
		t.Fatal("missing-evidence transform retained required evidence")
	}
	if len(byFamily["contradiction"].SourceContents) != 3 ||
		!strings.Contains(byFamily["contradiction"].SourceContents[2], "contradicts") {
		t.Fatal("contradiction transform missing")
	}
	if !strings.HasSuffix(byFamily["source_reorder"].SourcePaths[0], "/execution.txt") ||
		!strings.HasSuffix(byFamily["source_reorder"].SourcePaths[1], "/summary.txt") {
		t.Fatal("source reorder did not preserve non-lexical manifest order")
	}
	contradiction := byFamily["contradiction"]
	facts := IndependentFacts(contradiction)
	contradiction.Family = "base"
	if IndependentFacts(contradiction) != facts || !facts.SchemaContradiction {
		t.Fatal("independent facts depend on generator-assigned family")
	}
	if IndependentFacts(contradiction) == expectedFactsForFamily(contradiction.Family) {
		t.Fatal("family/evidence semantic mismatch was not detectable")
	}
	for i := range d.Conditions {
		if d.Conditions[i].ID == byFamily["contradiction"].ID {
			d.Conditions[i].Family = "base"
			break
		}
	}
	if err := ValidateCorpus(d); err == nil {
		t.Fatal("generator family mislabel passed corpus validation")
	}
}

func TestCorpusExactBaseIDsAndCommits(t *testing.T) {
	if LLMTraceFXCommit != "f9385e1d9ebf862ba46272fb8a45d7881b0b2a9c" ||
		AgentTraceCommit != "b109ec5b3714b842746e97ee8e975329d8582667" ||
		LLMTraceLicenseSHA256 != "c71d239df91726fc519c6eb72d318ec65820627232b2f796219e87dcf35d0ab4" ||
		AgentTraceLicenseSHA256 != "0a46f53c37edf8ba569b283cbc95fca5b37c7abee62fa5bd46efadf34fa82a80" {
		t.Fatal("external registry constants drift")
	}
	d := testCorpus(t)
	want := append(append(append([]string{}, distillBaseIDs...), llmTraceBaseIDs...), agentTraceBaseIDs...)
	for i, id := range want {
		if d.Bases[i].ID != id {
			t.Fatalf("base %d: got %q want %q", i, d.Bases[i].ID, id)
		}
	}
	for _, r := range d.Registry {
		switch r.Repository {
		case "Distill":
			if r.Commit != DistillCommit {
				t.Fatal("Distill commit drift")
			}
		case "AgentTrace":
			if r.Commit != AgentTraceCommit || r.RepositoryURL != "https://github.com/Siddhant-K-code/agent-trace" ||
				r.LicenseSHA256 != AgentTraceLicenseSHA256 {
				t.Fatal("AgentTrace commit drift")
			}
		case "LLMTraceFX":
			if r.Commit != LLMTraceFXCommit || r.Resolution != "resolved" || r.LicenseSHA256 != LLMTraceLicenseSHA256 {
				t.Fatal("LLMTraceFX registry drift")
			}
			exactAnchors := map[string]sourceAnchor{
				"KV-COMPLETE-01":            {"LLMTraceFX", "llmtracefx/cache_audit/demo.py", "3b25a5a05af63c6290b0b2afd8286206d9c8dd906162033bbf45fb6194566813"},
				"KV-INTEGRITY-02":           {"LLMTraceFX", "tests/cache_audit/test_reference_bundle.py", "d79752bda77b03c1f86b2be8fba6a6e3cf2160263fba32ce98969a1a286e938a"},
				"KV-CONTRADICTION-03":       {"LLMTraceFX", "tests/cache_audit/test_verdicts.py", "0ea122a8a8717bdb3b8727b79d3909a5a302ceaf59d1fb1dd8c451e38a2951e4"},
				"COMPARABILITY-IDENTITY-04": {"LLMTraceFX", "tests/optimizer/test_compare_hardening.py", "0d73d0c265e49a19fd9df4571b08c3185af3c40cbf0103a047b06a6fa7313251"},
				"ABSENCE-TTFT-05":           {"LLMTraceFX", "tests/optimizer/test_openai_api_collector.py", "ad3f1d29cd81ed2d67e92fbb1e556f7a3d868bf53d76d9b9f9da632f261ea00b"},
				"REFUSAL-TEARDOWN-06":       {"LLMTraceFX", "tests/deploy/test_vllm_kv_truth_lifecycle.py", "18de1c9e93ef404ce32586d3cbfda41c3ca1b62e912afca248909d3ebca1b8e2"},
				"CATALOG-LINEAGE-07":        {"LLMTraceFX", "tests/test_evidence_catalog.py", "ee6661b96a65216afe0e5a6f453a08f192169cbabacebe9ea23cd2d4431a8b39"},
				"AT-COV-01":                 {"AgentTrace", "tests/test_hooks.py", "041027cce1229a0b893f0d5b4b1b9c47329a4ef403a122fafb7ea5eb3840ee51"},
				"AT-CLAIM-01":               {"AgentTrace", "tests/test_assignment.py", "b928dd32da2de0276bbd5f15e5e72659d6ae9fa3f6404aba2d52be7c43059116"},
				"AT-REC-01":                 {"AgentTrace", "tests/test_lint.py", "3ee583f3bf4564abf7a412418ded28d3b8241f135fb8b7b4baf3acc05ffc47e7"},
				"AT-DIFF-01":                {"AgentTrace", "tests/test_diff_compare.py", "28d026e368081013ce536701e887f2c8ea1a1e18ecbd32acb9bc00fb01165a0c"},
				"AT-PRIV-01":                {"AgentTrace", "tests/test_anonymize.py", "5563fca89c19fd1b8a855aca9d14b51397d81ac4c77502731ecdbe69f388e712"},
				"AT-INT-01":                 {"AgentTrace", "tests/test_hash_chain.py", "1101a7e889a299593c481caee400e16f5870ad15610f0ce5de741b4164e07d8e"},
				"AT-USAGE-01":               {"AgentTrace", "tests/test_provider_cost.py", "d1cc9cfe7d0cc52d95e3d05ddad9058e02e8b444e05c42b634d8360f515eca9f"},
			}
			if len(sourceAnchors) != len(exactAnchors) {
				t.Fatalf("source anchor count drift: %d", len(sourceAnchors))
			}
			for id, wantAnchor := range exactAnchors {
				if sourceAnchors[id] != wantAnchor {
					t.Fatalf("compiled source anchor drift for %s", id)
				}
				found := false
				for _, entry := range d.Registry {
					if entry.Kind == "source-anchor" && entry.BaseID == id {
						found = entry.Path == wantAnchor.Path && entry.ContentSHA256 == wantAnchor.SHA256
					}
				}
				if !found {
					t.Fatalf("missing exact source anchor %s", id)
				}
			}
			unresolved, err := BuildCorpus(Config{LLMTraceFXCommit: OfflineUnresolved, Seed: "offline-fixture"})
			if err != nil || unresolved.Executable {
				t.Fatalf("offline-unresolved fixture invalid: executable=%v err=%v", unresolved.Executable, err)
			}
		default:
			t.Fatalf("unexpected repository %q", r.Repository)
		}
	}
}

func TestIndependentValidationRejectsContaminationAndPaths(t *testing.T) {
	d := testCorpus(t)
	d.Conditions[0].SourceContents[0] += " pilot-case-001"
	if err := ValidateCorpus(d); err == nil {
		t.Fatal("pilot contamination accepted")
	}

	d = testCorpus(t)
	d.Conditions[0].SourcePaths[0] = "../secret"
	if err := ValidateCorpus(d); err == nil {
		t.Fatal("traversal accepted")
	}
	d = testCorpus(t)
	d.Registry[0].Commit = strings.Repeat("0", 40)
	if err := ValidateCorpus(d); err == nil {
		t.Fatal("source commit drift accepted")
	}
}

func TestAgentTraceDefaultsToUnverifiedDowngrade(t *testing.T) {
	d, err := BuildCorpus(Config{LLMTraceFXCommit: LLMTraceFXCommit, Seed: "downgrade"})
	if err != nil {
		t.Fatal(err)
	}

	func() {
		raw := []byte(strings.Join([]string{
			"# Final-Study Contamination Ledger v1",
			"**Record:** `context-build-artifact/final-contamination-ledger/v1`",
			"## Repository role decision",
			"AgentTrace at " + AgentTraceCommit + " is eligible for the",
			"repository-level threshold holdout",
			"## Pilot exclusion audit",
			"| AgentTrace repository name or commit in pilot source/generated artifacts | absent |",
			"| AgentTrace final family IDs in pilot source/generated artifacts | absent |",
			"| Exact AgentTrace anchor-file digest reused by a pilot artifact | absent |",
			"| Exact AgentTrace public synthetic-bundle member digest reused by a pilot artifact | absent |",
			"| Pilot case/request/label identifiers reused as final identifiers | prohibited |",
			"| Pilot artifacts admitted to the final namespace | prohibited |",
			"Any early access or exact derivative discovered later downgrades AgentTrace",
		}, "\n"))
		env, _ := testReviewEnvironment()
		env.contaminationDigest = digestBytes(raw)
		validated, err := validateAgentTraceArtifact(raw, env.contaminationDigest, env)
		if err != nil {
			t.Fatal(err)
		}
		d, err := buildCorpus(Config{LLMTraceFXCommit: LLMTraceFXCommit, AgentTraceContamination: validated}, env)
		if err != nil || d.AgentTraceContamination.Status != AgentTraceHeldOut {
			t.Fatalf("pinned reviewed Markdown did not enable held-out: %#v err=%v", d.AgentTraceContamination, err)
		}
		if _, err := validateAgentTraceArtifact(raw, strings.Repeat("a", 64), env); err == nil {
			t.Fatal("arbitrary expected digest accepted")
		}
	}()

	func() {
		env, signers := testReviewEnvironment()
		artifact := AgentTraceContaminationArtifact{
			SchemaVersion: SchemaVersion + "/agenttrace-contamination",
			RepositoryURL: repositoryURL("AgentTrace"), Commit: AgentTraceCommit, Decision: "clean-held-out",
			TrustRootVersion: env.trustVersion,
		}
		artifact.TrustRootSHA256, _ = trustedReviewersDigest(env.trusted)
		for _, category := range contaminationCategories {
			artifact.Categories = append(artifact.Categories, ContaminationCategory{
				Name: category, Decision: "not-used", EvidenceSHA256: digestBytes([]byte("reviewed-" + category)),
			})
		}
		raw, err := finalizeContaminationArtifact(artifact, signers)
		if err != nil {
			t.Fatal(err)
		}
		env.contaminationDigest = digestBytes(raw)
		validated, err := validateAgentTraceArtifact(raw, digestBytes(raw), env)
		if err != nil {
			t.Fatal(err)
		}
		d, err := buildCorpus(Config{LLMTraceFXCommit: LLMTraceFXCommit, Seed: "clean-heldout", AgentTraceContamination: validated}, env)
		if err != nil {
			t.Fatal(err)
		}
		for _, base := range d.Bases {
			if base.Repository == "AgentTrace" && (base.RepositoryRole != "held_out" || base.Split != "held_out_repository") {
				t.Fatal("validated clean artifact did not enable held-out")
			}
		}
		if _, err := buildSchedule(d, "clean-heldout", env); err != nil {
			t.Fatal(err)
		}

		if _, err := ValidateAgentTraceArtifact(raw, digestBytes(raw)); err == nil {
			t.Fatal("caller-signed artifact passed absent production trust root")
		}
		fake := &ValidatedAgentTraceArtifact{raw: []byte(`{"decision":"clean-held-out"}`), digest: strings.Repeat("a", 64), decision: "clean-held-out"}
		if _, err := buildCorpus(Config{LLMTraceFXCommit: LLMTraceFXCommit, AgentTraceContamination: fake}, env); err == nil {
			t.Fatal("arbitrary digest enabled held-out")
		}
		wrongCommit := artifact
		wrongCommit.Commit = DistillCommit
		wrongRaw, _ := finalizeContaminationArtifact(wrongCommit, signers)
		wrongEnv := env
		wrongEnv.contaminationDigest = digestBytes(wrongRaw)
		if _, err := validateAgentTraceArtifact(wrongRaw, digestBytes(wrongRaw), wrongEnv); err == nil {
			t.Fatal("wrong AgentTrace commit accepted")
		}
		missingCategory := artifact
		missingCategory.Categories = append([]ContaminationCategory(nil), artifact.Categories[1:]...)
		missingRaw, _ := finalizeContaminationArtifact(missingCategory, signers)
		missingEnv := env
		missingEnv.contaminationDigest = digestBytes(missingRaw)
		if _, err := validateAgentTraceArtifact(missingRaw, digestBytes(missingRaw), missingEnv); err == nil {
			t.Fatal("missing contamination category accepted")
		}
		var unknownReviewer AgentTraceContaminationArtifact
		if err := strictDecode(bytes.NewReader(raw), &unknownReviewer); err != nil {
			t.Fatal(err)
		}
		unknownReviewer.Approvals[0].Review = "unknown-review"
		unknownReviewer.ArtifactSHA256, _ = contaminationArtifactDigest(unknownReviewer)
		unknownRaw, _ := canonicalJSON(unknownReviewer)
		unknownEnv := env
		unknownEnv.contaminationDigest = digestBytes(unknownRaw)
		if _, err := validateAgentTraceArtifact(unknownRaw, digestBytes(unknownRaw), unknownEnv); err == nil {
			t.Fatal("unknown contamination reviewer accepted")
		}
		var invalidSignature AgentTraceContaminationArtifact
		if err := strictDecode(bytes.NewReader(raw), &invalidSignature); err != nil {
			t.Fatal(err)
		}
		invalidSignature.Approvals[0].SignatureHex = strings.Repeat("0", 128)
		invalidSignature.ArtifactSHA256, _ = contaminationArtifactDigest(invalidSignature)
		invalidRaw, _ := canonicalJSON(invalidSignature)
		invalidEnv := env
		invalidEnv.contaminationDigest = digestBytes(invalidRaw)
		if _, err := validateAgentTraceArtifact(invalidRaw, digestBytes(invalidRaw), invalidEnv); err == nil {
			t.Fatal("invalid contamination signature accepted")
		}
		modified := append([]byte(nil), raw...)
		modified[len(modified)-1] ^= 1
		if _, err := validateAgentTraceArtifact(modified, digestBytes(raw), env); err == nil {
			t.Fatal("modified contamination artifact accepted")
		}
		if bytes.Contains(raw, []byte("credential")) {
			t.Fatal("contamination fixture contains credential-shaped canary")
		}
		contaminated := artifact
		contaminated.Decision = "contaminated-downgrade"
		contaminated.Categories = append([]ContaminationCategory(nil), artifact.Categories...)
		contaminated.Categories[0].Decision = "contaminated"
		contaminatedRaw, _ := finalizeContaminationArtifact(contaminated, signers)
		contaminatedEnv := env
		contaminatedEnv.contaminationDigest = digestBytes(contaminatedRaw)
		contaminatedValidated, err := validateAgentTraceArtifact(contaminatedRaw, digestBytes(contaminatedRaw), contaminatedEnv)
		if err != nil {
			t.Fatal(err)
		}
		downgraded, err := buildCorpus(Config{LLMTraceFXCommit: LLMTraceFXCommit, AgentTraceContamination: contaminatedValidated}, contaminatedEnv)
		if err != nil {
			t.Fatal(err)
		}
		for _, base := range downgraded.Bases {
			if base.Repository == "AgentTrace" && base.RepositoryRole != "confirmatory_secondary" {
				t.Fatal("contaminated artifact was overridden to held-out")
			}
		}
	}()
	count := 0
	for _, condition := range d.Conditions {
		if condition.Repository == "AgentTrace" {
			count++
			if condition.RepositoryRole != "confirmatory_secondary" || condition.Split != "confirmatory_secondary" {
				t.Fatalf("AgentTrace condition retained held-out identity: %#v", condition)
			}
		}
	}
	if count != 32 || d.AgentTraceContamination.EvidenceSHA256 != "" || d.AgentTraceContamination.Verified {
		t.Fatalf("invalid downgrade allocation: count=%d disposition=%#v", count, d.AgentTraceContamination)
	}
	schedule, err := BuildSchedule(d, "downgraded-schedule")
	if err != nil || len(schedule) != 420 {
		t.Fatalf("downgraded schedule: %d, %v", len(schedule), err)
	}
	for _, entry := range schedule {
		if entry.RepositoryRole == "held_out" || entry.Split == "held_out_repository" {
			t.Fatal("downgraded AgentTrace leaked into held-out schedule")
		}
	}
}

func TestContextArmGoldenProperties(t *testing.T) {
	c := Condition{
		ID: "golden", SourcePaths: []string{"z.txt", "a.txt", "copy.txt"},
		SourceContents: []string{"z\r\n", "same\r", "same\r"},
	}
	a, err := CompileContext(c, ArmRaw)
	if err != nil {
		t.Fatal(err)
	}
	if want := "--- BEGIN SOURCE: z.txt ---\nz\r\n--- END SOURCE: z.txt ---\n--- BEGIN SOURCE: a.txt ---\nsame\r\n--- END SOURCE: a.txt ---\n--- BEGIN SOURCE: copy.txt ---\nsame\r\n--- END SOURCE: copy.txt ---\n"; a.Content != want {
		t.Fatalf("Arm A golden mismatch:\n%q", a.Content)
	}
	c1, err := CompileContext(c, ArmDistillLock)
	if err != nil {
		t.Fatal(err)
	}
	c2, err := CompileContext(c, ArmDistillLock)
	if err != nil || c1 != c2 {
		t.Fatal("Arm C is not deterministic")
	}
	if c1.Content != "# Distill Context Bundle\n\nSchema: distill-lock/v0\n\n"+
		"<!-- distill-lock/v0 chunk=0 path=\"a.txt\" start=0 end=5 sha256=a6328afc76e9db71da297ebff4b0d3e7a7eb3b01d917c05a6573fef121b6ecb6 -->\n"+
		"same\n\n<!-- /distill-lock/v0 -->\n\n"+
		"<!-- distill-lock/v0 chunk=1 path=\"z.txt\" start=0 end=2 sha256=c865f6c5ab8d1b0bcd383a5e1e3879d22681c96bf462c269b7581d523fbe70ab -->\n"+
		"z\n\n<!-- /distill-lock/v0 -->\n\n" {
		t.Fatalf("Arm C golden mismatch: %q", c1.Content)
	}
	if _, err := CompileContext(c, "B"); err == nil {
		t.Fatal("omitted Arm B accepted")
	}
	if _, err := CompileContext(c, "D"); err == nil {
		t.Fatal("Arm D accepted")
	}
}
