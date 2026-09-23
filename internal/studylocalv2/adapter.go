package studylocalv2

import (
	"bufio"
	"bytes"
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sync"
	"syscall"
	"time"
)

const (
	AdapterProtocol       = SchemaVersion + "/mlx-adapter"
	AdapterPreloadSchema  = AdapterProtocol + "/preload-ready"
	AdapterReadySchema    = AdapterProtocol + "/ready"
	AdapterResponseSchema = AdapterProtocol + "/response"
	RuntimeManifestSchema = SchemaVersion + "/runtime-manifest"
	SandboxPolicy         = "(version 1) (allow default) (deny network*)"
	maxAdapterLineBytes   = 8 << 20
	maxAdapterStderrBytes = 1 << 20
)

type RuntimeDistribution struct {
	Name    string `json:"name"`
	Version string `json:"version"`
}

type RuntimeManifest struct {
	SchemaVersion           string                `json:"schema_version"`
	PythonVersion           string                `json:"python_version"`
	DistributionCount       int                   `json:"distribution_count"`
	Distributions           []RuntimeDistribution `json:"distributions"`
	FileCount               int                   `json:"file_count"`
	TotalBytes              int64                 `json:"total_bytes"`
	FullTreeSHA256          string                `json:"full_tree_sha256"`
	VenvDirectoryCount      int                   `json:"venv_directory_count"`
	VenvSymlinkCount        int                   `json:"venv_symlink_count"`
	BaseFileCount           int                   `json:"base_file_count"`
	BaseDirectoryCount      int                   `json:"base_directory_count"`
	BaseSymlinkCount        int                   `json:"base_symlink_count"`
	BaseTreeSHA256          string                `json:"base_tree_sha256"`
	BasePythonSHA256        string                `json:"base_python_sha256"`
	PackageClosureSHA256    string                `json:"package_closure_sha256"`
	RequirementsInputSHA256 string                `json:"requirements_input_sha256"`
	RequirementsSHA256      string                `json:"requirements_lock_sha256"`
	WheelVerificationSHA256 string                `json:"wheel_verification_sha256"`
	SitePackagesFileCount   int                   `json:"site_packages_file_count"`
	SitePackagesTreeSHA256  string                `json:"site_packages_tree_sha256"`
	ManifestSHA256          string                `json:"manifest_sha256"`
	BytecodeFiles           int                   `json:"bytecode_files"`
	StartupHooks            int                   `json:"startup_hooks"`
	Symlinks                int                   `json:"symlinks"`
}

type AdapterReady struct {
	SchemaVersion        string `json:"schema_version"`
	AdapterSHA256        string `json:"adapter_sha256"`
	ImplementationCommit string `json:"implementation_commit"`
	ModelManifestSHA256  string `json:"model_manifest_sha256"`
	RuntimeManifestHash  string `json:"runtime_manifest_sha256"`
	ModelLoaded          bool   `json:"model_loaded"`
	LoadNanoseconds      int64  `json:"load_nanoseconds"`
}

type AdapterPreloadReady struct {
	SchemaVersion             string `json:"schema_version"`
	AdapterSHA256             string `json:"adapter_sha256"`
	ImplementationCommit      string `json:"implementation_commit"`
	PackageManifestSHA256     string `json:"package_manifest_sha256"`
	AuthorizationSHA256       string `json:"authorization_sha256"`
	AdapterCheckReceiptSHA256 string `json:"adapter_check_receipt_sha256"`
	ModelManifestSHA256       string `json:"model_manifest_sha256"`
	RuntimeManifestSHA256     string `json:"runtime_manifest_sha256"`
	OutputNamespaceSHA256     string `json:"output_namespace_sha256"`
	ModelLoaded               bool   `json:"model_loaded"`
}

type AdapterTiming struct {
	TokenizeNanoseconds int64 `json:"tokenize_nanoseconds"`
	TTFTNanoseconds     int64 `json:"ttft_nanoseconds"`
	DecodeNanoseconds   int64 `json:"decode_nanoseconds"`
	TotalNanoseconds    int64 `json:"total_nanoseconds"`
}

type AdapterMemory struct {
	ActiveBytes int64 `json:"active_bytes"`
	CacheBytes  int64 `json:"cache_bytes"`
	PeakBytes   int64 `json:"peak_bytes"`
}

type AdapterQuestionResult struct {
	QuestionID        string        `json:"question_id"`
	RawResponseBase64 string        `json:"raw_response_base64"`
	RawResponseSHA256 string        `json:"raw_response_sha256"`
	PromptTokenIDs    []int         `json:"prompt_token_ids"`
	GeneratedTokenIDs []int         `json:"generated_token_ids"`
	PromptTokens      int           `json:"prompt_tokens"`
	GeneratedTokens   int           `json:"generated_tokens"`
	Projection        Projection    `json:"projection"`
	Timing            AdapterTiming `json:"timing"`
	Memory            AdapterMemory `json:"memory"`
}

type AdapterResponse struct {
	SchemaVersion string                  `json:"schema_version"`
	ObservationID string                  `json:"observation_id"`
	Questions     []AdapterQuestionResult `json:"questions"`
}

type Adapter interface {
	PreloadReady() AdapterPreloadReady
	Load(context.Context) (AdapterReady, error)
	Ready() AdapterReady
	Generate(context.Context, AdapterRequest) (AdapterResponse, []byte, []byte, error)
	Close(context.Context) error
	PID() int
}

type CommandAdapterOptions struct {
	RuntimePython           string
	AdapterPath             string
	RepositoryRoot          string
	ImplementationCommit    string
	ModelDirectory          string
	ModelManifestPath       string
	RuntimeManifestPath     string
	PackageManifestPath     string
	AuthorizationPath       string
	AdapterCheckReceiptPath string
	RuntimeTreeManifestPath string
	BaseTreeManifestPath    string
	WheelVerificationPath   string
	OutputNamespace         string
	RunNamespace            string
	RunManifestPath         string
}

type commandAdapter struct {
	command         *exec.Cmd
	stdin           io.WriteCloser
	lines           <-chan adapterLine
	stderr          *boundedBuffer
	preload         AdapterPreloadReady
	ready           AdapterReady
	runManifestPath string
	closeOnce       sync.Once
}

type adapterLine struct {
	data []byte
	err  error
}

func StartCommandAdapter(ctx context.Context, options CommandAdapterOptions) (Adapter, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return nil, fmt.Errorf("MLX adapter requires darwin/arm64")
	}
	python, err := safePythonExecutable(options.RuntimePython)
	if err != nil {
		return nil, fmt.Errorf("runtime python: %w", err)
	}
	adapterPath, err := safeRegularAbsolute(options.AdapterPath)
	if err != nil {
		return nil, fmt.Errorf("adapter: %w", err)
	}
	adapterBytes, err := os.ReadFile(adapterPath)
	if err != nil {
		return nil, err
	}
	if _, err := safeExactDirectory(options.RepositoryRoot); err != nil {
		return nil, fmt.Errorf("repository root: %w", err)
	}
	if _, err := safeExactDirectory(options.ModelDirectory); err != nil {
		return nil, fmt.Errorf("model directory: %w", err)
	}
	for label, path := range map[string]string{
		"model manifest": options.ModelManifestPath, "runtime manifest": options.RuntimeManifestPath,
		"package manifest": options.PackageManifestPath, "authorization": options.AuthorizationPath,
		"adapter-check receipt": options.AdapterCheckReceiptPath,
	} {
		if _, err := safeRegularAbsolute(path); err != nil {
			return nil, fmt.Errorf("%s: %w", label, err)
		}
	}
	if _, err := safeExactDirectory(options.RunNamespace); err != nil {
		return nil, fmt.Errorf("run namespace: %w", err)
	}
	if options.RunManifestPath == "" || !filepath.IsAbs(options.RunManifestPath) ||
		filepath.Dir(options.RunManifestPath) != options.RunNamespace {
		return nil, fmt.Errorf("run manifest must be an absolute path directly under the run namespace")
	}
	if _, err := os.Lstat(options.RunManifestPath); !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("run manifest must not exist before preload")
	}
	if options.OutputNamespace == "" || !filepath.IsAbs(options.OutputNamespace) {
		return nil, fmt.Errorf("output namespace must be absolute")
	}
	if _, err := os.Lstat(options.OutputNamespace); !errors.Is(err, os.ErrNotExist) {
		return nil, fmt.Errorf("output namespace must not exist")
	}
	args := []string{
		"-p", SandboxPolicy, python, "-I", "-B", "-S", adapterPath, "serve",
		"--trusted-repo-root", options.RepositoryRoot,
		"--expected-commit", options.ImplementationCommit,
		"--expected-self-sha256", DigestBytes(adapterBytes),
		"--model-path", options.ModelDirectory,
		"--model-manifest", options.ModelManifestPath,
		"--runtime-manifest", options.RuntimeManifestPath,
		"--package-manifest", options.PackageManifestPath,
		"--authorization", options.AuthorizationPath,
		"--adapter-check-receipt", options.AdapterCheckReceiptPath,
		"--runtime-tree-manifest", options.RuntimeTreeManifestPath,
		"--base-tree-manifest", options.BaseTreeManifestPath,
		"--wheel-verification", options.WheelVerificationPath,
		"--output-namespace", options.OutputNamespace,
		"--run-namespace", options.RunNamespace,
		"--run-manifest", options.RunManifestPath,
	}
	command := exec.Command("/usr/bin/sandbox-exec", args...)
	command.Dir = "/"
	command.Env = isolatedEnvironment(filepath.Join(filepath.Dir(options.OutputNamespace), "tmp"))
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	stdin, err := command.StdinPipe()
	if err != nil {
		return nil, err
	}
	stdout, err := command.StdoutPipe()
	if err != nil {
		return nil, err
	}
	stderrPipe, err := command.StderrPipe()
	if err != nil {
		return nil, err
	}
	if err := os.Mkdir(filepath.Join(filepath.Dir(options.OutputNamespace), "tmp"), 0o700); err != nil {
		return nil, err
	}
	if err := command.Start(); err != nil {
		return nil, err
	}
	lines := make(chan adapterLine, 1)
	go scanAdapterFrames(stdout, lines)
	stderr := &boundedBuffer{limit: maxAdapterStderrBytes}
	go func() {
		_, _ = io.Copy(stderr, stderrPipe)
	}()
	session := &commandAdapter{
		command: command, stdin: stdin, lines: lines, stderr: stderr,
		runManifestPath: options.RunManifestPath,
	}
	startupContext, cancel := context.WithTimeout(ctx, 12*time.Minute)
	defer cancel()
	line, err := session.nextLine(startupContext)
	if err != nil {
		_ = session.forceStop()
		return nil, fmt.Errorf("adapter startup: %w", err)
	}
	if err := strictJSONPayloadDecode(line, &session.preload, true); err != nil {
		_ = session.forceStop()
		return nil, fmt.Errorf("adapter preload envelope: %w", err)
	}
	if session.preload.SchemaVersion != AdapterPreloadSchema || session.preload.ModelLoaded ||
		session.preload.AdapterSHA256 != DigestBytes(adapterBytes) ||
		session.preload.ImplementationCommit != options.ImplementationCommit {
		_ = session.forceStop()
		return nil, fmt.Errorf("adapter preload identity mismatch")
	}
	return session, nil
}

func (adapter *commandAdapter) PreloadReady() AdapterPreloadReady {
	return adapter.preload
}

func (adapter *commandAdapter) Load(ctx context.Context) (AdapterReady, error) {
	if adapter.ready.ModelLoaded {
		return AdapterReady{}, fmt.Errorf("adapter model is already loaded")
	}
	runManifestBytes, err := os.ReadFile(adapter.runManifestPath)
	if err != nil {
		return AdapterReady{}, fmt.Errorf("read run manifest before load: %w", err)
	}
	var runManifest RunManifest
	if err := strictJSONObjectFileDecode(runManifestBytes, &runManifest); err != nil {
		return AdapterReady{}, fmt.Errorf("validate run manifest before load: %w", err)
	}
	command, err := canonicalJSON(map[string]string{
		"command": "load", "run_manifest_sha256": DigestBytes(runManifestBytes),
	})
	if err != nil {
		return AdapterReady{}, err
	}
	if _, err := adapter.stdin.Write(append(command, '\n')); err != nil {
		return AdapterReady{}, err
	}
	line, err := adapter.nextLine(ctx)
	if err != nil {
		return AdapterReady{}, fmt.Errorf("adapter load: %w", err)
	}
	if err := strictJSONPayloadDecode(line, &adapter.ready, true); err != nil {
		return AdapterReady{}, fmt.Errorf("adapter ready envelope: %w", err)
	}
	if adapter.ready.SchemaVersion != AdapterReadySchema || !adapter.ready.ModelLoaded ||
		adapter.ready.AdapterSHA256 != adapter.preload.AdapterSHA256 ||
		adapter.ready.ImplementationCommit != adapter.preload.ImplementationCommit ||
		adapter.ready.ModelManifestSHA256 != adapter.preload.ModelManifestSHA256 ||
		adapter.ready.RuntimeManifestHash != adapter.preload.RuntimeManifestSHA256 {
		return AdapterReady{}, fmt.Errorf("adapter ready identity mismatch")
	}
	return adapter.ready, nil
}

func (adapter *commandAdapter) Ready() AdapterReady {
	return adapter.ready
}

func (adapter *commandAdapter) PID() int {
	if adapter.command == nil || adapter.command.Process == nil {
		return 0
	}
	return adapter.command.Process.Pid
}

func (adapter *commandAdapter) Generate(ctx context.Context, request AdapterRequest) (AdapterResponse, []byte, []byte, error) {
	if !adapter.ready.ModelLoaded {
		return AdapterResponse{}, nil, nil, fmt.Errorf("adapter model is not loaded")
	}
	stderrOffset := adapter.stderr.Len()
	requestBytes, err := canonicalJSON(request)
	if err != nil {
		return AdapterResponse{}, nil, nil, err
	}
	requestBytes = append(requestBytes, '\n')
	if _, err := adapter.stdin.Write(requestBytes); err != nil {
		return AdapterResponse{}, nil, adapter.stderr.BytesFrom(stderrOffset), err
	}
	rawEnvelope, err := adapter.nextLine(ctx)
	if err != nil {
		return AdapterResponse{}, rawEnvelope, adapter.stderr.BytesFrom(stderrOffset), err
	}
	var response AdapterResponse
	if err := strictJSONPayloadDecode(rawEnvelope, &response, true); err != nil {
		return AdapterResponse{}, rawEnvelope, adapter.stderr.BytesFrom(stderrOffset), fmt.Errorf("adapter response envelope: %w", err)
	}
	if err := validateAdapterResponse(request, response); err != nil {
		return AdapterResponse{}, rawEnvelope, adapter.stderr.BytesFrom(stderrOffset), err
	}
	return response, rawEnvelope, adapter.stderr.BytesFrom(stderrOffset), nil
}

func validateAdapterResponse(request AdapterRequest, response AdapterResponse) error {
	if response.SchemaVersion != AdapterResponseSchema || response.ObservationID != request.ObservationID ||
		len(response.Questions) != len(request.Questions) {
		return fmt.Errorf("adapter response identity mismatch")
	}
	for i, result := range response.Questions {
		question := request.Questions[i]
		if result.QuestionID != question.ID || result.PromptTokens != len(result.PromptTokenIDs) ||
			result.GeneratedTokens != len(result.GeneratedTokenIDs) || result.PromptTokens < 1 ||
			result.GeneratedTokens < 0 || result.GeneratedTokens > request.Generation.MaxOutputTokens ||
			result.Timing.TokenizeNanoseconds < 0 ||
			result.Timing.TTFTNanoseconds < 0 || result.Timing.DecodeNanoseconds < 0 ||
			result.Timing.TotalNanoseconds < 0 || result.Memory.ActiveBytes < 0 ||
			result.Memory.CacheBytes < 0 || result.Memory.PeakBytes < 0 {
			return fmt.Errorf("invalid adapter measurement for %s", question.ID)
		}
		for _, token := range append(append([]int(nil), result.PromptTokenIDs...), result.GeneratedTokenIDs...) {
			if token < 0 {
				return fmt.Errorf("negative token ID for %s", question.ID)
			}
		}
		raw, err := base64.StdEncoding.DecodeString(result.RawResponseBase64)
		if err != nil || DigestBytes(raw) != result.RawResponseSHA256 {
			return fmt.Errorf("raw response identity mismatch for %s", question.ID)
		}
		expected := ParseCategorical(raw, question.Allowed)
		expectedBytes, _ := canonicalJSON(expected)
		actualBytes, _ := canonicalJSON(result.Projection)
		if !bytes.Equal(expectedBytes, actualBytes) {
			return fmt.Errorf("adapter parse projection mismatch for %s", question.ID)
		}
	}
	return nil
}

func (adapter *commandAdapter) Close(ctx context.Context) error {
	var result error
	adapter.closeOnce.Do(func() {
		command, _ := canonicalJSON(map[string]string{"command": "close"})
		_, _ = adapter.stdin.Write(append(command, '\n'))
		_ = adapter.stdin.Close()
		done := make(chan error, 1)
		go func() { done <- adapter.command.Wait() }()
		select {
		case err := <-done:
			result = err
		case <-ctx.Done():
			result = errors.Join(ctx.Err(), adapter.forceStop())
		}
	})
	return result
}

func (adapter *commandAdapter) nextLine(ctx context.Context) ([]byte, error) {
	select {
	case line, ok := <-adapter.lines:
		if !ok {
			return nil, fmt.Errorf("adapter stdout closed: %s", adapter.stderr.String())
		}
		return line.data, line.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}

func (adapter *commandAdapter) forceStop() error {
	if adapter.command == nil || adapter.command.Process == nil {
		return nil
	}
	pid := adapter.command.Process.Pid
	_ = syscall.Kill(-pid, syscall.SIGINT)
	time.Sleep(250 * time.Millisecond)
	if err := syscall.Kill(-pid, 0); err != nil {
		return nil
	}
	_ = syscall.Kill(-pid, syscall.SIGTERM)
	time.Sleep(250 * time.Millisecond)
	if err := syscall.Kill(-pid, 0); err != nil {
		return nil
	}
	return syscall.Kill(-pid, syscall.SIGKILL)
}

func scanAdapterFrames(stream io.Reader, out chan<- adapterLine) {
	defer close(out)
	reader := bufio.NewReaderSize(stream, 64<<10)
	for {
		frame, err := reader.ReadBytes('\n')
		if len(frame) > maxAdapterLineBytes {
			out <- adapterLine{err: fmt.Errorf("adapter frame exceeds %d bytes", maxAdapterLineBytes)}
			return
		}
		if err != nil {
			if len(frame) != 0 {
				out <- adapterLine{err: fmt.Errorf("adapter frame: %w", &framingError{status: FramingMissingLF})}
			} else if !errors.Is(err, io.EOF) {
				out <- adapterLine{err: err}
			}
			return
		}
		var envelope map[string]any
		if err := strictJSONFrameDecode(frame, &envelope); err != nil {
			out <- adapterLine{err: fmt.Errorf("adapter frame: %w", err)}
			return
		}
		out <- adapterLine{data: append([]byte(nil), frame[:len(frame)-1]...)}
	}
}

type boundedBuffer struct {
	mu        sync.Mutex
	data      []byte
	limit     int
	truncated bool
}

func (buffer *boundedBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	remaining := buffer.limit - len(buffer.data)
	if remaining > 0 {
		if len(data) < remaining {
			remaining = len(data)
		}
		buffer.data = append(buffer.data, data[:remaining]...)
	}
	if remaining < len(data) {
		buffer.truncated = true
	}
	return len(data), nil
}

func (buffer *boundedBuffer) Bytes() []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return append([]byte(nil), buffer.data...)
}

func (buffer *boundedBuffer) Len() int {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	return len(buffer.data)
}

func (buffer *boundedBuffer) BytesFrom(offset int) []byte {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	if offset < 0 || offset > len(buffer.data) {
		offset = len(buffer.data)
	}
	return append([]byte(nil), buffer.data[offset:]...)
}

func (buffer *boundedBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	data := append([]byte(nil), buffer.data...)
	if buffer.truncated {
		return string(data) + "...[truncated]"
	}
	return string(data)
}

func isolatedEnvironment(temporaryDirectory string) []string {
	return []string{
		"PATH=/usr/bin:/bin:/usr/sbin:/sbin",
		"HOME=/dev/null",
		"LC_ALL=C",
		"LANG=C",
		"TMPDIR=" + temporaryDirectory,
		"HF_HOME=" + filepath.Join(temporaryDirectory, "huggingface"),
		"TRANSFORMERS_CACHE=" + filepath.Join(temporaryDirectory, "transformers"),
		"HF_DATASETS_CACHE=" + filepath.Join(temporaryDirectory, "datasets"),
		"XDG_CACHE_HOME=" + filepath.Join(temporaryDirectory, "xdg-cache"),
		"HF_HUB_OFFLINE=1",
		"TRANSFORMERS_OFFLINE=1",
		"HF_DATASETS_OFFLINE=1",
		"HF_HUB_DISABLE_TELEMETRY=1",
		"DO_NOT_TRACK=1",
		"PYTHONNOUSERSITE=1",
		"PYTHONSAFEPATH=1",
		"PYTHONDONTWRITEBYTECODE=1",
		"PYTHONHASHSEED=0",
		"TOKENIZERS_PARALLELISM=false",
		"NO_PROXY=*",
		"no_proxy=*",
	}
}

func safeRegularAbsolute(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute path required")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Mode().Perm()&0o022 != 0 {
		return "", fmt.Errorf("path is missing, symlinked, non-regular, or group/world writable")
	}
	if err := requireCurrentOwner(info); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil || resolved != path {
		return "", fmt.Errorf("path contains a symbolic link")
	}
	return path, nil
}

func safePythonExecutable(path string) (string, error) {
	if path == "" || !filepath.IsAbs(path) {
		return "", fmt.Errorf("absolute runtime python path required")
	}
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&0o022 != 0 {
		return "", fmt.Errorf("runtime python is missing or group/world writable")
	}
	if err := requireCurrentOwner(info); err != nil {
		return "", err
	}
	resolved, err := filepath.EvalSymlinks(path)
	if err != nil {
		return "", err
	}
	resolvedInfo, err := os.Stat(resolved)
	if err != nil || !resolvedInfo.Mode().IsRegular() || resolvedInfo.Mode()&0o022 != 0 {
		return "", fmt.Errorf("runtime python target is not a trusted regular file")
	}
	if err := requireCurrentOwner(resolvedInfo); err != nil {
		return "", err
	}
	return path, nil
}

func readRuntimeManifest(path string) (RuntimeManifest, []byte, error) {
	info, err := os.Lstat(path)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() ||
		info.Mode().Perm()&0o077 != 0 {
		return RuntimeManifest{}, nil, fmt.Errorf("runtime manifest must be an owner-only regular file")
	}
	if err := requireCurrentOwner(info); err != nil {
		return RuntimeManifest{}, nil, err
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return RuntimeManifest{}, nil, err
	}
	var manifest RuntimeManifest
	if err := strictJSONObjectFileDecode(data, &manifest); err != nil {
		return RuntimeManifest{}, nil, err
	}
	if _, _, err := validateRuntimeManifestValue(manifest); err != nil {
		return RuntimeManifest{}, nil, err
	}
	return manifest, data, nil
}
