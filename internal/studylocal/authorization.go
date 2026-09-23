package studylocal

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
	"time"

	"golang.org/x/sys/unix"
)

const AuthorizationAcknowledgement = "I authorize only the pinned local 4B development/calibration schedule with zero network access, zero cloud spend, and no scaling control."

type ExecutionAuthorization struct {
	SchemaVersion                string    `json:"schema_version"`
	CreatedAt                    time.Time `json:"created_at"`
	Acknowledgement              string    `json:"acknowledgement"`
	ImplementationCommit         string    `json:"implementation_commit"`
	ImplementationTree           string    `json:"implementation_tree"`
	MainRef                      string    `json:"main_ref"`
	BuildPackage                 string    `json:"build_package"`
	BuildGoVersion               string    `json:"build_go_version"`
	BuildVCSRevision             string    `json:"build_vcs_revision"`
	BuildVCSModified             bool      `json:"build_vcs_modified"`
	ToolSHA256                   string    `json:"tool_sha256"`
	AdapterSHA256                string    `json:"adapter_sha256"`
	PackageManifestSHA256        string    `json:"package_manifest_sha256"`
	ScheduleSHA256               string    `json:"schedule_sha256"`
	ModelManifestSHA256          string    `json:"model_manifest_sha256"`
	ModelArtifactSHA256          string    `json:"model_artifact_sha256"`
	RuntimeManifestSHA256        string    `json:"runtime_manifest_sha256"`
	RuntimeFullTreeSHA256        string    `json:"runtime_full_tree_sha256"`
	OutputNamespaceSHA256        string    `json:"output_namespace_sha256"`
	AuthorizationDirectorySHA256 string    `json:"authorization_directory_sha256"`
	AttemptNonce                 string    `json:"attempt_nonce"`
	LedgerGenesisSHA256          string    `json:"ledger_genesis_sha256"`
	Observations                 int       `json:"observations"`
	NetworkAllowed               bool      `json:"network_allowed"`
	CloudSpendAuthorized         bool      `json:"cloud_spend_authorized"`
	ScalingControlEnabled        bool      `json:"scaling_control_enabled"`
	AuthorizationSHA256          string    `json:"authorization_sha256"`
}

type AuthorizeOptions struct {
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
	Acknowledgement             string
	CreatedAt                   time.Time
}

var authorizationFiles = []string{
	"authorization.json",
	"attempt-ledger.jsonl",
	"host-preflight.json",
	"model-manifest.json",
	"runtime-manifest.json",
	"SHA256SUMS",
}

type AttemptLedgerGenesis struct {
	SchemaVersion                string `json:"schema_version"`
	AttemptNonce                 string `json:"attempt_nonce"`
	AuthorizationDirectorySHA256 string `json:"authorization_directory_sha256"`
	OutputNamespaceSHA256        string `json:"output_namespace_sha256"`
	ImplementationCommit         string `json:"implementation_commit"`
	ImplementationTree           string `json:"implementation_tree"`
	PackageManifestSHA256        string `json:"package_manifest_sha256"`
	ScheduleSHA256               string `json:"schedule_sha256"`
	ModelManifestSHA256          string `json:"model_manifest_sha256"`
	RuntimeManifestSHA256        string `json:"runtime_manifest_sha256"`
	GenesisSHA256                string `json:"genesis_sha256"`
}

type AttemptLedgerEntry struct {
	SchemaVersion         string    `json:"schema_version"`
	Attempt               int       `json:"attempt"`
	AttemptNonce          string    `json:"attempt_nonce"`
	GenesisSHA256         string    `json:"genesis_sha256"`
	AuthorizationSHA256   string    `json:"authorization_sha256"`
	OutputNamespaceSHA256 string    `json:"output_namespace_sha256"`
	StartedAt             time.Time `json:"started_at"`
	EntrySHA256           string    `json:"entry_sha256"`
}

func Authorize(ctx context.Context, options AuthorizeOptions) (ExecutionAuthorization, error) {
	if options.Acknowledgement != AuthorizationAcknowledgement {
		return ExecutionAuthorization{}, fmt.Errorf("exact local-run acknowledgement is required")
	}
	pkg, err := ValidatePackage(options.PackageDirectory)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	modelManifest, modelBytes, err := ReadModelManifest(options.ModelManifestPath)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if err := ValidateModelManifest(options.ModelDirectory, modelManifest); err != nil {
		return ExecutionAuthorization{}, err
	}
	runtimeManifest, runtimeBytes, err := readRuntimeManifest(options.RuntimeManifestPath)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	verifyOptions := RuntimeManifestOptions{
		RuntimePython: options.RuntimePython, AdapterPath: options.AdapterPath,
		RepositoryRoot: options.RepositoryRoot, ImplementationCommit: "",
		ImplementationTree:      options.TrustedImplementationTree,
		RuntimeTreeManifestPath: options.RuntimeTreeManifestPath,
		BaseTreeManifestPath:    options.BaseTreeManifestPath,
		WheelVerificationPath:   options.WheelVerificationPath,
	}
	commit, tree, err := verifyMergedCleanRepository(options.RepositoryRoot, options.MainRef, options.AdapterPath)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if options.TrustedImplementationCommit == "" || options.TrustedImplementationTree == "" ||
		commit != options.TrustedImplementationCommit || tree != options.TrustedImplementationTree {
		return ExecutionAuthorization{}, fmt.Errorf("checkout does not match the externally reviewed implementation commit and tree")
	}
	build, err := verifyExecutableBuild(commit)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	authorizationOutput, err := safeAbsentOutput(
		options.OutputDirectory, options.RepositoryRoot, options.ModelDirectory,
		filepath.Dir(filepath.Dir(options.RuntimePython)),
	)
	if err != nil {
		return ExecutionAuthorization{}, fmt.Errorf("authorization output: %w", err)
	}
	verifyOptions.ImplementationCommit = commit
	verifyContext, cancel := runtimeManifestTimeoutContext(ctx)
	defer cancel()
	if err := VerifyRuntimeManifest(verifyContext, verifyOptions, runtimeManifest); err != nil {
		return ExecutionAuthorization{}, err
	}
	output, err := safeAbsentOutput(
		options.RunOutputDirectory, options.RepositoryRoot, options.ModelDirectory,
		filepath.Dir(filepath.Dir(options.RuntimePython)),
	)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if output == authorizationOutput {
		return ExecutionAuthorization{}, fmt.Errorf("authorization and run output namespaces must differ")
	}
	host, err := CollectHostPreflight(ctx, filepath.Dir(output), pkg.Protocol.Isolation)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if !host.Eligible {
		return ExecutionAuthorization{}, fmt.Errorf("host preflight refused execution: %s", strings.Join(host.Failures, ","))
	}
	adapter, err := safeRegularAbsolute(options.AdapterPath)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	adapterBytes, err := os.ReadFile(adapter)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	toolPath, err := os.Executable()
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	toolInfo, err := os.Stat(toolPath)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	toolSHA, err := digestRegularFile(toolPath, toolInfo)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	if options.CreatedAt.IsZero() {
		options.CreatedAt = time.Now().UTC()
	}
	nonceBytes := make([]byte, 32)
	if _, err := rand.Read(nonceBytes); err != nil {
		return ExecutionAuthorization{}, err
	}
	nonce := hex.EncodeToString(nonceBytes)
	genesis := AttemptLedgerGenesis{
		SchemaVersion: SchemaVersion + "/attempt-ledger-genesis",
		AttemptNonce:  nonce, AuthorizationDirectorySHA256: DigestBytes([]byte(authorizationOutput)),
		OutputNamespaceSHA256: DigestBytes([]byte(output)),
		ImplementationCommit:  commit, ImplementationTree: tree,
		PackageManifestSHA256: DigestBytes(pkg.Files["package-manifest.json"]),
		ScheduleSHA256:        pkg.Manifest.ScheduleSHA256, ModelManifestSHA256: modelManifest.ManifestSHA256,
		RuntimeManifestSHA256: runtimeManifest.ManifestSHA256,
	}
	genesis.GenesisSHA256, _ = DigestDomain("attempt-ledger-genesis", genesisProjection(genesis))
	authorization := ExecutionAuthorization{
		SchemaVersion: SchemaVersion + "/execution-authorization",
		CreatedAt:     options.CreatedAt.UTC(), Acknowledgement: options.Acknowledgement,
		ImplementationCommit: commit, MainRef: options.MainRef,
		ImplementationTree: tree, BuildPackage: build.Package,
		BuildGoVersion: build.GoVersion, BuildVCSRevision: build.Revision,
		BuildVCSModified: build.Modified,
		ToolSHA256:       toolSHA, AdapterSHA256: DigestBytes(adapterBytes),
		PackageManifestSHA256:        DigestBytes(pkg.Files["package-manifest.json"]),
		ScheduleSHA256:               pkg.Manifest.ScheduleSHA256,
		ModelManifestSHA256:          modelManifest.ManifestSHA256,
		ModelArtifactSHA256:          modelManifest.ArtifactSHA256,
		RuntimeManifestSHA256:        runtimeManifest.ManifestSHA256,
		RuntimeFullTreeSHA256:        runtimeManifest.FullTreeSHA256,
		OutputNamespaceSHA256:        DigestBytes([]byte(output)),
		AuthorizationDirectorySHA256: DigestBytes([]byte(authorizationOutput)),
		AttemptNonce:                 nonce, LedgerGenesisSHA256: genesis.GenesisSHA256,
		Observations: len(pkg.Schedule), NetworkAllowed: false,
		CloudSpendAuthorized: false, ScalingControlEnabled: false,
	}
	authorization.AuthorizationSHA256, _ = DigestDomain("execution-authorization", authorizationProjection(authorization))
	hostBytes, err := encodeHostSnapshot(host)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	authorizationBytes, err := canonicalJSONFile(authorization)
	if err != nil {
		return ExecutionAuthorization{}, err
	}
	files := map[string][]byte{
		"authorization.json": authorizationBytes, "host-preflight.json": hostBytes,
		"model-manifest.json": modelBytes, "runtime-manifest.json": runtimeBytes,
	}
	files["SHA256SUMS"] = renderChecksums(files)
	genesisBytes, _ := canonicalJSON(genesis)
	files["attempt-ledger.jsonl"] = append(genesisBytes, '\n')
	if err := writePrivateDirectory(authorizationOutput, authorizationFiles, files); err != nil {
		return ExecutionAuthorization{}, err
	}
	return authorization, nil
}

func authorizationProjection(authorization ExecutionAuthorization) ExecutionAuthorization {
	authorization.AuthorizationSHA256 = ""
	return authorization
}

func genesisProjection(genesis AttemptLedgerGenesis) AttemptLedgerGenesis {
	genesis.GenesisSHA256 = ""
	return genesis
}

func attemptProjection(attempt AttemptLedgerEntry) AttemptLedgerEntry {
	attempt.EntrySHA256 = ""
	return attempt
}

func LoadAuthorization(directory string) (ExecutionAuthorization, HostSnapshot, ModelManifest, RuntimeManifest, map[string][]byte, error) {
	files, err := readExactPrivateDirectory(directory, authorizationFiles, map[string]bool{"attempt-ledger.jsonl": true})
	if err != nil {
		return ExecutionAuthorization{}, HostSnapshot{}, ModelManifest{}, RuntimeManifest{}, nil, err
	}
	var authorization ExecutionAuthorization
	if err := strictDecode(files["authorization.json"], &authorization); err != nil {
		return ExecutionAuthorization{}, HostSnapshot{}, ModelManifest{}, RuntimeManifest{}, nil, err
	}
	digest, _ := DigestDomain("execution-authorization", authorizationProjection(authorization))
	if authorization.SchemaVersion != SchemaVersion+"/execution-authorization" ||
		authorization.Acknowledgement != AuthorizationAcknowledgement ||
		authorization.AuthorizationSHA256 != digest || authorization.Observations != ObservationsPerModel ||
		authorization.NetworkAllowed || authorization.CloudSpendAuthorized || authorization.ScalingControlEnabled ||
		authorization.ModelArtifactSHA256 != TargetModelArtifactSHA256 ||
		authorization.MainRef != "origin/main" ||
		len(authorization.ImplementationCommit) != 40 || len(authorization.ImplementationTree) != 40 ||
		authorization.BuildPackage != "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control" ||
		authorization.BuildVCSRevision != authorization.ImplementationCommit || authorization.BuildVCSModified ||
		authorization.AuthorizationDirectorySHA256 != DigestBytes([]byte(directory)) ||
		len(authorization.AttemptNonce) != 64 || len(authorization.LedgerGenesisSHA256) != 64 {
		return ExecutionAuthorization{}, HostSnapshot{}, ModelManifest{}, RuntimeManifest{}, nil, fmt.Errorf("invalid execution authorization")
	}
	var host HostSnapshot
	if err := strictDecode(files["host-preflight.json"], &host); err != nil || !host.Eligible {
		return ExecutionAuthorization{}, HostSnapshot{}, ModelManifest{}, RuntimeManifest{}, nil, fmt.Errorf("invalid authorization preflight")
	}
	var model ModelManifest
	if err := strictDecode(files["model-manifest.json"], &model); err != nil ||
		model.ManifestSHA256 != authorization.ModelManifestSHA256 {
		return ExecutionAuthorization{}, HostSnapshot{}, ModelManifest{}, RuntimeManifest{}, nil, fmt.Errorf("authorization model manifest mismatch")
	}
	var runtimeManifest RuntimeManifest
	if err := strictDecode(files["runtime-manifest.json"], &runtimeManifest); err != nil ||
		runtimeManifest.ManifestSHA256 != authorization.RuntimeManifestSHA256 ||
		runtimeManifest.FullTreeSHA256 != authorization.RuntimeFullTreeSHA256 {
		return ExecutionAuthorization{}, HostSnapshot{}, ModelManifest{}, RuntimeManifest{}, nil, fmt.Errorf("authorization runtime manifest mismatch")
	}
	if _, _, err := validateAttemptLedgerBytes(files["attempt-ledger.jsonl"], authorization, false); err != nil {
		return ExecutionAuthorization{}, HostSnapshot{}, ModelManifest{}, RuntimeManifest{}, nil, err
	}
	return authorization, host, model, runtimeManifest, files, nil
}

func consumeAuthorization(directory string, authorization ExecutionAuthorization) (AttemptLedgerEntry, error) {
	path := filepath.Join(directory, "attempt-ledger.jsonl")
	fd, err := unix.Open(path, unix.O_RDWR|unix.O_CLOEXEC|unix.O_NOFOLLOW, 0)
	if err != nil {
		return AttemptLedgerEntry{}, err
	}
	file := os.NewFile(uintptr(fd), path)
	if file == nil {
		_ = unix.Close(fd)
		return AttemptLedgerEntry{}, fmt.Errorf("open authorization attempt ledger")
	}
	defer func() { _ = file.Close() }()
	if err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB); err != nil {
		return AttemptLedgerEntry{}, fmt.Errorf("authorization attempt ledger is locked")
	}
	defer func() { _ = unix.Flock(int(file.Fd()), unix.LOCK_UN) }()
	if err := verifyOpenLedgerPath(file, path); err != nil {
		return AttemptLedgerEntry{}, err
	}
	if _, err := file.Seek(0, io.SeekStart); err != nil {
		return AttemptLedgerEntry{}, err
	}
	data, err := io.ReadAll(file)
	if err != nil {
		return AttemptLedgerEntry{}, err
	}
	if _, attempt, err := validateAttemptLedgerBytes(data, authorization, false); err != nil {
		return AttemptLedgerEntry{}, err
	} else if attempt != nil {
		return AttemptLedgerEntry{}, fmt.Errorf("authorization has already been consumed")
	}
	attempt := AttemptLedgerEntry{
		SchemaVersion: SchemaVersion + "/attempt-ledger-entry", Attempt: 1,
		AttemptNonce: authorization.AttemptNonce, GenesisSHA256: authorization.LedgerGenesisSHA256,
		AuthorizationSHA256:   authorization.AuthorizationSHA256,
		OutputNamespaceSHA256: authorization.OutputNamespaceSHA256, StartedAt: time.Now().UTC(),
	}
	attempt.EntrySHA256, _ = DigestDomain("attempt-ledger-entry", attemptProjection(attempt))
	line, _ := canonicalJSON(attempt)
	if _, err := file.Seek(0, io.SeekEnd); err != nil {
		return AttemptLedgerEntry{}, err
	}
	if _, err := file.Write(append(line, '\n')); err != nil {
		return AttemptLedgerEntry{}, err
	}
	if err := file.Sync(); err != nil {
		return AttemptLedgerEntry{}, err
	}
	if err := verifyOpenLedgerPath(file, path); err != nil {
		return AttemptLedgerEntry{}, err
	}
	if err := syncDirectory(directory); err != nil {
		return AttemptLedgerEntry{}, err
	}
	return attempt, nil
}

func verifyOpenLedgerPath(file *os.File, path string) error {
	openInfo, err := file.Stat()
	if err != nil {
		return err
	}
	if !openInfo.Mode().IsRegular() || openInfo.Mode().Perm()&0o077 != 0 {
		return fmt.Errorf("authorization attempt ledger must be an owner-only regular file")
	}
	if err := requireCurrentOwner(openInfo); err != nil {
		return err
	}
	pathInfo, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if pathInfo.Mode()&os.ModeSymlink != 0 || !os.SameFile(openInfo, pathInfo) {
		return fmt.Errorf("authorization attempt ledger path changed during consumption")
	}
	if err := requireCurrentOwner(pathInfo); err != nil {
		return err
	}
	return nil
}

func validateAttemptLedgerBytes(data []byte, authorization ExecutionAuthorization, requireAttempt bool) (AttemptLedgerGenesis, *AttemptLedgerEntry, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return AttemptLedgerGenesis{}, nil, fmt.Errorf("attempt ledger must end in LF")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) < 1 || len(lines) > 2 {
		return AttemptLedgerGenesis{}, nil, fmt.Errorf("attempt ledger has an invalid entry count")
	}
	var genesis AttemptLedgerGenesis
	if err := strictDecode(lines[0], &genesis); err != nil {
		return AttemptLedgerGenesis{}, nil, err
	}
	digest, _ := DigestDomain("attempt-ledger-genesis", genesisProjection(genesis))
	if genesis.SchemaVersion != SchemaVersion+"/attempt-ledger-genesis" ||
		genesis.GenesisSHA256 != digest || genesis.GenesisSHA256 != authorization.LedgerGenesisSHA256 ||
		genesis.AttemptNonce != authorization.AttemptNonce ||
		genesis.AuthorizationDirectorySHA256 != authorization.AuthorizationDirectorySHA256 ||
		genesis.OutputNamespaceSHA256 != authorization.OutputNamespaceSHA256 ||
		genesis.ImplementationCommit != authorization.ImplementationCommit ||
		genesis.ImplementationTree != authorization.ImplementationTree ||
		genesis.PackageManifestSHA256 != authorization.PackageManifestSHA256 ||
		genesis.ScheduleSHA256 != authorization.ScheduleSHA256 ||
		genesis.ModelManifestSHA256 != authorization.ModelManifestSHA256 ||
		genesis.RuntimeManifestSHA256 != authorization.RuntimeManifestSHA256 {
		return AttemptLedgerGenesis{}, nil, fmt.Errorf("attempt ledger genesis mismatch")
	}
	if len(lines) == 1 {
		if requireAttempt {
			return AttemptLedgerGenesis{}, nil, fmt.Errorf("authorization was never consumed")
		}
		return genesis, nil, nil
	}
	var attempt AttemptLedgerEntry
	if err := strictDecode(lines[1], &attempt); err != nil {
		return AttemptLedgerGenesis{}, nil, err
	}
	attemptDigest, _ := DigestDomain("attempt-ledger-entry", attemptProjection(attempt))
	if attempt.SchemaVersion != SchemaVersion+"/attempt-ledger-entry" || attempt.Attempt != 1 ||
		attempt.AttemptNonce != authorization.AttemptNonce ||
		attempt.GenesisSHA256 != authorization.LedgerGenesisSHA256 ||
		attempt.AuthorizationSHA256 != authorization.AuthorizationSHA256 ||
		attempt.OutputNamespaceSHA256 != authorization.OutputNamespaceSHA256 ||
		attempt.StartedAt.IsZero() || attempt.EntrySHA256 != attemptDigest {
		return AttemptLedgerGenesis{}, nil, fmt.Errorf("attempt ledger entry mismatch")
	}
	return genesis, &attempt, nil
}

func verifyMergedCleanRepository(repositoryRoot, mainRef, adapterPath string) (string, string, error) {
	repository, err := safeExactDirectory(repositoryRoot)
	if err != nil {
		return "", "", err
	}
	if err := requireTrustedPathAncestors(repository); err != nil {
		return "", "", err
	}
	if mainRef != "origin/main" {
		return "", "", fmt.Errorf("execution authorization requires the exact origin/main ref")
	}
	run := func(args ...string) ([]byte, error) {
		command := exec.Command("/usr/bin/git", append([]string{
			"--no-replace-objects", "-c", "core.fsmonitor=false", "-c", "core.hooksPath=/dev/null",
			"-c", "core.attributesFile=/dev/null", "-C", repository,
		}, args...)...)
		command.Env = []string{
			"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=/dev/null", "LC_ALL=C", "LANG=C",
			"GIT_CONFIG_NOSYSTEM=1", "GIT_CONFIG_GLOBAL=/dev/null", "GIT_NO_LAZY_FETCH=1",
			"GIT_NO_REPLACE_OBJECTS=1",
		}
		return command.Output()
	}
	headBytes, err := run("rev-parse", "--verify", "HEAD^{commit}")
	if err != nil {
		return "", "", err
	}
	mainBytes, err := run("rev-parse", "--verify", mainRef+"^{commit}")
	if err != nil {
		return "", "", err
	}
	head := strings.TrimSpace(string(headBytes))
	if head == "" || head != strings.TrimSpace(string(mainBytes)) {
		return "", "", fmt.Errorf("execution requires HEAD to equal the reviewed merged main ref")
	}
	treeBytes, err := run("rev-parse", "--verify", "HEAD^{tree}")
	if err != nil || len(strings.TrimSpace(string(treeBytes))) != 40 {
		return "", "", fmt.Errorf("execution source tree identity is unavailable")
	}
	tree := strings.TrimSpace(string(treeBytes))
	status, err := run("status", "--porcelain=v1", "--untracked-files=no")
	if err != nil || len(status) != 0 {
		return "", "", fmt.Errorf("execution requires a clean tracked worktree")
	}
	adapter, err := filepath.Abs(adapterPath)
	if err != nil {
		return "", "", err
	}
	relative, err := filepath.Rel(repository, adapter)
	if err != nil || !safeRelative(filepath.ToSlash(relative)) {
		return "", "", fmt.Errorf("adapter must be within the repository")
	}
	if err := requireTrustedPathAncestors(adapter); err != nil {
		return "", "", err
	}
	committed, err := run("show", head+":"+filepath.ToSlash(relative))
	if err != nil {
		return "", "", fmt.Errorf("adapter is not committed at HEAD")
	}
	disk, err := os.ReadFile(adapter)
	if err != nil || !bytes.Equal(committed, disk) {
		return "", "", fmt.Errorf("adapter differs from the committed implementation")
	}
	return head, tree, nil
}

type executableBuild struct {
	Package   string
	GoVersion string
	Revision  string
	Modified  bool
}

func verifyExecutableBuild(commit string) (executableBuild, error) {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return executableBuild{}, fmt.Errorf("runner executable build information is unavailable")
	}
	return validateExecutableBuildInfo(info, commit)
}

func validateExecutableBuildInfo(info *debug.BuildInfo, commit string) (executableBuild, error) {
	if info == nil || info.Path != "github.com/Siddhant-K-code/distill/cmd/distill-local-context-control" {
		return executableBuild{}, fmt.Errorf("runner executable entrypoint is not authenticated")
	}
	build := executableBuild{Package: info.Path, GoVersion: info.GoVersion}
	foundRevision, foundModified := false, false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			build.Revision, foundRevision = setting.Value, true
		case "vcs.modified":
			build.Modified, foundModified = setting.Value == "true", true
		}
	}
	if !foundRevision || !foundModified || build.Revision != commit || build.Modified {
		return executableBuild{}, fmt.Errorf("runner executable is not a clean build of the trusted implementation commit")
	}
	return build, nil
}

func safeAbsentOutput(path string, forbidden ...string) (string, error) {
	if path == "" || !filepath.IsAbs(path) || filepath.Clean(path) != path {
		return "", fmt.Errorf("output namespace must be a clean absolute path")
	}
	if _, err := os.Lstat(path); !errors.Is(err, os.ErrNotExist) {
		return "", fmt.Errorf("output namespace must not exist")
	}
	parent, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil || parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() ||
		parentInfo.Mode().Perm()&0o022 != 0 || parent == string(filepath.Separator) {
		return "", fmt.Errorf("output parent must be a trusted non-root directory")
	}
	if err := requireCurrentOwner(parentInfo); err != nil {
		return "", err
	}
	resolved := filepath.Join(parent, filepath.Base(path))
	for _, item := range forbidden {
		if item == "" {
			continue
		}
		root, err := filepath.EvalSymlinks(item)
		if err != nil {
			return "", err
		}
		relative, err := filepath.Rel(root, resolved)
		if err == nil && relative != ".." && !strings.HasPrefix(relative, ".."+string(filepath.Separator)) {
			return "", fmt.Errorf("output namespace overlaps a trusted input")
		}
	}
	return resolved, nil
}

func writePrivateDirectory(directory string, names []string, files map[string][]byte) error {
	if directory == "" || filepath.Clean(directory) != directory {
		return fmt.Errorf("unsafe private directory")
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(directory)
		}
	}()
	for _, name := range names {
		if err := writeExclusive(filepath.Join(directory, name), files[name], 0o600); err != nil {
			return err
		}
	}
	if err := syncDirectory(directory); err != nil {
		return err
	}
	cleanup = false
	return nil
}

func readExactPrivateDirectory(directory string, names []string, mutable map[string]bool) (map[string][]byte, error) {
	info, err := os.Lstat(directory)
	if err != nil || info.Mode()&os.ModeSymlink != 0 || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("private directory must be real and owner-only")
	}
	if err := requireCurrentOwner(info); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return nil, err
	}
	expected := append([]string(nil), names...)
	actual := make([]string, 0, len(entries))
	files := map[string][]byte{}
	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil || entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() ||
			entryInfo.Mode().Perm()&0o077 != 0 {
			return nil, fmt.Errorf("private directory contains an unsafe entry")
		}
		if err := requireCurrentOwner(entryInfo); err != nil {
			return nil, err
		}
		actual = append(actual, entry.Name())
		files[entry.Name()], err = os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return nil, err
		}
	}
	sort.Strings(expected)
	sort.Strings(actual)
	if fmt.Sprint(expected) != fmt.Sprint(actual) {
		return nil, fmt.Errorf("private directory file set mismatch")
	}
	immutableCount := len(names) - 1 - len(mutable)
	checksums, err := parseChecksums(files["SHA256SUMS"], immutableCount)
	if err != nil {
		return nil, err
	}
	for _, name := range names {
		if name != "SHA256SUMS" && !mutable[name] && checksums[name] != DigestBytes(files[name]) {
			return nil, fmt.Errorf("private checksum mismatch for %s", name)
		}
	}
	return files, nil
}
