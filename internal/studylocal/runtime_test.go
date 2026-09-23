package studylocal

import (
	"bytes"
	"context"
	"encoding/base64"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"
)

func TestParseCategoricalPreservesTerminalFailures(t *testing.T) {
	tests := []struct {
		name string
		raw  string
		want string
	}{
		{"valid", `{"answer":"accept"}`, ParseValid},
		{"invalid json", `{`, ParseInvalidJSON},
		{"extra key", `{"answer":"accept","why":"x"}`, ParseSchemaError},
		{"missing key", `{}`, ParseSchemaError},
		{"invalid enum", `{"answer":"maybe"}`, ParseInvalidEnum},
		{"trailing", `{"answer":"accept"} `, ParseTrailingText},
		{"trailing prose", `{"answer":"accept"} Explanation: ok`, ParseTrailingText},
		{"trailing object", `{"answer":"accept"}{"answer":"reject"}`, ParseTrailingText},
		{"non-object trailing", "\"accept\"\n", ParseTrailingText},
		{"duplicate", `{"answer":"accept","answer":"reject"}`, ParseDuplicateKey},
		{"duplicate with trailing", `{"answer":"accept","answer":"reject"} extra`, ParseDuplicateKey},
		{"nested duplicate", `{"answer":{"a":1,"a":2}}`, ParseDuplicateKey},
		{"unsorted citations", `{"answer":"accept|E02,E01"}`, ParseInvalidEnum},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			allowed := []string{"accept", "review", "reject"}
			if test.name == "unsorted citations" {
				allowed = []string{"accept|-", "accept|E01", "accept|E02", "accept|E01,E02"}
			}
			projection := ParseCategorical([]byte(test.raw), allowed)
			if projection.ParseStatus != test.want {
				t.Fatalf("got %s, want %s", projection.ParseStatus, test.want)
			}
			if projection.Confidence != nil || projection.ConfidenceBasis != "unavailable" {
				t.Fatal("parser fabricated confidence")
			}
		})
	}
}

func TestGoPythonProjectionClassifierParity(t *testing.T) {
	python, err := exec.LookPath("python3")
	if err != nil {
		t.Skip("python3 unavailable")
	}
	vectors := []struct {
		Raw     string
		Allowed []string
	}{
		{`{"answer":"accept|E01"}`, []string{"accept|E01"}},
		{"\n" + `{"answer":"accept|E01"}`, []string{"accept|E01"}},
		{`[{"answer":"accept|E01"}]`, []string{"accept|E01"}},
		{`"accept|E01"`, []string{"accept|E01"}},
		{`{"answer":{"a":1,"a":2}}`, []string{"accept|E01"}},
		{`{"answer":"accept|E01"} `, []string{"accept|E01"}},
		{`{"answer":NaN}`, []string{"accept|E01"}},
		{`{"answer":Infinity}`, []string{"accept|E01"}},
		{`{"answer":1e400}`, []string{"accept|E01"}},
		{`{"answer":` + strings.Repeat("9", 400) + `}`, []string{"accept|E01"}},
		{`{"answer":-` + strings.Repeat("9", 400) + `}`, []string{"accept|E01"}},
		{`{"answer":{"x":` + strings.Repeat("9", 400) + `}}`, []string{"accept|E01"}},
		{`{"answer":` + strings.Repeat("[", 300) + `0` + strings.Repeat("]", 300) + `}`, []string{"accept|E01"}},
		{`{"answer":"accept|E01"} Explanation: ok`, []string{"accept|E01"}},
		{`{"answer":"accept|E01"}{"answer":"reject|-"}`, []string{"accept|E01"}},
		{"\"accept|E01\"\n", []string{"accept|E01"}},
		{"null\n", []string{"accept|E01"}},
		{`{"answer":"a","answer":"b"} extra`, []string{"accept|E01"}},
	}
	wire := make([]map[string]any, len(vectors))
	for i, vector := range vectors {
		wire[i] = map[string]any{
			"raw_base64": base64.StdEncoding.EncodeToString([]byte(vector.Raw)),
			"allowed":    vector.Allowed,
		}
	}
	input, _ := canonicalJSON(wire)
	script := filepath.Join("..", "..", "tools", "local-context-control-mlx.py")
	command := exec.Command(python, "-I", "-B", script, "classifier-self-test")
	command.Stdin = bytes.NewReader(input)
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var statuses []string
	if err := strictDecode(bytes.TrimSpace(output), &statuses); err != nil {
		t.Fatal(err)
	}
	for i, vector := range vectors {
		want := ParseCategorical([]byte(vector.Raw), vector.Allowed).ParseStatus
		if statuses[i] != want {
			t.Fatalf("classifier mismatch for %q: Go=%s Python=%s", vector.Raw, want, statuses[i])
		}
	}
}

func TestCategoricalVocabularyCannotLeakAdjudication(t *testing.T) {
	sources := []EvidenceSource{
		{ID: "E01", Path: "a.txt", Bytes: "evidence_id=E01\n", SHA256: DigestBytes([]byte("evidence_id=E01\n"))},
		{ID: "E02", Path: "b.txt", Bytes: "evidence_id=E02\n", SHA256: DigestBytes([]byte("evidence_id=E02\n"))},
	}
	context := Context{ConditionID: "condition", Arm: ArmRaw, Content: "same", ContentSHA256: DigestBytes([]byte("same"))}
	entry := ScheduleEntry{ObservationID: "observation", ConditionID: "condition", Arm: ArmRaw, ContextSHA256: context.ContentSHA256}
	first := Condition{ID: "condition", Sources: sources, Adjudication: Adjudication{Decision: "accept", SupportingEvidenceIDs: []string{"E01"}}}
	second := first
	second.Adjudication = Adjudication{Decision: "reject", SupportingEvidenceIDs: []string{"E02"}}
	generation := GenerationConfig{Sampling: "test"}
	left, err := BuildAdapterRequest(entry, first, context, generation)
	if err != nil {
		t.Fatal(err)
	}
	right, err := BuildAdapterRequest(entry, second, context, generation)
	if err != nil {
		t.Fatal(err)
	}
	leftBytes, _ := canonicalJSON(left.Questions)
	rightBytes, _ := canonicalJSON(right.Questions)
	if !bytes.Equal(leftBytes, rightBytes) {
		t.Fatal("independent adjudication leaked into the model output vocabulary")
	}
	allowed := left.Questions[0].Allowed
	if len(allowed) != 12 ||
		!contains(allowed, "accept|-") || !contains(allowed, "accept|E01,E02") ||
		!contains(allowed, "review|E02") || !contains(allowed, "reject|E01") {
		t.Fatalf("categorical vocabulary does not cover all decision/citation combinations: %v", allowed)
	}
}

func TestRuntimeManifestRejectsBytecodeContamination(t *testing.T) {
	manifest := RuntimeManifest{
		SchemaVersion: RuntimeManifestSchema, PythonVersion: "3.13.15",
		DistributionCount: 34, Distributions: make([]RuntimeDistribution, 34),
		FileCount: 100, TotalBytes: 1000, FullTreeSHA256: string(make([]byte, 64)),
		BytecodeFiles: 1,
	}
	projection := manifest
	projection.ManifestSHA256 = ""
	manifest.ManifestSHA256, _ = DigestDomain("runtime-manifest", projection)
	if _, _, err := validateRuntimeManifestValue(manifest); err == nil {
		t.Fatal("runtime with unexpected __pycache__/.pyc was accepted")
	}
}

func TestTrustedTreeVerificationRejectsMutationAndWritableAncestor(t *testing.T) {
	repositoryRoot, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	root, err := os.MkdirTemp(repositoryRoot, ".trusted-path-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = os.RemoveAll(root) }()
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	directory := filepath.Join(root, "lib")
	if err := os.Mkdir(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "value.txt")
	if err := os.WriteFile(file, []byte("trusted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	manifest := []byte("D\tlib\t755\nF\tlib/value.txt\t644\t8\t7bd39a7cbcf687fd60f819645b8bcaf731a9f19cb102484a7b84530516d7e8b8\n")
	if err := verifyTreeManifest(root, manifest, 1, 1, 0, true); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(file, []byte("mutated\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyTreeManifest(root, manifest, 1, 1, 0, true); err == nil {
		t.Fatal("mutated tree passed trusted verification")
	}
	writable := filepath.Join(root, "writable")
	if err := os.Mkdir(writable, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(writable, 0o777); err != nil {
		t.Fatal(err)
	}
	candidate := filepath.Join(writable, "candidate")
	if err := os.WriteFile(candidate, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(writable, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := requireTrustedPathAncestors(candidate); err != nil {
		t.Fatalf("trusted positive-control path was rejected: %v", err)
	}
	if err := os.Chmod(writable, 0o777); err != nil {
		t.Fatal(err)
	}
	if err := requireTrustedPathAncestors(candidate); err == nil ||
		!strings.Contains(err.Error(), "group/world writable") ||
		!strings.Contains(err.Error(), writable) {
		t.Fatalf("group/world-writable path ancestor was not specifically rejected: %v", err)
	}
}

func TestTrustedTreeVerificationPinsSymlinkTargetsAndModes(t *testing.T) {
	resolvedTemp, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(resolvedTemp, "tree")
	lib := filepath.Join(root, "lib")
	if err := os.MkdirAll(lib, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(lib, "value.txt")
	if err := os.WriteFile(target, []byte("trusted\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("value.txt", filepath.Join(lib, "value-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("lib", filepath.Join(root, "lib-link")); err != nil {
		t.Fatal(err)
	}
	manifest := []byte(
		"D\tlib\t755\n" +
			"F\tlib/value.txt\t644\t8\t7bd39a7cbcf687fd60f819645b8bcaf731a9f19cb102484a7b84530516d7e8b8\n" +
			"L\tlib/value-link\t644\tvalue.txt\n" +
			"L\tlib-link\t755\tlib\n",
	)
	if err := verifyTreeManifest(root, manifest, 1, 1, 2, true); err != nil {
		t.Fatal(err)
	}
	if err := os.Remove(filepath.Join(lib, "value-link")); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("missing.txt", filepath.Join(lib, "value-link")); err != nil {
		t.Fatal(err)
	}
	if err := verifyTreeManifest(root, manifest, 1, 1, 2, true); err == nil {
		t.Fatal("changed symlink target was accepted")
	}

	modeRoot := filepath.Join(resolvedTemp, "mode-tree")
	if err := os.Mkdir(modeRoot, 0o700); err != nil {
		t.Fatal(err)
	}
	externalTarget := filepath.Join(resolvedTemp, "external-target")
	if err := os.WriteFile(externalTarget, []byte("external\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(externalTarget, filepath.Join(modeRoot, "external-link")); err != nil {
		t.Fatal(err)
	}
	modeManifest := []byte("L\texternal-link\t644\t" + externalTarget + "\n")
	if err := verifyTreeManifest(modeRoot, modeManifest, 0, 0, 1, false); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(externalTarget, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyTreeManifest(modeRoot, modeManifest, 0, 0, 1, false); err == nil ||
		!strings.Contains(err.Error(), "symlink target mode") {
		t.Fatalf("changed symlink target mode was not rejected: %v", err)
	}
}

type fakeSystemProbe struct {
	outputs map[string]string
	pid     int
	disk    int64
}

func (probe fakeSystemProbe) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	key := name
	for _, arg := range args {
		key += "\x00" + arg
	}
	value, ok := probe.outputs[key]
	if !ok {
		return nil, fmt.Errorf("unexpected command %q", key)
	}
	return []byte(value), nil
}

func (probe fakeSystemProbe) StatFS(string) (int64, error) { return probe.disk, nil }
func (probe fakeSystemProbe) PID() int                     { return probe.pid }

func cleanPreflightProbe() fakeSystemProbe {
	return fakeSystemProbe{
		pid: 100, disk: 100 << 30,
		outputs: map[string]string{
			"/usr/bin/sw_vers\x00-productVersion":                "27.0\n",
			"/usr/bin/sw_vers\x00-buildVersion":                  "26A428\n",
			"/usr/bin/uname\x00-m":                               "arm64\n",
			"/usr/sbin/sysctl\x00-n\x00machdep.cpu.brand_string": "Apple M5 Pro\n",
			"/usr/sbin/sysctl\x00-n\x00hw.memsize":               "25769803776\n",
			"/usr/bin/vm_stat":                                   "Mach Virtual Memory Statistics: (page size of 16384 bytes)\nPages free: 500000.\nPages inactive: 100000.\nPages speculative: 1000.\nPages purgeable: 1000.\n",
			"/usr/bin/memory_pressure\x00-Q":                     "System-wide memory free percentage: 45%\n",
			"/usr/sbin/sysctl\x00-n\x00vm.swapusage":             "total = 16384.00M  used = 8192.00M  free = 8192.00M\n",
			"/usr/bin/pmset\x00-g\x00batt":                       "Now drawing from 'AC Power'\n",
			"/usr/bin/xcrun\x00xctrace\x00version":               "xctrace version 27.0 (27A266a)\n",
			"/usr/bin/sandbox-exec\x00-p\x00" + SandboxPolicy + "\x00/usr/bin/python3\x00-I\x00-B\x00-c\x00import socket\ns = socket.socket()\ntry:\n    s.connect((\"127.0.0.1\", 9))\nexcept PermissionError:\n    raise SystemExit(0)\nexcept OSError:\n    raise SystemExit(2)\nraise SystemExit(1)": "",
			"/bin/ps\x00-axo\x00pid=,ppid=,rss=,comm=": "1 0 1024 launchd\n100 1 1024 distill-local-context-control\n101 100 1024 helper\n",
		},
	}
}

func TestPreflightRejectsUnrelatedHeavyProcess(t *testing.T) {
	probe := cleanPreflightProbe()
	probe.outputs["/bin/ps\x00-axo\x00pid=,ppid=,rss=,comm="] += "900 1 3200000 unrelated-heavy\n"
	protocol, err := BuildProtocol()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collectHostPreflight(context.Background(), t.TempDir(), protocol.Isolation, probe)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Eligible || !contains(snapshot.Failures, "unrelated_high_memory_process") ||
		len(snapshot.HeavyProcesses) != 1 || snapshot.HeavyProcesses[0].PID != 900 {
		t.Fatalf("heavy process did not fail closed: %+v", snapshot)
	}
}

func TestPreflightExcludesOwnAncestorChain(t *testing.T) {
	probe := cleanPreflightProbe()
	probe.outputs["/bin/ps\x00-axo\x00pid=,ppid=,rss=,comm="] =
		"1 0 1024 launchd\n50 1 3200000 copilot-parent\n100 50 1024 distill-local-context-control\n"
	protocol, err := BuildProtocol()
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := collectHostPreflight(context.Background(), t.TempDir(), protocol.Isolation, probe)
	if err != nil {
		t.Fatal(err)
	}
	if !snapshot.Eligible || len(snapshot.HeavyProcesses) != 0 {
		t.Fatalf("own ancestor was treated as unrelated: %+v", snapshot)
	}
}

type fakeAdapter struct {
	malformedObservation string
	attempts             map[string]int
}

func (adapter *fakeAdapter) Ready() AdapterReady         { return AdapterReady{ModelLoaded: true} }
func (adapter *fakeAdapter) PID() int                    { return os.Getpid() }
func (adapter *fakeAdapter) Close(context.Context) error { return nil }

func (adapter *fakeAdapter) Generate(_ context.Context, request AdapterRequest) (AdapterResponse, []byte, []byte, error) {
	if adapter.attempts == nil {
		adapter.attempts = map[string]int{}
	}
	adapter.attempts[request.ObservationID]++
	response := AdapterResponse{SchemaVersion: AdapterResponseSchema, ObservationID: request.ObservationID}
	for _, question := range request.Questions {
		answer := question.Allowed[0]
		raw := []byte(fmt.Sprintf(`{"answer":%q}`, answer))
		if request.ObservationID == adapter.malformedObservation && question.ID == "decision_with_evidence" {
			raw = []byte(`{"answer":"accept","answer":"reject"}`)
		}
		response.Questions = append(response.Questions, AdapterQuestionResult{
			QuestionID: question.ID, RawResponseBase64: base64.StdEncoding.EncodeToString(raw),
			RawResponseSHA256: DigestBytes(raw), PromptTokenIDs: []int{1, 2},
			GeneratedTokenIDs: []int{3}, PromptTokens: 2, GeneratedTokens: 1,
			Projection: ParseCategorical(raw, question.Allowed),
			Timing:     AdapterTiming{TokenizeNanoseconds: 1, TTFTNanoseconds: 1, DecodeNanoseconds: 1, TotalNanoseconds: 3},
			Memory:     AdapterMemory{ActiveBytes: 1, CacheBytes: 1, PeakBytes: 1},
		})
	}
	envelope, _ := canonicalJSON(response)
	return response, envelope, []byte{}, nil
}

func TestFakeAdapterRunsExactlyOneAttemptPerScheduleEntry(t *testing.T) {
	packageDirectory := filepath.Join(t.TempDir(), "package")
	if _, err := Prepare(packageDirectory); err != nil {
		t.Fatal(err)
	}
	pkg, err := ValidatePackage(packageDirectory)
	if err != nil {
		t.Fatal(err)
	}
	runDirectory, err := normalizeExistingPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(runDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(runDirectory, "calls"), 0o700); err != nil {
		t.Fatal(err)
	}
	authorizationRoot, err := normalizeExistingPath(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	authorizationDirectory := filepath.Join(authorizationRoot, "authorization")
	adapter := &fakeAdapter{malformedObservation: pkg.Schedule[0].ObservationID}
	monitor := func(_ context.Context, _ int, _ int64, _ string, _ IsolationLimits, ready chan<- error) (ObservationResourceSummary, error) {
		ready <- nil
		return ObservationResourceSummary{
			SampleCount: 2, MinimumVMAvailablePct: 50, MinimumPressureFreePct: 50,
			MinimumDiskFreeBytes:    100 << 30,
			RSSMeasurementSemantics: "sampled process RSS every two seconds; not a true high-water mark",
			Samples: []RuntimeSample{
				{CollectedAt: time.Now().UTC(), VMAvailablePercent: 50, MemoryPressureFreePercent: 50, DiskFreeBytes: 100 << 30},
				{CollectedAt: time.Now().UTC(), VMAvailablePercent: 50, MemoryPressureFreePercent: 50, DiskFreeBytes: 100 << 30},
			},
		}, nil
	}
	binding, authorizationFiles := fakeExecutionBinding(pkg, authorizationDirectory, runDirectory)
	completion, err := executeSchedule(context.Background(), pkg, binding, runDirectory, adapter, monitor)
	if err != nil {
		t.Fatal(err)
	}
	if completion.ObservationsRecorded != ObservationsPerModel || completion.Malformed != 1 {
		t.Fatalf("unexpected completion: %+v", completion)
	}
	for _, entry := range pkg.Schedule {
		if adapter.attempts[entry.ObservationID] != 1 {
			t.Fatalf("%s attempted %d times", entry.ObservationID, adapter.attempts[entry.ObservationID])
		}
	}
	completion.CompletedAt = time.Now().UTC()
	completion.RuntimeReverified = true
	completion.ModelReverified = true
	completionBytes, _ := canonicalJSONFile(completion)
	runManifest := RunManifest{
		SchemaVersion: SchemaVersion + "/run-manifest", StartedAt: time.Now().UTC(),
		ImplementationCommit: binding.Authorization.ImplementationCommit,
		AuthorizationSHA256:  binding.Authorization.AuthorizationSHA256, PackageManifestSHA256: DigestBytes(pkg.Files["package-manifest.json"]),
		ScheduleSHA256: pkg.Manifest.ScheduleSHA256, ObservationsExpected: len(pkg.Schedule),
		AttemptsPerObservation: 1, ModelID: TargetModelID, ModelRevision: TargetModelRevision,
		AttemptNonce: binding.Authorization.AttemptNonce,
	}
	_, attempt, err := validateAttemptLedgerBytes(authorizationFiles["attempt-ledger.jsonl"], binding.Authorization, true)
	if err != nil {
		t.Fatal(err)
	}
	runManifest.AttemptLedgerEntrySHA256 = attempt.EntrySHA256
	runManifestBytes, _ := canonicalJSONFile(runManifest)
	for name, data := range map[string][]byte{
		"authorization.json":    authorizationFiles["authorization.json"],
		"host-preflight.json":   authorizationFiles["host-preflight.json"],
		"model-manifest.json":   authorizationFiles["model-manifest.json"],
		"runtime-manifest.json": authorizationFiles["runtime-manifest.json"], "run-manifest.json": runManifestBytes,
		"completion.json": completionBytes,
	} {
		if err := os.WriteFile(filepath.Join(runDirectory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.Mkdir(filepath.Join(runDirectory, "adapter-workspace"), 0o700); err != nil {
		t.Fatal(err)
	}
	if err := writePrivateDirectory(authorizationDirectory, authorizationFilesListForTest(), authorizationFiles); err != nil {
		t.Fatal(err)
	}
	summary, err := SummarizeResults(packageDirectory, authorizationDirectory, runDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if summary.Measured.Observations != ObservationsPerModel || summary.Measured.Malformed != 1 ||
		summary.Repeatability.RepeatedConditionArmPairs != 48 {
		t.Fatalf("unexpected verified result summary: %+v", summary)
	}
	target := pkg.Schedule[1]
	callDirectory := filepath.Join(runDirectory, "calls", fmt.Sprintf("%03d-%s", target.Index, target.ObservationID))
	receiptPath := filepath.Join(callDirectory, "receipt.json")
	var receipt ExecutionReceipt
	receiptBytes, err := os.ReadFile(receiptPath)
	if err != nil || strictDecode(receiptBytes, &receipt) != nil {
		t.Fatal("read receipt for stdout-binding mutation")
	}
	replacement := []byte(`{"answer":"reject|-"}`)
	rawPath := filepath.Join(callDirectory, "raw-decision_with_evidence.bin")
	if err := os.WriteFile(rawPath, replacement, 0o600); err != nil {
		t.Fatal(err)
	}
	receipt.Questions[0].RawResponseSHA256 = DigestBytes(replacement)
	receipt.Questions[0].RawResponseBytes = len(replacement)
	receipt.Questions[0].Projection = ParseCategorical(replacement, decisionEvidenceEnums(pkg.Corpus.Conditions[0].Sources))
	receipt.ReceiptSHA256, _ = DigestDomain("execution-receipt", receiptProjection(receipt))
	mutatedReceipt, _ := canonicalJSONFile(receipt)
	if err := os.WriteFile(receiptPath, mutatedReceipt, 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := validateObservation(callDirectory, target, pkg); err == nil {
		t.Fatal("receipt/raw substitution that disagrees with adapter stdout was accepted")
	}
}

func fakeExecutionBinding(pkg Package, authorizationDirectory, runDirectory string) (executionBinding, map[string][]byte) {
	modelManifest := ModelManifest{
		SchemaVersion: ModelManifestSchema, OfficialModelID: TargetModelID,
		OfficialRevision: TargetModelRevision, Format: TargetModelFormat,
		QuantizationMode: "affine", QuantizationBits: 4, QuantizationGroupSize: 64,
		ConverterPackage: ConverterPackage, ConverterVersion: ConverterVersion,
		ConverterRevision: ConverterRevision, ConversionManifestSHA256: ConversionManifestSHA256,
		FileManifestSHA256: ModelFileManifestSHA256,
		Files:              append([]ModelFile(nil), expectedModelFiles...), FileCount: ModelFileCount,
		TotalBytes: ModelTotalBytes, ArtifactSHA256: TargetModelArtifactSHA256,
	}
	modelManifest.ManifestSHA256, _ = DigestDomain("model-manifest", modelManifestProjection(modelManifest))
	modelBytes, _ := canonicalJSONFile(modelManifest)
	distributionNames := make([]string, 0, len(expectedRuntimeDistributions))
	for name := range expectedRuntimeDistributions {
		distributionNames = append(distributionNames, name)
	}
	sort.Strings(distributionNames)
	runtimeManifest := RuntimeManifest{
		SchemaVersion: RuntimeManifestSchema, PythonVersion: "3.13.15",
		DistributionCount: len(distributionNames), FileCount: 5532, TotalBytes: 335_118_336,
		FullTreeSHA256:     RuntimeVenvManifestSHA256,
		VenvDirectoryCount: 831, VenvSymlinkCount: 3,
		BaseFileCount: 1942, BaseDirectoryCount: 191, BaseSymlinkCount: 8,
		BaseTreeSHA256: BasePythonManifestSHA256, BasePythonSHA256: BasePythonBinarySHA256,
		PackageClosureSHA256:    RuntimeClosureSHA256,
		RequirementsInputSHA256: "4f6fad6a9e96d8ee1efdf59f1bd423c4425647c6d72db83ab9d335f5fea7c21b",
		RequirementsSHA256:      RuntimeRequirementsSHA256,
		WheelVerificationSHA256: RuntimeWheelVerificationSHA256,
		SitePackagesFileCount:   1, SitePackagesTreeSHA256: strings.Repeat("a", 64),
	}
	for _, name := range distributionNames {
		runtimeManifest.Distributions = append(runtimeManifest.Distributions, RuntimeDistribution{Name: name, Version: expectedRuntimeDistributions[name]})
	}
	runtimeManifest.ManifestSHA256, _ = DigestDomain("runtime-manifest", func() RuntimeManifest {
		value := runtimeManifest
		value.ManifestSHA256 = ""
		return value
	}())
	runtimeBytes, _ := canonicalJSONFile(runtimeManifest)
	authorization := ExecutionAuthorization{
		SchemaVersion:        SchemaVersion + "/execution-authorization",
		Acknowledgement:      AuthorizationAcknowledgement,
		ImplementationCommit: "0000000000000000000000000000000000000000",
		ImplementationTree:   "0000000000000000000000000000000000000000",
		MainRef:              "origin/main",
		BuildPackage:         "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control",
		BuildGoVersion:       "go1.24.0", BuildVCSRevision: "0000000000000000000000000000000000000000",
		PackageManifestSHA256: DigestBytes(pkg.Files["package-manifest.json"]),
		ScheduleSHA256:        pkg.Manifest.ScheduleSHA256, ModelManifestSHA256: modelManifest.ManifestSHA256,
		ModelArtifactSHA256: TargetModelArtifactSHA256, RuntimeManifestSHA256: runtimeManifest.ManifestSHA256,
		RuntimeFullTreeSHA256: runtimeManifest.FullTreeSHA256, Observations: ObservationsPerModel,
		AuthorizationDirectorySHA256: DigestBytes([]byte(authorizationDirectory)),
		OutputNamespaceSHA256:        DigestBytes([]byte(runDirectory)),
		AttemptNonce:                 strings.Repeat("1", 64),
	}
	genesis := AttemptLedgerGenesis{
		SchemaVersion:                SchemaVersion + "/attempt-ledger-genesis",
		AttemptNonce:                 authorization.AttemptNonce,
		AuthorizationDirectorySHA256: authorization.AuthorizationDirectorySHA256,
		OutputNamespaceSHA256:        authorization.OutputNamespaceSHA256,
		ImplementationCommit:         authorization.ImplementationCommit, ImplementationTree: authorization.ImplementationTree,
		PackageManifestSHA256: authorization.PackageManifestSHA256, ScheduleSHA256: authorization.ScheduleSHA256,
		ModelManifestSHA256: authorization.ModelManifestSHA256, RuntimeManifestSHA256: authorization.RuntimeManifestSHA256,
	}
	genesis.GenesisSHA256, _ = DigestDomain("attempt-ledger-genesis", genesisProjection(genesis))
	authorization.LedgerGenesisSHA256 = genesis.GenesisSHA256
	authorization.AuthorizationSHA256, _ = DigestDomain("execution-authorization", authorizationProjection(authorization))
	authorizationBytes, _ := canonicalJSONFile(authorization)
	attempt := AttemptLedgerEntry{
		SchemaVersion: SchemaVersion + "/attempt-ledger-entry", Attempt: 1,
		AttemptNonce: authorization.AttemptNonce, GenesisSHA256: genesis.GenesisSHA256,
		AuthorizationSHA256:   authorization.AuthorizationSHA256,
		OutputNamespaceSHA256: authorization.OutputNamespaceSHA256, StartedAt: time.Now().UTC(),
	}
	attempt.EntrySHA256, _ = DigestDomain("attempt-ledger-entry", attemptProjection(attempt))
	genesisBytes, _ := canonicalJSON(genesis)
	attemptBytes, _ := canonicalJSON(attempt)
	hostBytes, _ := canonicalJSONFile(HostSnapshot{SchemaVersion: SchemaVersion + "/host-preflight", Eligible: true})
	files := map[string][]byte{
		"authorization.json": authorizationBytes, "host-preflight.json": hostBytes,
		"model-manifest.json": modelBytes, "runtime-manifest.json": runtimeBytes,
		"attempt-ledger.jsonl": append(append(append([]byte(nil), genesisBytes...), '\n'), append(attemptBytes, '\n')...),
	}
	files["SHA256SUMS"] = renderChecksums(map[string][]byte{
		"authorization.json": authorizationBytes, "host-preflight.json": hostBytes,
		"model-manifest.json": modelBytes, "runtime-manifest.json": runtimeBytes,
	})
	return executionBinding{
		Authorization: authorization, Attempt: attempt, ModelManifest: modelManifest, RuntimeManifest: runtimeManifest,
	}, files
}

func authorizationFilesListForTest() []string {
	return append([]string(nil), authorizationFiles...)
}

func TestAdapterEnvironmentIsOfflineAndNoBytecode(t *testing.T) {
	environment := isolatedEnvironment("/private/tmp/local-control")
	for _, expected := range []string{
		"HF_HUB_OFFLINE=1", "TRANSFORMERS_OFFLINE=1", "HF_DATASETS_OFFLINE=1",
		"PYTHONSAFEPATH=1", "PYTHONDONTWRITEBYTECODE=1", "HOME=/dev/null",
	} {
		if !contains(environment, expected) {
			t.Fatalf("missing %s", expected)
		}
	}
}

func TestModelManifestRequiresExactEightFiles(t *testing.T) {
	root := t.TempDir()
	if _, err := BuildModelManifest(root); err == nil {
		t.Fatal("empty model directory was accepted")
	}
}

func TestFrozenModelContractKnownAnswer(t *testing.T) {
	data, err := frozenModelContractJSON(expectedModelFiles)
	if err != nil {
		t.Fatal(err)
	}
	if got := DigestBytes(data); got != TargetModelArtifactSHA256 {
		t.Fatalf("legacy model contract digest = %s, want %s", got, TargetModelArtifactSHA256)
	}
}

func TestRuntimeManifestRetiredIdentityCannotAuthorize(t *testing.T) {
	protocol, err := BuildProtocol()
	if err != nil {
		t.Fatal(err)
	}
	if protocol.RuntimeStatus == "" || protocol.RetiredRuntimeSHA256 != RuntimeIdentitySHA256 {
		t.Fatal("retired runtime custody event is not frozen")
	}
}

func TestBuildProvenanceRejectsWrongRevisionAndDirtyBuild(t *testing.T) {
	commit := strings.Repeat("a", 40)
	base := &debug.BuildInfo{
		Path:      "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control",
		GoVersion: "go1.24.0",
		Settings: []debug.BuildSetting{
			{Key: "vcs.revision", Value: commit},
			{Key: "vcs.modified", Value: "false"},
		},
	}
	if _, err := validateExecutableBuildInfo(base, commit); err != nil {
		t.Fatal(err)
	}
	wrong := *base
	wrong.Settings = append([]debug.BuildSetting(nil), base.Settings...)
	wrong.Settings[0].Value = strings.Repeat("b", 40)
	if _, err := validateExecutableBuildInfo(&wrong, commit); err == nil {
		t.Fatal("wrong build revision was accepted")
	}
	dirty := *base
	dirty.Settings = append([]debug.BuildSetting(nil), base.Settings...)
	dirty.Settings[1].Value = "true"
	if _, err := validateExecutableBuildInfo(&dirty, commit); err == nil {
		t.Fatal("dirty executable build was accepted")
	}
}

func TestAuthorizationLedgerIsSingleUseAndPathBound(t *testing.T) {
	packageDirectory := filepath.Join(t.TempDir(), "package")
	if _, err := Prepare(packageDirectory); err != nil {
		t.Fatal(err)
	}
	pkg, err := ValidatePackage(packageDirectory)
	if err != nil {
		t.Fatal(err)
	}
	root := t.TempDir()
	authorizationDirectory := filepath.Join(root, "authorization")
	runDirectory := filepath.Join(root, "run")
	binding, files := fakeExecutionBinding(pkg, authorizationDirectory, runDirectory)
	lines := bytes.Split(files["attempt-ledger.jsonl"], []byte{'\n'})
	files["attempt-ledger.jsonl"] = append(append([]byte(nil), lines[0]...), '\n')
	if err := writePrivateDirectory(authorizationDirectory, authorizationFilesListForTest(), files); err != nil {
		t.Fatal(err)
	}
	loaded, _, _, _, _, err := LoadAuthorization(authorizationDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := consumeAuthorization(authorizationDirectory, loaded); err != nil {
		t.Fatal(err)
	}
	if _, err := consumeAuthorization(authorizationDirectory, loaded); err == nil {
		t.Fatal("authorization was consumed twice")
	}
	copyDirectory := filepath.Join(root, "authorization-copy")
	if err := os.Mkdir(copyDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	for _, name := range authorizationFiles {
		data, err := os.ReadFile(filepath.Join(authorizationDirectory, name))
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(copyDirectory, name), data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if _, _, _, _, _, err := LoadAuthorization(copyDirectory); err == nil {
		t.Fatal("copied authorization reset the bound attempt universe")
	}
	if binding.Authorization.AuthorizationDirectorySHA256 != DigestBytes([]byte(authorizationDirectory)) {
		t.Fatal("authorization directory binding drifted")
	}
}

func TestOpenAuthorizationLedgerRejectsPathReplacement(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "attempt-ledger.jsonl")
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	file, err := os.OpenFile(path, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = file.Close() }()
	original := filepath.Join(directory, "original.jsonl")
	if err := os.Rename(path, original); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{}\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := verifyOpenLedgerPath(file, path); err == nil {
		t.Fatal("replaced ledger directory entry was accepted")
	}
}

func TestAnalysisWeightsBasesEqually(t *testing.T) {
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
	conditions := map[string]Condition{}
	for _, condition := range corpus.Conditions {
		conditions[condition.ID] = condition
	}
	receipts := make([]ExecutionReceipt, 0, len(schedule))
	for _, entry := range schedule {
		truth := conditions[entry.ConditionID].Adjudication.Decision
		answer := "accept"
		if answer == truth {
			answer = "reject"
		}
		if entry.Arm == ArmDistillLock && entry.Dataset == "distill" {
			answer = truth
		}
		receipts = append(receipts, ExecutionReceipt{
			ObservationID: entry.ObservationID, ConditionID: entry.ConditionID,
			BaseID: entry.BaseID, Dataset: entry.Dataset, Arm: entry.Arm,
			Replicate: entry.Replicate, Status: "valid",
			ResourceSummary: ObservationResourceSummary{MinimumVMAvailablePct: 50, MinimumPressureFreePct: 50},
			Questions: []QuestionReceipt{{
				QuestionID: "decision_with_evidence", Projection: Projection{
					Answer: stringPointer(answer + "|-"), ConfidenceBasis: "unavailable", ParseStatus: ParseValid,
				},
			}},
		})
	}

	summary, err := analyzeResults(validatedRun{
		Package:  Package{Corpus: corpus, Protocol: protocol, Contexts: contexts, Schedule: schedule},
		Receipts: receipts,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.Inferred.EqualWeightMeanCorrectnessDelta != 0.5 {
		t.Fatalf("condition counts influenced base weighting: got %f", summary.Inferred.EqualWeightMeanCorrectnessDelta)
	}
}

func TestCitationJaccardDenominators(t *testing.T) {
	valid := []observationScore{
		{receipt: ExecutionReceipt{ConditionID: "c", Arm: ArmRaw, Replicate: 1, Status: "valid"}},
		{receipt: ExecutionReceipt{ConditionID: "c", Arm: ArmRaw, Replicate: 2, Status: "valid"}},
		{receipt: ExecutionReceipt{ConditionID: "c", Arm: ArmRaw, Replicate: 3, Status: "valid"}},
	}
	metrics := repeatMetrics(valid)
	if metrics.CitationTriplesEvaluated != 1 || metrics.CitationNonemptyUnionTriples != 0 ||
		metrics.MeanEvidenceJaccard == nil || *metrics.MeanEvidenceJaccard != 1 ||
		metrics.MeanNonemptyEvidenceJaccard != nil {
		t.Fatalf("unexpected empty-citation convention: %+v", metrics)
	}
	invalid := append([]observationScore(nil), valid...)
	invalid[1].receipt.Status = "malformed"
	metrics = repeatMetrics(invalid)
	if metrics.CitationTriplesEvaluated != 0 || metrics.MeanEvidenceJaccard != nil ||
		metrics.MeanNonemptyEvidenceJaccard != nil {
		t.Fatalf("malformed triple entered citation denominator: %+v", metrics)
	}
}

func stringPointer(value string) *string {
	return &value
}
