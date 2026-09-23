package studylocalv2

import (
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"testing"
	"time"
)

func requirePython3(t *testing.T) string {
	t.Helper()
	python, err := exec.LookPath("python3")
	if err == nil {
		return python
	}
	if os.Getenv("DISTILL_REQUIRE_PYTHON") != "" {
		t.Fatalf("python3 is required for cross-language conformance: %v", err)
	}
	t.Skip("python3 unavailable")
	return ""
}

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
	python := requirePython3(t)
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
	script := filepath.Join("..", "..", "tools", "local-context-control-mlx-v2.py")
	command := exec.Command(python, "-I", "-B", "-S", script, "classifier-self-test")
	command.Stdin = bytes.NewReader(append(input, '\n'))
	output, err := command.Output()
	if err != nil {
		t.Fatal(err)
	}
	var statuses []string
	if err := strictJSONFileDecode(output, &statuses); err != nil {
		t.Fatal(err)
	}
	for i, vector := range vectors {
		want := ParseCategorical([]byte(vector.Raw), vector.Allowed).ParseStatus
		if statuses[i] != want {
			t.Fatalf("classifier mismatch for %q: Go=%s Python=%s", vector.Raw, want, statuses[i])
		}
	}
}

func TestGoPythonFramingConformance(t *testing.T) {
	python := requirePython3(t)
	request := framingConformanceRequest()
	goResponse, err := evaluateFramingConformance(request)
	if err != nil {
		t.Fatal(err)
	}
	if goResponse.ConformanceSHA256 != FramingConformanceKnownAnswerSHA256 {
		t.Fatalf("framing conformance digest = %s, want %s", goResponse.ConformanceSHA256, FramingConformanceKnownAnswerSHA256)
	}
	input, err := canonicalJSONFile(request)
	if err != nil {
		t.Fatal(err)
	}
	script := filepath.Join("..", "..", "tools", "local-context-control-mlx-v2.py")
	command := exec.Command(python, "-I", "-B", "-S", script, "framing-conformance")
	command.Stdin = bytes.NewReader(input)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("python framing conformance: %v: %s", err, output)
	}
	var pythonResponse FramingConformanceResponse
	if err := strictJSONObjectFileDecode(output, &pythonResponse); err != nil {
		t.Fatal(err)
	}
	if pythonResponse.Implementation != "python" || pythonResponse.ModelRuntimeImported ||
		pythonResponse.RequestSHA256 != goResponse.RequestSHA256 ||
		pythonResponse.ConformanceSHA256 != goResponse.ConformanceSHA256 ||
		fmt.Sprint(pythonResponse.Results) != fmt.Sprint(goResponse.Results) {
		t.Fatalf("Go/Python framing mismatch:\nGo: %#v\nPython: %#v", goResponse, pythonResponse)
	}
}

func TestAdapterCheckCannotImportOrLoadModelRuntime(t *testing.T) {
	python := requirePython3(t)
	script := filepath.Join("..", "..", "tools", "local-context-control-mlx-v2.py")
	check := `
import ast, pathlib, sys
source = pathlib.Path(sys.argv[1]).read_text(encoding="utf-8")
tree = ast.parse(source)
functions = {node.name: node for node in tree.body if isinstance(node, (ast.FunctionDef, ast.AsyncFunctionDef))}
target = functions["adapter_check"]
for node in ast.walk(target):
    if isinstance(node, (ast.Import, ast.ImportFrom)):
        raise SystemExit("adapter_check contains an import")
    if isinstance(node, ast.Call):
        name = node.func.id if isinstance(node.func, ast.Name) else node.func.attr if isinstance(node.func, ast.Attribute) else ""
        if name == "load":
            raise SystemExit("adapter_check calls load")
for node in tree.body:
    if isinstance(node, (ast.Import, ast.ImportFrom)):
        names = [alias.name for alias in node.names]
        if any(name == "mlx" or name.startswith("mlx.") or name == "mlx_lm" or name.startswith("mlx_lm.") for name in names):
            raise SystemExit("model runtime imported at module scope")
`
	command := exec.Command(python, "-I", "-B", "-S", "-c", check, script)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("adapter-check static model-load proof: %v: %s", err, output)
	}
	temporary := t.TempDir()
	request := framingConformanceRequest()
	input, err := canonicalJSONFile(request)
	if err != nil {
		t.Fatal(err)
	}
	command = exec.Command(python, "-I", "-B", "-S", script, "framing-conformance")
	command.Env = []string{
		"PATH=/usr/bin:/bin", "HOME=/dev/null", "LC_ALL=C", "LANG=C",
		"TMPDIR=" + temporary, "PYTHONDONTWRITEBYTECODE=1", "PYTHONNOUSERSITE=1",
	}
	command.Stdin = bytes.NewReader(input)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("zero-model conformance command: %v: %s", err, output)
	}
	entries, err := os.ReadDir(temporary)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("zero-model command created cache or bytecode: %v", entries)
	}
}

func TestAdapterCheckFailureCreatesNoAuthorizationOrLedger(t *testing.T) {
	root := t.TempDir()
	checkDirectory := filepath.Join(root, "check")
	authorizationDirectory := filepath.Join(root, "authorization")
	_, err := RunAdapterCheck(context.Background(), AdapterCheckOptions{
		PackageDirectory: filepath.Join(root, "missing-package"),
		OutputDirectory:  checkDirectory, RunOutputDirectory: filepath.Join(root, "run"),
	})
	if err == nil {
		t.Fatal("invalid adapter-check unexpectedly succeeded")
	}
	for _, path := range []string{
		checkDirectory, authorizationDirectory,
		filepath.Join(authorizationDirectory, "attempt-ledger.jsonl"),
	} {
		if _, statErr := os.Lstat(path); !errors.Is(statErr, os.ErrNotExist) {
			t.Fatalf("adapter-check failure created %s", path)
		}
	}
}

func TestAdapterCheckReceiptRejectsBoundIdentityDrift(t *testing.T) {
	now := time.Now().UTC()
	request := AdapterCheckRequest{
		SchemaVersion: AdapterCheckRequestSchema, StartedAt: now, ExpiresAt: now.Add(time.Hour),
		ImplementationCommit: strings.Repeat("a", 40), ImplementationTree: strings.Repeat("b", 40),
		BuildPackage:   "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control-v2",
		BuildGoVersion: "go1.24.0", BuildVCSRevision: strings.Repeat("a", 40),
		InputFileSHA256: map[string]string{
			"adapter": strings.Repeat("1", 64), "tool": strings.Repeat("2", 64),
			"runtime_python": strings.Repeat("3", 64),
		},
		PackageManifestSHA256: strings.Repeat("4", 64),
		ProtocolSHA256:        strings.Repeat("5", 64), ScheduleSHA256: strings.Repeat("6", 64),
		ModelManifestSHA256: strings.Repeat("7", 64), ModelArtifactSHA256: TargetModelArtifactSHA256,
		RuntimeManifestSHA256: strings.Repeat("8", 64), RuntimeFullTreeSHA256: RuntimeVenvManifestSHA256,
		HostPreflightSHA256:       strings.Repeat("9", 64),
		ConformanceRequestSHA256:  strings.Repeat("c", 64),
		OutputNamespaceSHA256:     DigestBytes([]byte("/private/tmp/run")),
		AdapterCheckDirectoryHash: DigestBytes([]byte("/private/tmp/check")),
	}

	request.RequestSHA256, _ = DigestDomain("adapter-check-request", adapterCheckRequestProjection(request))
	conformance, err := evaluateFramingConformance(framingConformanceRequest())
	if err != nil {
		t.Fatal(err)
	}
	receipt := AdapterCheckReceipt{
		SchemaVersion: AdapterCheckReceiptSchema, RequestSHA256: request.RequestSHA256,
		StartedAt: request.StartedAt, CompletedAt: request.StartedAt.Add(time.Second),
		ExpiresAt: request.ExpiresAt, ImplementationCommit: request.ImplementationCommit,
		ImplementationTree: request.ImplementationTree, BuildPackage: request.BuildPackage,
		BuildGoVersion: request.BuildGoVersion, BuildVCSRevision: request.BuildVCSRevision,
		InputFileSHA256:       request.InputFileSHA256,
		PackageManifestSHA256: request.PackageManifestSHA256,
		ProtocolSHA256:        request.ProtocolSHA256, ScheduleSHA256: request.ScheduleSHA256,
		ModelManifestSHA256:       request.ModelManifestSHA256,
		ModelArtifactSHA256:       request.ModelArtifactSHA256,
		RuntimeManifestSHA256:     request.RuntimeManifestSHA256,
		RuntimeFullTreeSHA256:     request.RuntimeFullTreeSHA256,
		RuntimeInterpreterSHA256:  request.InputFileSHA256["runtime_python"],
		AdapterSHA256:             request.InputFileSHA256["adapter"],
		ToolSHA256:                request.InputFileSHA256["tool"],
		HostPreflightSHA256:       request.HostPreflightSHA256,
		ConformanceRequestSHA256:  request.ConformanceRequestSHA256,
		FramingConformanceSHA256:  conformance.ConformanceSHA256,
		FramingConformanceVectors: len(conformance.Results),
		OutputNamespaceSHA256:     request.OutputNamespaceSHA256,
		AdapterCheckDirectoryHash: request.AdapterCheckDirectoryHash,
	}
	sign := func(value *AdapterCheckReceipt) {
		value.ReceiptSHA256, _ = DigestDomain("adapter-check-receipt", adapterCheckReceiptProjection(*value))
	}
	sign(&receipt)
	if err := validateAdapterCheckReceiptValue(request, receipt, conformance, now.Add(2*time.Second)); err != nil {
		t.Fatalf("valid receipt rejected: %v", err)
	}
	tests := map[string]func(*AdapterCheckReceipt){
		"source commit": func(value *AdapterCheckReceipt) { value.ImplementationCommit = strings.Repeat("d", 40) },
		"source tree":   func(value *AdapterCheckReceipt) { value.ImplementationTree = strings.Repeat("d", 40) },
		"binary":        func(value *AdapterCheckReceipt) { value.ToolSHA256 = strings.Repeat("d", 64) },
		"adapter":       func(value *AdapterCheckReceipt) { value.AdapterSHA256 = strings.Repeat("d", 64) },
		"interpreter":   func(value *AdapterCheckReceipt) { value.RuntimeInterpreterSHA256 = strings.Repeat("d", 64) },
		"package":       func(value *AdapterCheckReceipt) { value.PackageManifestSHA256 = strings.Repeat("d", 64) },
		"protocol":      func(value *AdapterCheckReceipt) { value.ProtocolSHA256 = strings.Repeat("d", 64) },
		"schedule":      func(value *AdapterCheckReceipt) { value.ScheduleSHA256 = strings.Repeat("d", 64) },
		"model":         func(value *AdapterCheckReceipt) { value.ModelManifestSHA256 = strings.Repeat("d", 64) },
		"runtime":       func(value *AdapterCheckReceipt) { value.RuntimeManifestSHA256 = strings.Repeat("d", 64) },
		"namespace":     func(value *AdapterCheckReceipt) { value.OutputNamespaceSHA256 = strings.Repeat("d", 64) },
		"check path":    func(value *AdapterCheckReceipt) { value.AdapterCheckDirectoryHash = strings.Repeat("d", 64) },
		"model loaded":  func(value *AdapterCheckReceipt) { value.ModelLoaded = true },
		"network":       func(value *AdapterCheckReceipt) { value.NetworkAllowed = true },
	}
	for name, mutate := range tests {
		t.Run(name, func(t *testing.T) {
			changed := receipt
			changed.InputFileSHA256 = maps.Clone(receipt.InputFileSHA256)
			mutate(&changed)
			sign(&changed)
			if err := validateAdapterCheckReceiptValue(request, changed, conformance, now.Add(2*time.Second)); err == nil {
				t.Fatal("bound identity drift was accepted")
			}
		})
	}
	stale := receipt
	stale.ReceiptSHA256 = ""
	sign(&stale)
	if err := validateAdapterCheckReceiptValue(request, stale, conformance, request.ExpiresAt); err == nil {
		t.Fatal("expired adapter-check receipt was accepted")
	}
}

func TestAdapterCheckReceiptUseIsSingleUseAndPathBound(t *testing.T) {
	directory := t.TempDir()
	request := AdapterCheckRequest{AdapterCheckDirectoryHash: DigestBytes([]byte(directory))}
	receipt := AdapterCheckReceipt{ReceiptSHA256: strings.Repeat("a", 64)}
	genesis := AdapterCheckUseGenesis{
		SchemaVersion:             AdapterCheckReceiptSchema + "/use-genesis",
		AdapterCheckReceiptSHA256: receipt.ReceiptSHA256,
		AdapterCheckDirectoryHash: request.AdapterCheckDirectoryHash,
	}
	genesis.GenesisSHA256, _ = DigestDomain(
		"adapter-check-use-genesis", adapterCheckUseGenesisProjection(genesis),
	)
	genesisBytes, _ := canonicalJSON(genesis)
	ledgerPath := filepath.Join(directory, "adapter-check-use.jsonl")
	if err := os.WriteFile(ledgerPath, append(genesisBytes, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	authorization := ExecutionAuthorization{
		AuthorizationDirectorySHA256: strings.Repeat("b", 64),
		AuthorizationSHA256:          strings.Repeat("c", 64),
	}
	if err := consumeAdapterCheck(directory, request, receipt, authorization); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(ledgerPath)
	if err != nil {
		t.Fatal(err)
	}
	_, use, err := validateAdapterCheckUseLedger(data, request, receipt)
	if err != nil || use == nil ||
		use.AuthorizationDirectorySHA256 != authorization.AuthorizationDirectorySHA256 ||
		use.AuthorizationSHA256 != authorization.AuthorizationSHA256 {
		t.Fatalf("invalid adapter-check use record: use=%+v err=%v", use, err)
	}
	if err := consumeAdapterCheck(directory, request, receipt, authorization); err == nil {
		t.Fatal("adapter-check receipt replay was accepted")
	}
	copied := t.TempDir()
	copiedRequest := request
	copiedRequest.AdapterCheckDirectoryHash = DigestBytes([]byte(copied))
	if _, _, err := validateAdapterCheckUseLedger(data, copiedRequest, receipt); err == nil {
		t.Fatal("copied adapter-check path was accepted")
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
			"/usr/bin/sandbox-exec\x00-p\x00" + SandboxPolicy + "\x00/usr/bin/python3\x00-I\x00-B\x00-S\x00-c\x00import socket\ns = socket.socket()\ntry:\n    s.connect((\"127.0.0.1\", 9))\nexcept PermissionError:\n    raise SystemExit(0)\nexcept OSError:\n    raise SystemExit(2)\nraise SystemExit(1)": "",
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

func (adapter *fakeAdapter) Ready() AdapterReady { return AdapterReady{ModelLoaded: true} }
func (adapter *fakeAdapter) PreloadReady() AdapterPreloadReady {
	return AdapterPreloadReady{ModelLoaded: false}
}
func (adapter *fakeAdapter) Load(context.Context) (AdapterReady, error) {
	return adapter.Ready(), nil
}
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
		"adapter-check.json":    authorizationFiles["adapter-check.json"],
		"package-manifest.json": pkg.Files["package-manifest.json"],
		"host-preflight.json":   authorizationFiles["host-preflight.json"],
		"model-manifest.json":   authorizationFiles["model-manifest.json"],
		"runtime-manifest.json": authorizationFiles["runtime-manifest.json"],
		"run-manifest.json":     runManifestBytes, "completion.json": completionBytes,
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
	if err != nil || strictJSONObjectFileDecode(receiptBytes, &receipt) != nil {
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
	goConformance, err := evaluateFramingConformance(framingConformanceRequest())
	if err != nil {
		panic(err)
	}
	now := time.Now().UTC()
	check := AdapterCheckReceipt{
		SchemaVersion: AdapterCheckReceiptSchema, RequestSHA256: strings.Repeat("2", 64),
		StartedAt: now, CompletedAt: now.Add(time.Second), ExpiresAt: now.Add(time.Hour),
		ImplementationCommit: "0000000000000000000000000000000000000000",
		ImplementationTree:   "0000000000000000000000000000000000000000",
		BuildPackage:         "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control-v2",
		BuildGoVersion:       "go1.24.0", BuildVCSRevision: "0000000000000000000000000000000000000000",
		InputFileSHA256:       map[string]string{"adapter": strings.Repeat("3", 64)},
		PackageManifestSHA256: DigestBytes(pkg.Files["package-manifest.json"]),
		ProtocolSHA256:        pkg.Protocol.ProtocolSHA256, ScheduleSHA256: pkg.Manifest.ScheduleSHA256,
		ModelManifestSHA256:      modelManifest.ManifestSHA256,
		ModelArtifactSHA256:      modelManifest.ArtifactSHA256,
		RuntimeManifestSHA256:    runtimeManifest.ManifestSHA256,
		RuntimeFullTreeSHA256:    runtimeManifest.FullTreeSHA256,
		RuntimeInterpreterSHA256: strings.Repeat("4", 64), AdapterSHA256: strings.Repeat("3", 64),
		ToolSHA256: strings.Repeat("5", 64), HostPreflightSHA256: strings.Repeat("6", 64),
		ConformanceRequestSHA256:  strings.Repeat("7", 64),
		FramingConformanceSHA256:  goConformance.ConformanceSHA256,
		FramingConformanceVectors: len(goConformance.Results),
		OutputNamespaceSHA256:     DigestBytes([]byte(runDirectory)),
		AdapterCheckDirectoryHash: DigestBytes([]byte(filepath.Join(authorizationDirectory, "adapter-check"))),
	}
	check.ReceiptSHA256, _ = DigestDomain("adapter-check-receipt", adapterCheckReceiptProjection(check))
	checkBytes, _ := canonicalJSONFile(check)
	authorization := ExecutionAuthorization{
		SchemaVersion:        SchemaVersion + "/execution-authorization",
		Acknowledgement:      AuthorizationAcknowledgement,
		ImplementationCommit: "0000000000000000000000000000000000000000",
		ImplementationTree:   "0000000000000000000000000000000000000000",
		MainRef:              "origin/main",
		BuildPackage:         "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control-v2",
		BuildGoVersion:       "go1.24.0", BuildVCSRevision: "0000000000000000000000000000000000000000",
		PackageManifestSHA256: DigestBytes(pkg.Files["package-manifest.json"]),
		ScheduleSHA256:        pkg.Manifest.ScheduleSHA256, ModelManifestSHA256: modelManifest.ManifestSHA256,
		ModelArtifactSHA256: TargetModelArtifactSHA256, RuntimeManifestSHA256: runtimeManifest.ManifestSHA256,
		RuntimeFullTreeSHA256: runtimeManifest.FullTreeSHA256, Observations: ObservationsPerModel,
		AdapterCheckRequestSHA256:    check.RequestSHA256,
		AdapterCheckReceiptSHA256:    check.ReceiptSHA256,
		AdapterCheckDirectorySHA256:  check.AdapterCheckDirectoryHash,
		AdapterCheckExpiresAt:        check.ExpiresAt,
		FramingConformanceSHA256:     check.FramingConformanceSHA256,
		ProtocolSHA256:               pkg.Protocol.ProtocolSHA256,
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
		AdapterCheckReceiptSHA256: authorization.AdapterCheckReceiptSHA256,
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
		"authorization.json": authorizationBytes, "adapter-check.json": checkBytes,
		"host-preflight.json": hostBytes,
		"model-manifest.json": modelBytes, "runtime-manifest.json": runtimeBytes,
		"attempt-ledger.jsonl": append(append(append([]byte(nil), genesisBytes...), '\n'), append(attemptBytes, '\n')...),
	}
	files["SHA256SUMS"] = renderChecksums(map[string][]byte{
		"authorization.json": authorizationBytes, "adapter-check.json": checkBytes,
		"host-preflight.json": hostBytes,
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
		Path:      "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control-v2",
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
	if _, appended, err := consumeAuthorization(authorizationDirectory, loaded); err != nil || !appended {
		t.Fatal(err)
	}
	if _, _, err := consumeAuthorization(authorizationDirectory, loaded); err == nil {
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

func TestAuthorizationConsumptionReportsDurableAppendOnPostSyncFailure(t *testing.T) {
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
	genesisBytes := bytes.Split(files["attempt-ledger.jsonl"], []byte{'\n'})[0]
	files["attempt-ledger.jsonl"] = append(genesisBytes, '\n')
	if err := writePrivateDirectory(authorizationDirectory, authorizationFilesListForTest(), files); err != nil {
		t.Fatal(err)
	}
	expected := errors.New("post-sync failure")
	attempt, appended, err := consumeAuthorizationWithPostAppend(
		authorizationDirectory, binding.Authorization,
		func(*os.File, string, string) error { return expected },
	)
	if !appended || !errors.Is(err, expected) || attempt.EntrySHA256 == "" {
		t.Fatalf("durable append was not surfaced: appended=%v attempt=%+v err=%v", appended, attempt, err)
	}
	data, err := os.ReadFile(filepath.Join(authorizationDirectory, "attempt-ledger.jsonl"))
	if err != nil {
		t.Fatal(err)
	}
	if _, persisted, err := validateAttemptLedgerBytes(data, binding.Authorization, true); err != nil || persisted == nil {
		t.Fatalf("durable attempt was not preserved after post-sync failure: %v", err)
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

func stringPointer(value string) *string {
	return &value
}
