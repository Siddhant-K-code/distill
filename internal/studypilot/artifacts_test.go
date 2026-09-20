package studypilot

import (
	"bytes"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strings"
	"testing"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

func TestBuildArtifactsDeterministicAcrossInputOrderAndWorkingDirectory(t *testing.T) {
	first, err := BuildArtifacts(nil)
	if err != nil {
		t.Fatal(err)
	}
	ids := make([]string, 0, len(corpusDefinitions()))
	for _, definition := range corpusDefinitions() {
		ids = append(ids, definition.ID)
	}
	for left, right := 0, len(ids)-1; left < right; left, right = left+1, right-1 {
		ids[left], ids[right] = ids[right], ids[left]
	}
	second, err := BuildArtifacts(ids)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, second) {
		t.Fatal("shuffled generation changed artifact bytes")
	}

	original, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	root := testRoot(t)
	if err := os.Chdir(root); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(original) })
	third, err := BuildArtifacts(nil)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(first, third) {
		t.Fatal("working directory changed artifact bytes")
	}
}

func TestPrepareValidateAndNoOverwrite(t *testing.T) {
	root := testRoot(t)
	output := filepath.Join(root, "pilot")
	summary, err := Prepare(output)
	if err != nil {
		t.Fatal(err)
	}
	if summary.CaseCount != 18 || summary.ProviderCalls != 0 || summary.FinalStudyEligible {
		t.Fatalf("unexpected summary: %+v", summary)
	}
	if _, err := Prepare(output); err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("second publication must fail without overwrite: %v", err)
	}
	if _, err := Validate(output); err != nil {
		t.Fatal(err)
	}
}

func TestValidationRejectsTamperingAndFileSetChanges(t *testing.T) {
	tests := map[string]func(string){
		"tampered label": func(root string) {
			path := filepath.Join(root, "labels.jsonl")
			data, _ := os.ReadFile(path)
			data = bytes.Replace(data, []byte(`"phase":"pilot"`), []byte(`"phase":"final"`), 1)
			_ = os.WriteFile(path, data, 0o644)
		},
		"tampered schedule": func(root string) {
			path := filepath.Join(root, "schedule.jsonl")
			data, _ := os.ReadFile(path)
			data = bytes.Replace(data, []byte(`"schedule_index":0`), []byte(`"schedule_index":9`), 1)
			_ = os.WriteFile(path, data, 0o644)
		},
		"noncanonical JSON": func(root string) {
			path := filepath.Join(root, "requests.jsonl")
			data, _ := os.ReadFile(path)
			data = bytes.Replace(data, []byte(`{`), []byte(`{ `), 1)
			_ = os.WriteFile(path, data, 0o644)
		},
		"extra file": func(root string) {
			_ = os.WriteFile(filepath.Join(root, "unexpected"), []byte("x"), 0o644)
		},
		"missing file": func(root string) {
			_ = os.Remove(filepath.Join(root, "cases.jsonl"))
		},
		"schema hash": func(root string) {
			path := filepath.Join(root, "receipt-schema.json")
			data, _ := os.ReadFile(path)
			data[0] = ' '
			_ = os.WriteFile(path, data, 0o644)
		},
		"duplicate case ID": func(root string) {
			path := filepath.Join(root, "cases.jsonl")
			data, _ := os.ReadFile(path)
			data = bytes.Replace(data, []byte(`"case_id":"pilot-case-002"`), []byte(`"case_id":"pilot-case-001"`), 1)
			_ = os.WriteFile(path, data, 0o644)
		},
		"private path needle": func(root string) {
			path := filepath.Join(root, "cases.jsonl")
			data, _ := os.ReadFile(path)
			data = bytes.Replace(data, []byte(`Synthetic public pilot handoff.`), []byte(`/Users/private/work`), 1)
			_ = os.WriteFile(path, data, 0o644)
		},
	}
	names := make([]string, 0, len(tests))
	for name := range tests {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		t.Run(name, func(t *testing.T) {
			root := testRoot(t)
			if _, err := Prepare(filepath.Join(root, "pilot")); err != nil {
				t.Fatal(err)
			}
			output := filepath.Join(root, "pilot")
			tests[name](output)
			refreshIntegrity(t, output)
			if _, err := Validate(output); err == nil {
				t.Fatal("tampered artifact unexpectedly validated")
			}
		})
	}
}

func TestRequestsCarryFrozenStateWithoutGroundTruth(t *testing.T) {
	files, err := BuildArtifacts(nil)
	if err != nil {
		t.Fatal(err)
	}
	requests, err := parseJSONL[RequestRecord](files["requests.jsonl"])
	if err != nil {
		t.Fatal(err)
	}
	if len(requests) != 18 {
		t.Fatalf("got %d requests", len(requests))
	}
	for _, request := range requests {
		if request.State.Content == "" || digestText(request.State.Content) != request.State.SHA256 {
			t.Fatalf("%s lacks hash-bound request state", request.RequestID)
		}
	}
	for _, needle := range []string{`"ground_truth"`, `"expected_baseline_action"`, `"derivation_reasons"`, `"label_seal_digest"`} {
		if bytes.Contains(files["requests.jsonl"], []byte(needle)) {
			t.Fatalf("requests leak sealed field %s", needle)
		}
	}
}

func TestRegisteredTransformationsPreserveBasePairing(t *testing.T) {
	files, err := BuildArtifacts(nil)
	if err != nil {
		t.Fatal(err)
	}
	cases, err := parseJSONL[CaseRecord](files["cases.jsonl"])
	if err != nil {
		t.Fatal(err)
	}
	for _, record := range cases {
		if record.Perturbation.BaseSourceSetDigest != record.BaseSourceSet.SourceSetDigest ||
			record.Perturbation.ResultSourceSetDigest != record.SourceSet.SourceSetDigest {
			t.Fatalf("%s does not bind base/result source sets", record.CaseID)
		}
		switch record.Category {
		case "source_reorder":
			if reflect.DeepEqual(record.BaseSourceSet.ManifestOrder, record.SourceSet.ManifestOrder) {
				t.Fatal("source reorder left manifest order unchanged")
			}
		case "irrelevant_append":
			if len(record.BaseSourceSet.Sources) != len(record.SourceSet.Sources) {
				t.Fatal("irrelevant append added a source instead of appending text")
			}
			if !strings.Contains(record.SourceSet.Sources[0].Content, "Irrelevant control:") {
				t.Fatal("irrelevant append text is absent")
			}
		}
	}
	schedules, err := parseJSONL[ScheduleRecord](files["schedule.jsonl"])
	if err != nil {
		t.Fatal(err)
	}
	for _, schedule := range schedules {
		if schedule.ContextArm != "raw_deterministic_concatenation" ||
			schedule.CompilerDigest != rawCompilerDigest() {
			t.Fatal("schedule misidentifies the raw deterministic context arm")
		}
	}
}

func refreshIntegrity(t *testing.T, directory string) {
	t.Helper()
	files := make(map[string][]byte)
	for _, name := range artifactFileNames {
		if name == "SHA256SUMS" {
			continue
		}
		data, err := os.ReadFile(filepath.Join(directory, name))
		if err == nil {
			files[name] = data
		}
	}
	manifestData, ok := files["pilot-manifest.json"]
	if !ok {
		return
	}
	var manifest Manifest
	if err := decodeStrict(bytes.TrimSuffix(manifestData, []byte{'\n'}), &manifest); err != nil {
		t.Fatal(err)
	}
	for index := range manifest.Files {
		if data, exists := files[manifest.Files[index].Path]; exists {
			manifest.Files[index].Bytes = len(data)
			manifest.Files[index].SHA256 = digestBytes(data)
		}
	}
	updated, err := canonicalJSONFile(manifest)
	if err != nil {
		t.Fatal(err)
	}
	files["pilot-manifest.json"] = updated
	if err := os.WriteFile(filepath.Join(directory, "pilot-manifest.json"), updated, 0o644); err != nil {
		t.Fatal(err)
	}
	checksums := renderChecksums(files)
	if err := os.WriteFile(filepath.Join(directory, "SHA256SUMS"), checksums, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestPolicyRuleTableFixtures(t *testing.T) {
	safe := safeEvidence()
	fixtures := []struct {
		name   string
		status string
		change func(*Evidence)
		want   string
	}{
		{"safe", "valid", func(*Evidence) {}, "accept"},
		{"test failure", "valid", func(e *Evidence) { e.ObservedTestsSupport = "no" }, "reject"},
		{"verifier failure", "valid", func(e *Evidence) { e.VerifierSupport = "no" }, "reject"},
		{"external effect", "valid", func(e *Evidence) { e.ExternalEffectsResolved = "no" }, "reject"},
		{"contradiction", "valid", func(e *Evidence) { e.Contradiction = true }, "reject"},
		{"stale", "valid", func(e *Evidence) { e.Stale = true }, "review"},
		{"cleanup missing", "valid", func(e *Evidence) { e.CleanupComplete = "no" }, "review"},
		{"medium risk", "valid", func(e *Evidence) { e.PatchRisk = "medium" }, "review"},
		{"registered inapplicable", "valid", func(e *Evidence) {
			e.CleanupComplete = "not_applicable"
			e.ExternalEffectsResolved = "not_applicable"
			e.Applicability.Cleanup = false
			e.Applicability.ExternalEffects = false
		}, "accept"},
		{"failed", "failed", func(*Evidence) {}, "review"},
		{"not attempted", "not_attempted", func(*Evidence) {}, "review"},
	}
	for _, fixture := range fixtures {
		t.Run(fixture.name, func(t *testing.T) {
			evidence := safe
			fixture.change(&evidence)
			result, err := EvaluatePolicy(fixture.status, evidence)
			if err != nil {
				t.Fatal(err)
			}
			if result.Result != fixture.want {
				t.Fatalf("got %q, want %q", result.Result, fixture.want)
			}
		})
	}
}

func TestEmbeddedReceiptSchemaDigest(t *testing.T) {
	if got := digestBytes(contextartifact.PilotSchema); got != contextartifact.PilotSchemaSHA256 {
		t.Fatalf("schema digest = %s", got)
	}
}

func testRoot(t *testing.T) string {
	t.Helper()
	build := filepath.Clean(filepath.Join("..", "..", "build"))
	if err := os.MkdirAll(build, 0o755); err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(build, "studypilot-test-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	absolute, err := filepath.Abs(root)
	if err != nil {
		t.Fatal(err)
	}
	return absolute
}
