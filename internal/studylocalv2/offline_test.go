package studylocalv2

import (
	"bytes"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestFrozenCorpusAndScheduleAllocation(t *testing.T) {
	corpus, err := BuildCorpus()
	if err != nil {
		t.Fatal(err)
	}
	protocol, err := BuildProtocol()
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := BuildContexts(corpus, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := BuildSchedule(corpus, protocol, contexts)
	if err != nil {
		t.Fatal(err)
	}
	if len(corpus.Bases) != 14 || len(corpus.Conditions) != 118 ||
		len(protocol.RepeatConditionIDs) != 24 || len(schedule) != 332 {
		t.Fatalf("unexpected allocation: bases=%d conditions=%d repeats=%d observations=%d",
			len(corpus.Bases), len(corpus.Conditions), len(protocol.RepeatConditionIDs), len(schedule))
	}
	contextBytes, err := canonicalJSONL(contexts)
	if err != nil {
		t.Fatal(err)
	}
	scheduleBytes, err := canonicalJSONL(schedule)
	if err != nil {
		t.Fatal(err)
	}
	if corpus.CorpusSHA256 != KnownCorpusSHA256 ||
		protocol.ProtocolSHA256 != KnownProtocolSHA256 ||
		DigestBytes(contextBytes) != KnownContextsSHA256 ||
		DigestBytes(scheduleBytes) != KnownScheduleSHA256 {
		t.Fatal("v2 known-answer identities drifted")
	}
	repeatBases := map[string]bool{}
	conditions := map[string]Condition{}
	for _, condition := range corpus.Conditions {
		conditions[condition.ID] = condition
	}
	for _, id := range protocol.RepeatConditionIDs {
		condition, ok := conditions[id]
		if !ok {
			t.Fatalf("unknown repeat condition %s", id)
		}
		repeatBases[condition.BaseID] = true
	}
	if len(repeatBases) != 14 {
		t.Fatalf("repeat subset covers %d bases, want 14", len(repeatBases))
	}
}

func TestNoHeldOutRecordsOrFinalStudyDependency(t *testing.T) {
	corpus, err := BuildCorpus()
	if err != nil {
		t.Fatal(err)
	}
	for _, base := range corpus.Bases {
		if strings.HasPrefix(base.ID, "AT-") || base.Dataset == "agenttrace" {
			t.Fatalf("held-out base included: %+v", base)
		}
	}
	for _, condition := range corpus.Conditions {
		for _, source := range condition.Sources {
			lower := strings.ToLower(source.Path + "\x00" + source.Bytes)
			if strings.Contains(lower, "agenttrace") || strings.Contains(lower, "agent-trace") ||
				strings.Contains(lower, "held_out") || strings.Contains(lower, "held-out") {
				t.Fatalf("held-out bytes included in %s", condition.ID)
			}
		}
	}
	_, current, _, _ := runtime.Caller(0)
	entries, err := os.ReadDir(filepath.Dir(current))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) != ".go" || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		data, err := os.ReadFile(filepath.Join(filepath.Dir(current), entry.Name()))
		if err != nil {
			t.Fatal(err)
		}
		if bytes.Contains(data, []byte("internal/studyfinal")) || bytes.Contains(data, []byte("api.typesafe.ai")) {
			t.Fatalf("%s imports a frozen study or provider endpoint", entry.Name())
		}
	}
}

func TestArtifactGenerationIsByteStable(t *testing.T) {
	first, _, err := BuildArtifacts(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	second, _, err := BuildArtifacts(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if len(first) != len(second) {
		t.Fatal("artifact set size changed")
	}
	for name, want := range first {
		if !bytes.Equal(want, second[name]) {
			t.Fatalf("%s changed across clean generation roots", name)
		}
	}
}

func TestRawArmPreservesSubmittedOrderAndBytes(t *testing.T) {
	condition := Condition{ID: "test", Sources: []EvidenceSource{
		{ID: "E01", Path: "z.txt", Bytes: "z\r\n", SHA256: DigestBytes([]byte("z\r\n"))},
		{ID: "E02", Path: "a.txt", Bytes: "a", SHA256: DigestBytes([]byte("a"))},
	}}
	context, err := compileRaw(condition)
	if err != nil {
		t.Fatal(err)
	}
	want := "--- BEGIN SOURCE: z.txt ---\nz\r\n--- END SOURCE: z.txt ---\n" +
		"--- BEGIN SOURCE: a.txt ---\na\n--- END SOURCE: a.txt ---\n"
	if context.Content != want {
		t.Fatalf("raw context changed:\n%s", context.Content)
	}
}

func TestAdjudicationUsesVerifiedEvidenceBytes(t *testing.T) {
	sources := []EvidenceSource{
		{ID: "E01", Path: "summary.txt", Bytes: "evidence_id=E01\nrequired_evidence=present\n", SHA256: DigestBytes([]byte("evidence_id=E01\nrequired_evidence=present\n"))},
		{ID: "E02", Path: "execution.txt", Bytes: "evidence_id=E02\ntests=pass\nrisk=low\n", SHA256: DigestBytes([]byte("evidence_id=E02\ntests=pass\nrisk=low\n"))},
	}
	truth, err := DeriveAdjudication(sources)
	if err != nil || truth.Decision != "accept" {
		t.Fatalf("accept truth: %+v, %v", truth, err)
	}
	sources[1].Bytes = "evidence_id=E02\ntests=fail\nrisk=low\n"
	sources[1].SHA256 = DigestBytes([]byte(sources[1].Bytes))
	truth, err = DeriveAdjudication(sources)
	if err != nil || truth.Decision != "reject" {
		t.Fatalf("reject truth: %+v, %v", truth, err)
	}
	sources[1].SHA256 = strings.Repeat("0", 64)
	if _, err := DeriveAdjudication(sources); err == nil {
		t.Fatal("tampered evidence was accepted")
	}
}

func TestDistillLockMechanismsAreExercised(t *testing.T) {
	corpus, err := BuildCorpus()
	if err != nil {
		t.Fatal(err)
	}
	contexts, err := BuildContexts(corpus, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	compiled := map[string]Context{}
	for _, context := range contexts {
		if context.Arm == ArmDistillLock {
			compiled[context.ConditionID] = context
		}
	}
	duplicate := compiled["distill-lock-budget-whole-chunk::exact_duplicate"]
	if duplicate.LockDuplicateCount < 1 {
		t.Fatal("exact-duplicate condition did not exercise Distill Lock deduplication")
	}
	chunked := compiled["distill-lock-golden-v0::base"]
	if chunked.LockChunkCount <= chunked.LockSourceCount {
		t.Fatal("chunking condition did not split a source")
	}
	budget := compiled["distill-lock-budget-whole-chunk::base"]
	if budget.LockSelectedCount+budget.LockDuplicateCount >= budget.LockChunkCount {
		t.Fatal("budget condition did not exclude a whole chunk")
	}
}

func TestDistillBasesHaveDistinctAnchorsAndTruthProfiles(t *testing.T) {
	corpus, err := BuildCorpus()
	if err != nil {
		t.Fatal(err)
	}
	anchors := map[string]bool{}
	profiles := map[string]string{}
	for _, anchor := range corpus.SourceRegistry {
		if anchor.Dataset == "distill" {
			key := anchor.Path + "\x00" + anchor.SHA256
			if anchors[key] {
				t.Fatalf("duplicate Distill base anchor %s", key)
			}
			anchors[key] = true
		}
	}
	for _, condition := range corpus.Conditions {
		if condition.Dataset == "distill" {
			profiles[condition.BaseID] += condition.Adjudication.Decision + "\x00"
		}
	}
	uniqueProfiles := map[string]bool{}
	for _, profile := range profiles {
		uniqueProfiles[profile] = true
	}
	if len(anchors) != 7 || len(uniqueProfiles) < 4 {
		t.Fatalf("Distill bases are insufficiently distinct: anchors=%d profiles=%d", len(anchors), len(uniqueProfiles))
	}
}

func TestPackageValidationRejectsContextSubstitution(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "package")
	if _, err := Prepare(directory); err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(directory, "contexts.jsonl")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	data = bytes.Replace(data, []byte("required_evidence=present"), []byte("required_evidence=absentx"), 1)
	if err := os.Chmod(path, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := ValidatePackage(directory); err == nil {
		t.Fatal("tampered context package was accepted")
	}
}

func TestPublishedResultSchemaMatchesPackage(t *testing.T) {
	published, err := os.ReadFile(filepath.Join("..", "..", "research", "context-is-a-build-artifact", "local-control-result-schema-v2.json"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(published, resultSchema()) {
		t.Fatal("published result schema differs from packaged bytes")
	}
	if DigestBytes(published) != KnownResultSchemaSHA256 {
		t.Fatal("published result schema known-answer digest drifted")
	}
}

func TestPackageManifestKnownAnswer(t *testing.T) {
	files, _, err := BuildArtifacts(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if DigestBytes(files["package-manifest.json"]) != KnownPackageManifestSHA256 {
		t.Fatal("package manifest known-answer digest drifted")
	}
}

func TestPackageRejectsSelfConsistentCorpusReplacement(t *testing.T) {
	files, _, err := BuildArtifacts(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	var corpus Corpus
	if err := strictJSONObjectFileDecode(files["corpus.json"], &corpus); err != nil {
		t.Fatal(err)
	}
	condition := &corpus.Conditions[0]
	condition.Sources[0].Bytes += "replacement_marker=true\n"
	condition.Sources[0].SHA256 = DigestBytes([]byte(condition.Sources[0].Bytes))
	condition.SourceSetSHA256, _ = DigestDomain("source-set", condition.Sources)
	condition.TransformSHA256, _ = DigestDomain("transform", struct {
		ID, Version, Seed, SourceSetSHA256 string
		ExpectedEquivalent                 bool
	}{condition.ID, condition.TransformVersion, condition.Seed, condition.SourceSetSHA256, condition.ExpectedEquivalent})
	condition.Adjudication, err = DeriveAdjudication(condition.Sources)
	if err != nil {
		t.Fatal(err)
	}
	corpus.CorpusSHA256, _ = DigestDomain("corpus", struct {
		SchemaVersion          string
		StudyKind              string
		DevelopmentCalibration bool
		FinalStudyEligible     bool
		Bases                  []Base
		Conditions             []Condition
		SourceRegistry         []SourceAnchor
	}{corpus.SchemaVersion, corpus.StudyKind, corpus.DevelopmentCalibration, corpus.FinalStudyEligible, corpus.Bases, corpus.Conditions, corpus.SourceRegistry})
	files["corpus.json"], _ = canonicalJSONFile(corpus)
	delete(files, "SHA256SUMS")
	files["SHA256SUMS"] = renderChecksums(files)
	directory := filepath.Join(t.TempDir(), "replacement")
	if err := os.Mkdir(directory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range packageFiles {
		if err := os.WriteFile(filepath.Join(directory, name), files[name], 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := ValidatePackage(directory); err == nil ||
		!strings.Contains(err.Error(), "compiled non-held-out definitions") {
		t.Fatalf("self-consistent replacement was not rejected: %v", err)
	}
}
