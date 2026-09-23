package studylocalv2

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"maps"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"golang.org/x/sys/unix"
)

const (
	AdapterCheckRequestSchema = SchemaVersion + "/adapter-check-request"
	AdapterCheckReceiptSchema = SchemaVersion + "/adapter-check-receipt"
	AdapterCheckMaxValidity   = 2 * time.Hour
)

type AdapterCheckRequest struct {
	SchemaVersion             string            `json:"schema_version"`
	StartedAt                 time.Time         `json:"started_at"`
	ExpiresAt                 time.Time         `json:"expires_at"`
	ImplementationCommit      string            `json:"implementation_commit"`
	ImplementationTree        string            `json:"implementation_tree"`
	MainRef                   string            `json:"main_ref"`
	BuildPackage              string            `json:"build_package"`
	BuildGoVersion            string            `json:"build_go_version"`
	BuildVCSRevision          string            `json:"build_vcs_revision"`
	BuildVCSModified          bool              `json:"build_vcs_modified"`
	ToolPath                  string            `json:"tool_path"`
	AdapterPath               string            `json:"adapter_path"`
	RepositoryRoot            string            `json:"repository_root"`
	PackageManifestPath       string            `json:"package_manifest_path"`
	ModelDirectory            string            `json:"model_directory"`
	ModelManifestPath         string            `json:"model_manifest_path"`
	RuntimePython             string            `json:"runtime_python"`
	RuntimeManifestPath       string            `json:"runtime_manifest_path"`
	RuntimeTreeManifestPath   string            `json:"runtime_tree_manifest_path"`
	BaseTreeManifestPath      string            `json:"base_tree_manifest_path"`
	WheelVerificationPath     string            `json:"wheel_verification_path"`
	HostPreflightPath         string            `json:"host_preflight_path"`
	ConformanceRequestPath    string            `json:"conformance_request_path"`
	OutputNamespace           string            `json:"output_namespace"`
	AdapterCheckDirectory     string            `json:"adapter_check_directory"`
	InputFileSHA256           map[string]string `json:"input_file_sha256"`
	PackageManifestSHA256     string            `json:"package_manifest_sha256"`
	ProtocolSHA256            string            `json:"protocol_sha256"`
	ScheduleSHA256            string            `json:"schedule_sha256"`
	ModelManifestSHA256       string            `json:"model_manifest_sha256"`
	ModelArtifactSHA256       string            `json:"model_artifact_sha256"`
	RuntimeManifestSHA256     string            `json:"runtime_manifest_sha256"`
	RuntimeFullTreeSHA256     string            `json:"runtime_full_tree_sha256"`
	HostPreflightSHA256       string            `json:"host_preflight_sha256"`
	ConformanceRequestSHA256  string            `json:"conformance_request_sha256"`
	OutputNamespaceSHA256     string            `json:"output_namespace_sha256"`
	AdapterCheckDirectoryHash string            `json:"adapter_check_directory_sha256"`
	ModelLoadedExpected       bool              `json:"model_loaded_expected"`
	NetworkAllowed            bool              `json:"network_allowed"`
	RequestSHA256             string            `json:"request_sha256"`
}

type AdapterCheckReceipt struct {
	SchemaVersion             string            `json:"schema_version"`
	RequestSHA256             string            `json:"request_sha256"`
	StartedAt                 time.Time         `json:"started_at"`
	CompletedAt               time.Time         `json:"completed_at"`
	ExpiresAt                 time.Time         `json:"expires_at"`
	ImplementationCommit      string            `json:"implementation_commit"`
	ImplementationTree        string            `json:"implementation_tree"`
	BuildPackage              string            `json:"build_package"`
	BuildGoVersion            string            `json:"build_go_version"`
	BuildVCSRevision          string            `json:"build_vcs_revision"`
	BuildVCSModified          bool              `json:"build_vcs_modified"`
	InputFileSHA256           map[string]string `json:"input_file_sha256"`
	PackageManifestSHA256     string            `json:"package_manifest_sha256"`
	ProtocolSHA256            string            `json:"protocol_sha256"`
	ScheduleSHA256            string            `json:"schedule_sha256"`
	ModelManifestSHA256       string            `json:"model_manifest_sha256"`
	ModelArtifactSHA256       string            `json:"model_artifact_sha256"`
	RuntimeManifestSHA256     string            `json:"runtime_manifest_sha256"`
	RuntimeFullTreeSHA256     string            `json:"runtime_full_tree_sha256"`
	RuntimeInterpreterSHA256  string            `json:"runtime_interpreter_sha256"`
	AdapterSHA256             string            `json:"adapter_sha256"`
	ToolSHA256                string            `json:"tool_sha256"`
	HostPreflightSHA256       string            `json:"host_preflight_sha256"`
	ConformanceRequestSHA256  string            `json:"conformance_request_sha256"`
	FramingConformanceSHA256  string            `json:"framing_conformance_sha256"`
	FramingConformanceVectors int               `json:"framing_conformance_vectors"`
	OutputNamespaceSHA256     string            `json:"output_namespace_sha256"`
	AdapterCheckDirectoryHash string            `json:"adapter_check_directory_sha256"`
	ModelLoaded               bool              `json:"model_loaded"`
	NetworkAllowed            bool              `json:"network_allowed"`
	ReceiptSHA256             string            `json:"receipt_sha256"`
}

type AdapterCheckOptions struct {
	PackageDirectory            string
	ModelDirectory              string
	ModelManifestPath           string
	RuntimePython               string
	RuntimeManifestPath         string
	RuntimeTreeManifestPath     string
	BaseTreeManifestPath        string
	WheelVerificationPath       string
	AdapterPath                 string
	RepositoryRoot              string
	MainRef                     string
	TrustedImplementationCommit string
	TrustedImplementationTree   string
	RunOutputDirectory          string
	OutputDirectory             string
	ExpiresAt                   time.Time
	StartedAt                   time.Time
}

type AdapterCheckUseGenesis struct {
	SchemaVersion             string `json:"schema_version"`
	AdapterCheckReceiptSHA256 string `json:"adapter_check_receipt_sha256"`
	AdapterCheckDirectoryHash string `json:"adapter_check_directory_sha256"`
	GenesisSHA256             string `json:"genesis_sha256"`
}

type AdapterCheckUse struct {
	SchemaVersion                string    `json:"schema_version"`
	AdapterCheckReceiptSHA256    string    `json:"adapter_check_receipt_sha256"`
	GenesisSHA256                string    `json:"genesis_sha256"`
	AuthorizationDirectorySHA256 string    `json:"authorization_directory_sha256"`
	AuthorizationSHA256          string    `json:"authorization_sha256"`
	UsedAt                       time.Time `json:"used_at"`
	UseSHA256                    string    `json:"use_sha256"`
}

var adapterCheckFiles = []string{
	"adapter-check-request.json",
	"adapter-check.json",
	"adapter-check-use.jsonl",
	"conformance-request.json",
	"host-preflight.json",
	"SHA256SUMS",
}

func RunAdapterCheck(ctx context.Context, options AdapterCheckOptions) (receipt AdapterCheckReceipt, err error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return AdapterCheckReceipt{}, fmt.Errorf("MLX adapter check requires darwin/arm64")
	}
	options.PackageDirectory, err = filepath.Abs(options.PackageDirectory)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	pkg, err := ValidatePackage(options.PackageDirectory)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	model, _, err := ReadModelManifest(options.ModelManifestPath)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	if err := ValidateModelManifest(options.ModelDirectory, model); err != nil {
		return AdapterCheckReceipt{}, err
	}
	runtimeManifest, _, err := readRuntimeManifest(options.RuntimeManifestPath)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	commit, tree, err := verifyMergedCleanRepository(options.RepositoryRoot, options.MainRef, options.AdapterPath)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	if commit != options.TrustedImplementationCommit || tree != options.TrustedImplementationTree {
		return AdapterCheckReceipt{}, fmt.Errorf("checkout does not match the reviewed v2 commit and tree")
	}
	build, err := verifyExecutableBuild(commit)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	output, err := safeAbsentOutput(
		options.RunOutputDirectory, options.RepositoryRoot, options.ModelDirectory,
		filepath.Dir(filepath.Dir(options.RuntimePython)),
	)
	if err != nil {
		return AdapterCheckReceipt{}, fmt.Errorf("run output: %w", err)
	}
	checkDirectory, err := safeAbsentOutput(
		options.OutputDirectory, options.RepositoryRoot, options.ModelDirectory,
		filepath.Dir(filepath.Dir(options.RuntimePython)),
	)
	if err != nil {
		return AdapterCheckReceipt{}, fmt.Errorf("adapter-check output: %w", err)
	}
	if checkDirectory == output {
		return AdapterCheckReceipt{}, fmt.Errorf("adapter-check and run namespaces must differ")
	}
	if options.StartedAt.IsZero() {
		options.StartedAt = time.Now().UTC()
	}
	options.StartedAt = options.StartedAt.UTC()
	options.ExpiresAt = options.ExpiresAt.UTC()
	if !options.ExpiresAt.After(options.StartedAt) ||
		options.ExpiresAt.Sub(options.StartedAt) > AdapterCheckMaxValidity {
		return AdapterCheckReceipt{}, fmt.Errorf("adapter-check expiry must be after start and within %s", AdapterCheckMaxValidity)
	}
	host, err := CollectHostPreflight(ctx, filepath.Dir(output), pkg.Protocol.Isolation)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	if !host.Eligible {
		return AdapterCheckReceipt{}, fmt.Errorf("host preflight refused adapter check")
	}
	hostBytes, err := encodeHostSnapshot(host)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	conformance := framingConformanceRequest()
	goConformance, err := evaluateFramingConformance(conformance)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	if goConformance.ConformanceSHA256 != FramingConformanceKnownAnswerSHA256 {
		return AdapterCheckReceipt{}, fmt.Errorf("framing conformance known answer drifted")
	}
	conformanceBytes, err := canonicalJSONFile(conformance)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	if err := os.Mkdir(checkDirectory, 0o700); err != nil {
		return AdapterCheckReceipt{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(checkDirectory)
		}
	}()
	hostPath := filepath.Join(checkDirectory, "host-preflight.json")
	conformancePath := filepath.Join(checkDirectory, "conformance-request.json")
	if err := writeExclusive(hostPath, hostBytes, 0o600); err != nil {
		return AdapterCheckReceipt{}, err
	}
	if err := writeExclusive(conformancePath, conformanceBytes, 0o600); err != nil {
		return AdapterCheckReceipt{}, err
	}
	request, err := buildAdapterCheckRequest(
		options, pkg, model, runtimeManifest, build, commit, tree, output, checkDirectory,
		hostPath, hostBytes, conformancePath, conformanceBytes,
	)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	requestBytes, err := canonicalJSONFile(request)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	requestPath := filepath.Join(checkDirectory, "adapter-check-request.json")
	if err := writeExclusive(requestPath, requestBytes, 0o600); err != nil {
		return AdapterCheckReceipt{}, err
	}
	temporary := filepath.Join(checkDirectory, "tmp")
	if err := os.Mkdir(temporary, 0o700); err != nil {
		return AdapterCheckReceipt{}, err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	command := exec.CommandContext(
		ctx, "/usr/bin/sandbox-exec", "-p", SandboxPolicy,
		options.RuntimePython, "-I", "-B", "-S", options.AdapterPath, "adapter-check",
		"--request", requestPath,
	)
	command.Dir = "/"
	command.Env = isolatedEnvironment(temporary)
	var stdout, stderr boundedBuffer
	stdout.limit = maxAdapterLineBytes
	stderr.limit = maxAdapterStderrBytes
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return AdapterCheckReceipt{}, fmt.Errorf("adapter check: %w: %s", err, stderr.String())
	}
	if err := os.RemoveAll(temporary); err != nil {
		return AdapterCheckReceipt{}, err
	}
	if err := strictJSONFrameDecode(stdout.Bytes(), &receipt); err != nil {
		return AdapterCheckReceipt{}, fmt.Errorf("adapter-check receipt frame: %w", err)
	}
	if err := validateAdapterCheckReceiptValue(request, receipt, goConformance, time.Now().UTC()); err != nil {
		return AdapterCheckReceipt{}, err
	}
	receiptBytes, err := canonicalJSONFile(receipt)
	if err != nil {
		return AdapterCheckReceipt{}, err
	}
	if err := writeExclusive(filepath.Join(checkDirectory, "adapter-check.json"), receiptBytes, 0o600); err != nil {
		return AdapterCheckReceipt{}, err
	}
	useGenesis := AdapterCheckUseGenesis{
		SchemaVersion:             AdapterCheckReceiptSchema + "/use-genesis",
		AdapterCheckReceiptSHA256: receipt.ReceiptSHA256,
		AdapterCheckDirectoryHash: receipt.AdapterCheckDirectoryHash,
	}
	useGenesis.GenesisSHA256, _ = DigestDomain(
		"adapter-check-use-genesis", adapterCheckUseGenesisProjection(useGenesis),
	)
	useGenesisBytes, _ := canonicalJSON(useGenesis)
	if err := writeExclusive(
		filepath.Join(checkDirectory, "adapter-check-use.jsonl"),
		append(useGenesisBytes, '\n'), 0o600,
	); err != nil {
		return AdapterCheckReceipt{}, err
	}
	files := map[string][]byte{
		"adapter-check-request.json": requestBytes,
		"adapter-check.json":         receiptBytes,
		"conformance-request.json":   conformanceBytes,
		"host-preflight.json":        hostBytes,
	}
	if err := writeExclusive(filepath.Join(checkDirectory, "SHA256SUMS"), renderChecksums(files), 0o600); err != nil {
		return AdapterCheckReceipt{}, err
	}
	if err := syncDirectory(checkDirectory); err != nil {
		return AdapterCheckReceipt{}, err
	}
	published = true
	return receipt, nil
}

func buildAdapterCheckRequest(
	options AdapterCheckOptions,
	pkg Package,
	model ModelManifest,
	runtimeManifest RuntimeManifest,
	build executableBuild,
	commit, tree, output, checkDirectory, hostPath string,
	hostBytes []byte,
	conformancePath string,
	conformanceBytes []byte,
) (AdapterCheckRequest, error) {
	toolPath, err := os.Executable()
	if err != nil {
		return AdapterCheckRequest{}, err
	}
	toolPath, err = filepath.EvalSymlinks(toolPath)
	if err != nil {
		return AdapterCheckRequest{}, err
	}
	adapterPath, err := safeRegularAbsolute(options.AdapterPath)
	if err != nil {
		return AdapterCheckRequest{}, err
	}
	runtimePython, err := safePythonExecutable(options.RuntimePython)
	if err != nil {
		return AdapterCheckRequest{}, err
	}
	resolvedPython, err := filepath.EvalSymlinks(runtimePython)
	if err != nil {
		return AdapterCheckRequest{}, err
	}
	packageManifestPath := filepath.Join(options.PackageDirectory, "package-manifest.json")
	inputPaths := map[string]string{
		"adapter":               adapterPath,
		"tool":                  toolPath,
		"package_manifest":      packageManifestPath,
		"model_manifest":        options.ModelManifestPath,
		"runtime_manifest":      options.RuntimeManifestPath,
		"runtime_python":        resolvedPython,
		"runtime_tree_manifest": options.RuntimeTreeManifestPath,
		"base_tree_manifest":    options.BaseTreeManifestPath,
		"wheel_verification":    options.WheelVerificationPath,
		"host_preflight":        hostPath,
		"conformance_request":   conformancePath,
	}
	hashes := make(map[string]string, len(inputPaths))
	for name, path := range inputPaths {
		hash, err := digestSafeInputFile(path)
		if err != nil {
			return AdapterCheckRequest{}, fmt.Errorf("%s: %w", name, err)
		}
		hashes[name] = hash
	}
	request := AdapterCheckRequest{
		SchemaVersion: AdapterCheckRequestSchema, StartedAt: options.StartedAt,
		ExpiresAt: options.ExpiresAt, ImplementationCommit: commit, ImplementationTree: tree,
		MainRef: options.MainRef, BuildPackage: build.Package, BuildGoVersion: build.GoVersion,
		BuildVCSRevision: build.Revision, BuildVCSModified: build.Modified,
		ToolPath: toolPath, AdapterPath: adapterPath, RepositoryRoot: options.RepositoryRoot,
		PackageManifestPath: packageManifestPath,
		ModelDirectory:      options.ModelDirectory, ModelManifestPath: options.ModelManifestPath,
		RuntimePython: resolvedPython, RuntimeManifestPath: options.RuntimeManifestPath,
		RuntimeTreeManifestPath: options.RuntimeTreeManifestPath,
		BaseTreeManifestPath:    options.BaseTreeManifestPath,
		WheelVerificationPath:   options.WheelVerificationPath,
		HostPreflightPath:       hostPath, ConformanceRequestPath: conformancePath,
		OutputNamespace: output, AdapterCheckDirectory: checkDirectory, InputFileSHA256: hashes,
		PackageManifestSHA256: DigestBytes(pkg.Files["package-manifest.json"]),
		ProtocolSHA256:        pkg.Protocol.ProtocolSHA256, ScheduleSHA256: pkg.Manifest.ScheduleSHA256,
		ModelManifestSHA256: model.ManifestSHA256, ModelArtifactSHA256: model.ArtifactSHA256,
		RuntimeManifestSHA256:     runtimeManifest.ManifestSHA256,
		RuntimeFullTreeSHA256:     runtimeManifest.FullTreeSHA256,
		HostPreflightSHA256:       DigestBytes(hostBytes),
		ConformanceRequestSHA256:  DigestBytes(conformanceBytes),
		OutputNamespaceSHA256:     DigestBytes([]byte(output)),
		AdapterCheckDirectoryHash: DigestBytes([]byte(checkDirectory)),
		ModelLoadedExpected:       false, NetworkAllowed: false,
	}
	request.RequestSHA256, err = DigestDomain("adapter-check-request", adapterCheckRequestProjection(request))
	if err != nil {
		return AdapterCheckRequest{}, err
	}
	return request, nil
}

func adapterCheckRequestProjection(request AdapterCheckRequest) AdapterCheckRequest {
	request.RequestSHA256 = ""
	return request
}

func adapterCheckReceiptProjection(receipt AdapterCheckReceipt) AdapterCheckReceipt {
	receipt.ReceiptSHA256 = ""
	return receipt
}

func adapterCheckUseGenesisProjection(genesis AdapterCheckUseGenesis) AdapterCheckUseGenesis {
	genesis.GenesisSHA256 = ""
	return genesis
}

func adapterCheckUseProjection(use AdapterCheckUse) AdapterCheckUse {
	use.UseSHA256 = ""
	return use
}

func digestSafeInputFile(path string) (string, error) {
	safe, err := safeRegularAbsolute(path)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(safe)
	if err != nil {
		return "", err
	}
	return digestRegularFile(safe, info)
}

func validateAdapterCheckReceiptValue(
	request AdapterCheckRequest,
	receipt AdapterCheckReceipt,
	conformance FramingConformanceResponse,
	now time.Time,
) error {
	requestDigest, _ := DigestDomain("adapter-check-request", adapterCheckRequestProjection(request))
	receiptDigest, _ := DigestDomain("adapter-check-receipt", adapterCheckReceiptProjection(receipt))
	if request.SchemaVersion != AdapterCheckRequestSchema || request.RequestSHA256 != requestDigest ||
		receipt.SchemaVersion != AdapterCheckReceiptSchema ||
		receipt.RequestSHA256 != request.RequestSHA256 ||
		receipt.ReceiptSHA256 != receiptDigest ||
		receipt.StartedAt != request.StartedAt || receipt.ExpiresAt != request.ExpiresAt ||
		receipt.CompletedAt.Before(receipt.StartedAt) || receipt.CompletedAt.After(receipt.ExpiresAt) ||
		!now.Before(receipt.ExpiresAt) ||
		receipt.ImplementationCommit != request.ImplementationCommit ||
		receipt.ImplementationTree != request.ImplementationTree ||
		receipt.BuildPackage != request.BuildPackage || receipt.BuildGoVersion != request.BuildGoVersion ||
		receipt.BuildVCSRevision != request.BuildVCSRevision ||
		receipt.BuildVCSModified != request.BuildVCSModified ||
		!maps.Equal(receipt.InputFileSHA256, request.InputFileSHA256) ||
		receipt.PackageManifestSHA256 != request.PackageManifestSHA256 ||
		receipt.ProtocolSHA256 != request.ProtocolSHA256 ||
		receipt.ScheduleSHA256 != request.ScheduleSHA256 ||
		receipt.ModelManifestSHA256 != request.ModelManifestSHA256 ||
		receipt.ModelArtifactSHA256 != request.ModelArtifactSHA256 ||
		receipt.RuntimeManifestSHA256 != request.RuntimeManifestSHA256 ||
		receipt.RuntimeFullTreeSHA256 != request.RuntimeFullTreeSHA256 ||
		receipt.RuntimeInterpreterSHA256 != request.InputFileSHA256["runtime_python"] ||
		receipt.AdapterSHA256 != request.InputFileSHA256["adapter"] ||
		receipt.ToolSHA256 != request.InputFileSHA256["tool"] ||
		receipt.HostPreflightSHA256 != request.HostPreflightSHA256 ||
		receipt.ConformanceRequestSHA256 != request.ConformanceRequestSHA256 ||
		receipt.FramingConformanceSHA256 != conformance.ConformanceSHA256 ||
		receipt.FramingConformanceVectors != len(conformance.Results) ||
		receipt.OutputNamespaceSHA256 != request.OutputNamespaceSHA256 ||
		receipt.AdapterCheckDirectoryHash != request.AdapterCheckDirectoryHash ||
		receipt.ModelLoaded || receipt.NetworkAllowed {
		return fmt.Errorf("adapter-check receipt binding mismatch")
	}
	return nil
}

func LoadAndVerifyAdapterCheck(
	ctx context.Context,
	directory string,
	options AdapterCheckOptions,
	now time.Time,
) (AdapterCheckRequest, AdapterCheckReceipt, HostSnapshot, error) {
	var err error
	options.PackageDirectory, err = filepath.Abs(options.PackageDirectory)
	if err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	files, err := readExactPrivateDirectory(
		directory, adapterCheckFiles, map[string]bool{"adapter-check-use.jsonl": true},
	)
	if err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	var request AdapterCheckRequest
	if err := strictJSONObjectFileDecode(files["adapter-check-request.json"], &request); err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	var receipt AdapterCheckReceipt
	if err := strictJSONObjectFileDecode(files["adapter-check.json"], &receipt); err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	var host HostSnapshot
	if err := strictJSONObjectFileDecode(files["host-preflight.json"], &host); err != nil || !host.Eligible {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("invalid adapter-check host preflight")
	}
	var conformanceRequest FramingConformanceRequest
	if err := strictJSONObjectFileDecode(files["conformance-request.json"], &conformanceRequest); err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	goConformance, err := evaluateFramingConformance(conformanceRequest)
	if err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	frozenConformance := framingConformanceRequest()
	if goConformance.ConformanceSHA256 != FramingConformanceKnownAnswerSHA256 ||
		len(conformanceRequest.Vectors) != len(frozenConformance.Vectors) {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("framing conformance request is not the frozen known answer")
	}
	if request.AdapterCheckDirectory != directory ||
		request.AdapterCheckDirectoryHash != DigestBytes([]byte(directory)) ||
		request.HostPreflightPath != filepath.Join(directory, "host-preflight.json") ||
		request.ConformanceRequestPath != filepath.Join(directory, "conformance-request.json") ||
		request.HostPreflightSHA256 != DigestBytes(files["host-preflight.json"]) ||
		request.ConformanceRequestSHA256 != DigestBytes(files["conformance-request.json"]) {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("adapter-check directory was copied or rebound")
	}
	if err := validateAdapterCheckReceiptValue(request, receipt, goConformance, now.UTC()); err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	if _, use, err := validateAdapterCheckUseLedger(files["adapter-check-use.jsonl"], request, receipt); err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	} else if use != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("adapter-check receipt has already been used")
	}
	if options.OutputDirectory != directory ||
		options.PackageDirectory == "" ||
		request.PackageManifestPath != filepath.Join(options.PackageDirectory, "package-manifest.json") ||
		request.ModelDirectory != options.ModelDirectory ||
		request.ModelManifestPath != options.ModelManifestPath ||
		request.RuntimeManifestPath != options.RuntimeManifestPath ||
		request.RuntimeTreeManifestPath != options.RuntimeTreeManifestPath ||
		request.BaseTreeManifestPath != options.BaseTreeManifestPath ||
		request.WheelVerificationPath != options.WheelVerificationPath ||
		request.AdapterPath != options.AdapterPath ||
		request.RepositoryRoot != options.RepositoryRoot ||
		request.OutputNamespace != options.RunOutputDirectory ||
		request.ImplementationCommit != options.TrustedImplementationCommit ||
		request.ImplementationTree != options.TrustedImplementationTree ||
		request.MainRef != options.MainRef {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("adapter-check request does not match authorization inputs")
	}
	runtimePython, err := safePythonExecutable(options.RuntimePython)
	if err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	resolvedPython, err := filepath.EvalSymlinks(runtimePython)
	if err != nil || resolvedPython != request.RuntimePython {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("runtime interpreter changed after adapter check")
	}
	pkg, err := ValidatePackage(options.PackageDirectory)
	if err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	model, _, err := ReadModelManifest(options.ModelManifestPath)
	if err != nil || ValidateModelManifest(options.ModelDirectory, model) != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("model changed after adapter check")
	}
	runtimeManifest, _, err := readRuntimeManifest(options.RuntimeManifestPath)
	if err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	commit, tree, err := verifyMergedCleanRepository(options.RepositoryRoot, options.MainRef, options.AdapterPath)
	if err != nil || commit != request.ImplementationCommit || tree != request.ImplementationTree {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("source changed after adapter check")
	}
	build, err := verifyExecutableBuild(commit)
	if err != nil || build.Package != request.BuildPackage || build.GoVersion != request.BuildGoVersion ||
		build.Revision != request.BuildVCSRevision || build.Modified != request.BuildVCSModified {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("binary changed after adapter check")
	}
	inputPaths := map[string]string{
		"adapter": options.AdapterPath, "tool": request.ToolPath,
		"package_manifest": request.PackageManifestPath, "model_manifest": options.ModelManifestPath,
		"runtime_manifest": options.RuntimeManifestPath, "runtime_python": request.RuntimePython,
		"runtime_tree_manifest": options.RuntimeTreeManifestPath,
		"base_tree_manifest":    options.BaseTreeManifestPath, "wheel_verification": options.WheelVerificationPath,
		"host_preflight": request.HostPreflightPath, "conformance_request": request.ConformanceRequestPath,
	}
	for name, path := range inputPaths {
		digest, err := digestSafeInputFile(path)
		if err != nil || digest != request.InputFileSHA256[name] {
			return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("%s changed after adapter check", name)
		}
	}
	output, err := safeAbsentOutput(
		options.RunOutputDirectory, options.RepositoryRoot, options.ModelDirectory,
		filepath.Dir(filepath.Dir(options.RuntimePython)),
	)
	if err != nil || output != request.OutputNamespace ||
		DigestBytes([]byte(output)) != request.OutputNamespaceSHA256 {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("run namespace changed after adapter check")
	}
	if request.PackageManifestSHA256 != DigestBytes(pkg.Files["package-manifest.json"]) ||
		request.ProtocolSHA256 != pkg.Protocol.ProtocolSHA256 ||
		request.ScheduleSHA256 != pkg.Manifest.ScheduleSHA256 ||
		request.ModelManifestSHA256 != model.ManifestSHA256 ||
		request.ModelArtifactSHA256 != model.ArtifactSHA256 ||
		request.RuntimeManifestSHA256 != runtimeManifest.ManifestSHA256 ||
		request.RuntimeFullTreeSHA256 != runtimeManifest.FullTreeSHA256 {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, fmt.Errorf("semantic identity changed after adapter check")
	}
	verifyContext, cancel := runtimeManifestTimeoutContext(ctx)
	defer cancel()
	if err := VerifyRuntimeManifest(verifyContext, RuntimeManifestOptions{
		RuntimePython: options.RuntimePython, AdapterPath: options.AdapterPath,
		RepositoryRoot: options.RepositoryRoot, ImplementationCommit: commit,
		RuntimeTreeManifestPath: options.RuntimeTreeManifestPath,
		BaseTreeManifestPath:    options.BaseTreeManifestPath,
		WheelVerificationPath:   options.WheelVerificationPath,
	}, runtimeManifest); err != nil {
		return AdapterCheckRequest{}, AdapterCheckReceipt{}, HostSnapshot{}, err
	}
	return request, receipt, host, nil
}

func readAdapterCheckFilesForAuthorization(directory string) (map[string][]byte, error) {
	files, err := readExactPrivateDirectory(
		directory, adapterCheckFiles, map[string]bool{"adapter-check-use.jsonl": true},
	)
	if err != nil {
		return nil, err
	}
	out := make(map[string][]byte, len(files))
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		out[name] = bytes.Clone(files[name])
	}
	return out, nil
}

func validateAdapterCheckUseLedger(
	data []byte,
	request AdapterCheckRequest,
	receipt AdapterCheckReceipt,
) (AdapterCheckUseGenesis, *AdapterCheckUse, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' || bytes.ContainsRune(data, '\r') {
		return AdapterCheckUseGenesis{}, nil, fmt.Errorf("adapter-check use ledger envelope is invalid")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) < 1 || len(lines) > 2 {
		return AdapterCheckUseGenesis{}, nil, fmt.Errorf("adapter-check use ledger entry count is invalid")
	}
	var genesis AdapterCheckUseGenesis
	if err := strictJSONPayloadDecode(lines[0], &genesis, true); err != nil {
		return AdapterCheckUseGenesis{}, nil, err
	}
	genesisDigest, _ := DigestDomain(
		"adapter-check-use-genesis", adapterCheckUseGenesisProjection(genesis),
	)
	if genesis.SchemaVersion != AdapterCheckReceiptSchema+"/use-genesis" ||
		genesis.AdapterCheckReceiptSHA256 != receipt.ReceiptSHA256 ||
		genesis.AdapterCheckDirectoryHash != request.AdapterCheckDirectoryHash ||
		genesis.GenesisSHA256 != genesisDigest {
		return AdapterCheckUseGenesis{}, nil, fmt.Errorf("adapter-check use genesis mismatch")
	}
	if len(lines) == 1 {
		return genesis, nil, nil
	}
	var use AdapterCheckUse
	if err := strictJSONPayloadDecode(lines[1], &use, true); err != nil {
		return AdapterCheckUseGenesis{}, nil, err
	}
	useDigest, _ := DigestDomain("adapter-check-use", adapterCheckUseProjection(use))
	if use.SchemaVersion != AdapterCheckReceiptSchema+"/use" ||
		use.AdapterCheckReceiptSHA256 != receipt.ReceiptSHA256 ||
		use.GenesisSHA256 != genesis.GenesisSHA256 ||
		len(use.AuthorizationDirectorySHA256) != 64 ||
		len(use.AuthorizationSHA256) != 64 || use.UsedAt.IsZero() ||
		use.UseSHA256 != useDigest {
		return AdapterCheckUseGenesis{}, nil, fmt.Errorf("adapter-check use entry mismatch")
	}
	return genesis, &use, nil
}

func consumeAdapterCheck(
	directory string,
	request AdapterCheckRequest,
	receipt AdapterCheckReceipt,
	authorization ExecutionAuthorization,
) error {
	path := filepath.Join(directory, "adapter-check-use.jsonl")
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return fmt.Errorf("open adapter-check use ledger")
	}
	defer func() { _ = file.Close() }()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return fmt.Errorf("adapter-check use ledger is locked")
	}
	defer func() { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }()
	if err := verifyOpenLedgerPath(file, path); err != nil {
		return err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return err
	}
	genesis, use, err := validateAdapterCheckUseLedger(data, request, receipt)
	if err != nil {
		return err
	}
	if use != nil {
		return fmt.Errorf("adapter-check receipt has already been used")
	}
	entry := AdapterCheckUse{
		SchemaVersion:                AdapterCheckReceiptSchema + "/use",
		AdapterCheckReceiptSHA256:    receipt.ReceiptSHA256,
		GenesisSHA256:                genesis.GenesisSHA256,
		AuthorizationDirectorySHA256: authorization.AuthorizationDirectorySHA256,
		AuthorizationSHA256:          authorization.AuthorizationSHA256,
		UsedAt:                       time.Now().UTC(),
	}
	entry.UseSHA256, _ = DigestDomain("adapter-check-use", adapterCheckUseProjection(entry))
	line, _ := canonicalJSON(entry)
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return err
	}
	if err := file.Sync(); err != nil {
		return err
	}
	if err := verifyOpenLedgerPath(file, path); err != nil {
		return err
	}
	return syncDirectory(directory)
}
