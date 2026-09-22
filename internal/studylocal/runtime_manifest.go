package studylocal

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
	"syscall"
	"time"
)

var expectedRuntimeDistributions = map[string]string{
	"anyio": "4.9.0", "certifi": "2025.7.9", "click": "8.1.8",
	"filelock": "3.32.4", "fsspec": "2026.7.0", "h11": "0.16.0",
	"hf-xet": "1.6.0", "httpcore": "1.0.9", "httpx": "0.28.1",
	"huggingface-hub": "1.13.0", "idna": "3.15", "jinja2": "3.1.6",
	"markdown-it-py": "3.0.0", "markupsafe": "3.0.2", "mdurl": "0.1.2",
	"mlx": "0.32.2", "mlx-lm": "0.31.3", "mlx-metal": "0.32.2",
	"numpy": "2.2.6", "packaging": "26.3", "protobuf": "6.33.5",
	"pygments": "2.20.0", "pyyaml": "6.0.2", "regex": "2026.7.19",
	"rich": "14.0.0", "safetensors": "0.8.0", "sentencepiece": "0.2.2",
	"shellingham": "1.5.4", "sniffio": "1.3.1", "tokenizers": "0.23.1",
	"tqdm": "4.70.0", "transformers": "5.16.1", "typer": "0.16.0",
	"typing-extensions": "4.14.1",
}

type RuntimeManifestOptions struct {
	RuntimePython           string
	AdapterPath             string
	RepositoryRoot          string
	ImplementationCommit    string
	OutputPath              string
	RuntimeTreeManifestPath string
	BaseTreeManifestPath    string
	WheelVerificationPath   string
}

func WriteRuntimeManifest(ctx context.Context, options RuntimeManifestOptions) (RuntimeManifest, error) {
	if runtime.GOOS != "darwin" || runtime.GOARCH != "arm64" {
		return RuntimeManifest{}, fmt.Errorf("MLX runtime manifest requires darwin/arm64")
	}
	python, err := safePythonExecutable(options.RuntimePython)
	if err != nil {
		return RuntimeManifest{}, err
	}
	adapter, err := safeRegularAbsolute(options.AdapterPath)
	if err != nil {
		return RuntimeManifest{}, err
	}
	repository, err := safeExactDirectory(options.RepositoryRoot)
	if err != nil {
		return RuntimeManifest{}, err
	}
	adapterBytes, err := os.ReadFile(adapter)
	if err != nil {
		return RuntimeManifest{}, err
	}
	temporary, err := os.MkdirTemp(filepath.Dir(options.OutputPath), ".local-runtime-manifest-")
	if err != nil {
		return RuntimeManifest{}, err
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	command := exec.CommandContext(
		ctx, "/usr/bin/sandbox-exec", "-p", SandboxPolicy,
		python, "-I", "-B", adapter, "runtime-manifest",
		"--trusted-repo-root", repository,
		"--expected-commit", options.ImplementationCommit,
		"--expected-self-sha256", DigestBytes(adapterBytes),
		"--runtime-tree-manifest", options.RuntimeTreeManifestPath,
		"--base-tree-manifest", options.BaseTreeManifestPath,
		"--wheel-verification", options.WheelVerificationPath,
	)
	command.Dir = "/"
	command.Env = isolatedEnvironment(temporary)
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	var stdout, stderr boundedBuffer
	stdout.limit = maxAdapterLineBytes
	stderr.limit = maxAdapterStderrBytes
	command.Stdout, command.Stderr = &stdout, &stderr
	if err := command.Run(); err != nil {
		return RuntimeManifest{}, fmt.Errorf("runtime manifest probe: %w: %s", err, stderr.String())
	}
	data := bytes.TrimSuffix(stdout.Bytes(), []byte{'\n'})
	var manifest RuntimeManifest
	if err := strictDecode(data, &manifest); err != nil {
		return RuntimeManifest{}, fmt.Errorf("runtime manifest output: %w", err)
	}
	if manifest.BytecodeFiles != 0 || manifest.StartupHooks != 0 || manifest.Symlinks != 0 {
		return RuntimeManifest{}, fmt.Errorf("runtime contains bytecode, startup hooks, or symlinks")
	}
	if _, _, err := validateRuntimeManifestValue(manifest); err != nil {
		return RuntimeManifest{}, err
	}
	if options.OutputPath == "" || filepath.Clean(options.OutputPath) != options.OutputPath {
		return RuntimeManifest{}, fmt.Errorf("unsafe runtime-manifest output")
	}
	if err := writeExclusive(options.OutputPath, append(data, '\n'), 0o600); err != nil {
		return RuntimeManifest{}, err
	}
	return manifest, nil
}

func validateRuntimeManifestValue(manifest RuntimeManifest) (RuntimeManifest, []byte, error) {
	data, err := canonicalJSONFile(manifest)
	if err != nil {
		return RuntimeManifest{}, nil, err
	}
	if manifest.SchemaVersion != RuntimeManifestSchema || manifest.PythonVersion != "3.13.15" ||
		manifest.DistributionCount != 34 || len(manifest.Distributions) != 34 ||
		manifest.FileCount != 5532 || manifest.TotalBytes != 335_118_336 ||
		manifest.VenvDirectoryCount != 831 || manifest.VenvSymlinkCount != 3 ||
		manifest.BaseFileCount != 1942 || manifest.BaseDirectoryCount != 191 ||
		manifest.BaseSymlinkCount != 8 ||
		manifest.FullTreeSHA256 != RuntimeVenvManifestSHA256 ||
		manifest.BaseTreeSHA256 != BasePythonManifestSHA256 ||
		manifest.BasePythonSHA256 != BasePythonBinarySHA256 ||
		manifest.PackageClosureSHA256 != RuntimeClosureSHA256 ||
		manifest.RequirementsInputSHA256 != "4f6fad6a9e96d8ee1efdf59f1bd423c4425647c6d72db83ab9d335f5fea7c21b" ||
		manifest.RequirementsSHA256 != RuntimeRequirementsSHA256 ||
		manifest.WheelVerificationSHA256 != RuntimeWheelVerificationSHA256 ||
		manifest.SitePackagesFileCount < 1 || len(manifest.SitePackagesTreeSHA256) != 64 ||
		manifest.BytecodeFiles != 0 || manifest.StartupHooks != 0 || manifest.Symlinks != 0 {
		return RuntimeManifest{}, nil, fmt.Errorf("runtime manifest violates the clean-runtime contract")
	}
	expectedNames := make([]string, 0, len(expectedRuntimeDistributions))
	for name := range expectedRuntimeDistributions {
		expectedNames = append(expectedNames, name)
	}
	sort.Strings(expectedNames)
	for i, name := range expectedNames {
		if manifest.Distributions[i].Name != name ||
			manifest.Distributions[i].Version != expectedRuntimeDistributions[name] {
			return RuntimeManifest{}, nil, fmt.Errorf("runtime distribution closure mismatch at %s", name)
		}
	}
	projection := manifest
	projection.ManifestSHA256 = ""
	digest, _ := DigestDomain("runtime-manifest", projection)
	if digest != manifest.ManifestSHA256 {
		return RuntimeManifest{}, nil, fmt.Errorf("runtime manifest digest mismatch")
	}
	return manifest, data, nil
}

func VerifyRuntimeManifest(ctx context.Context, options RuntimeManifestOptions, expected RuntimeManifest) error {
	if options.OutputPath != "" {
		return fmt.Errorf("verification does not accept an output path")
	}
	temporary, err := os.CreateTemp("", "local-runtime-manifest-*.json")
	if err != nil {
		return err
	}
	path := temporary.Name()
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Remove(path); err != nil {
		return err
	}
	defer func() { _ = os.Remove(path) }()
	options.OutputPath = path
	actual, err := WriteRuntimeManifest(ctx, options)
	if err != nil {
		return err
	}
	actualBytes, _ := canonicalJSON(actual)
	expectedBytes, _ := canonicalJSON(expected)
	if !bytes.Equal(actualBytes, expectedBytes) {
		return fmt.Errorf("runtime tree changed after its manifest was frozen")
	}
	return nil
}

func runtimeManifestTimeoutContext(parent context.Context) (context.Context, context.CancelFunc) {
	return context.WithTimeout(parent, 12*time.Minute)
}
