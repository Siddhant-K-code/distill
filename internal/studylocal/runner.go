package studylocal

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"syscall"
	"time"
)

type RuntimeSample struct {
	CollectedAt               time.Time `json:"collected_at"`
	VMAvailablePercent        float64   `json:"vm_available_percent"`
	MemoryPressureFreePercent float64   `json:"memory_pressure_free_percent"`
	SwapUsedBytes             int64     `json:"swap_used_bytes"`
	DiskFreeBytes             int64     `json:"disk_free_bytes"`
	ProcessRSSBytes           int64     `json:"process_rss_bytes"`
}

type ObservationResourceSummary struct {
	SampleCount             int             `json:"sample_count"`
	MinimumVMAvailablePct   float64         `json:"minimum_vm_available_percent"`
	MinimumPressureFreePct  float64         `json:"minimum_memory_pressure_free_percent"`
	MaximumSwapUsedBytes    int64           `json:"maximum_swap_used_bytes"`
	MaximumSwapGrowthBytes  int64           `json:"maximum_swap_growth_bytes"`
	MinimumDiskFreeBytes    int64           `json:"minimum_disk_free_bytes"`
	MaximumSampledRSSBytes  int64           `json:"maximum_sampled_process_rss_bytes"`
	RSSMeasurementSemantics string          `json:"rss_measurement_semantics"`
	Samples                 []RuntimeSample `json:"samples"`
}

type QuestionReceipt struct {
	QuestionID        string        `json:"question_id"`
	RawResponseSHA256 string        `json:"raw_response_sha256"`
	RawResponseBytes  int           `json:"raw_response_bytes"`
	PromptTokenIDs    []int         `json:"prompt_token_ids"`
	GeneratedTokenIDs []int         `json:"generated_token_ids"`
	PromptTokens      int           `json:"prompt_tokens"`
	GeneratedTokens   int           `json:"generated_tokens"`
	Projection        Projection    `json:"projection"`
	Timing            AdapterTiming `json:"timing"`
	Memory            AdapterMemory `json:"memory"`
	TokensPerSecond   *float64      `json:"tokens_per_second"`
}

type ExecutionReceipt struct {
	SchemaVersion            string                     `json:"schema_version"`
	ObservationID            string                     `json:"observation_id"`
	ScheduleIndex            int                        `json:"schedule_index"`
	ConditionID              string                     `json:"condition_id"`
	BaseID                   string                     `json:"base_id"`
	Dataset                  string                     `json:"dataset"`
	Arm                      string                     `json:"arm"`
	Replicate                int                        `json:"replicate"`
	Status                   string                     `json:"status"`
	RequestSHA256            string                     `json:"request_sha256"`
	AdapterEnvelopeSHA256    string                     `json:"adapter_envelope_sha256,omitempty"`
	AdapterStderrSHA256      string                     `json:"adapter_stderr_sha256"`
	Questions                []QuestionReceipt          `json:"questions"`
	ResourceSummary          ObservationResourceSummary `json:"resource_summary"`
	RequestLatencyNanos      int64                      `json:"request_latency_nanoseconds"`
	StartedAt                time.Time                  `json:"started_at"`
	FinishedAt               time.Time                  `json:"finished_at"`
	Error                    string                     `json:"error,omitempty"`
	AuthorizationSHA256      string                     `json:"authorization_sha256"`
	AttemptNonce             string                     `json:"attempt_nonce"`
	AttemptLedgerEntrySHA256 string                     `json:"attempt_ledger_entry_sha256"`
	ModelManifestSHA256      string                     `json:"model_manifest_sha256"`
	RuntimeManifestSHA256    string                     `json:"runtime_manifest_sha256"`
	ReceiptSHA256            string                     `json:"receipt_sha256"`
}

type RunManifest struct {
	SchemaVersion            string    `json:"schema_version"`
	StartedAt                time.Time `json:"started_at"`
	ImplementationCommit     string    `json:"implementation_commit"`
	AuthorizationSHA256      string    `json:"authorization_sha256"`
	AttemptNonce             string    `json:"attempt_nonce"`
	AttemptLedgerEntrySHA256 string    `json:"attempt_ledger_entry_sha256"`
	PackageManifestSHA256    string    `json:"package_manifest_sha256"`
	ScheduleSHA256           string    `json:"schedule_sha256"`
	ObservationsExpected     int       `json:"observations_expected"`
	AttemptsPerObservation   int       `json:"attempts_per_observation"`
	ModelID                  string    `json:"model_id"`
	ModelRevision            string    `json:"model_revision"`
	ScalingControlEnabled    bool      `json:"scaling_control_enabled"`
	NetworkAllowed           bool      `json:"network_allowed"`
}

type CompletionRecord struct {
	SchemaVersion           string    `json:"schema_version"`
	CompletedAt             time.Time `json:"completed_at"`
	ObservationsExpected    int       `json:"observations_expected"`
	ObservationsRecorded    int       `json:"observations_recorded"`
	Valid                   int       `json:"valid"`
	Malformed               int       `json:"malformed"`
	AdapterErrors           int       `json:"adapter_errors"`
	ReceiptCollectionSHA256 string    `json:"receipt_collection_sha256"`
	RuntimeReverified       bool      `json:"runtime_reverified"`
	ModelReverified         bool      `json:"model_reverified"`
}

type AbortedRecord struct {
	SchemaVersion            string    `json:"schema_version"`
	AbortedAt                time.Time `json:"aborted_at"`
	AuthorizationSHA256      string    `json:"authorization_sha256"`
	AttemptLedgerEntrySHA256 string    `json:"attempt_ledger_entry_sha256"`
	Error                    string    `json:"error"`
}

type RunOptions struct {
	PackageDirectory        string
	AuthorizationDirectory  string
	RunDirectory            string
	ModelDirectory          string
	RuntimePython           string
	RuntimeTreeManifestPath string
	BaseTreeManifestPath    string
	WheelVerificationPath   string
	AdapterPath             string
	RepositoryRoot          string
}

type executionBinding struct {
	Authorization   ExecutionAuthorization
	Attempt         AttemptLedgerEntry
	Host            HostSnapshot
	ModelManifest   ModelManifest
	RuntimeManifest RuntimeManifest
}

type observationMonitor func(context.Context, int, int64, string, IsolationLimits, chan<- error) (ObservationResourceSummary, error)

func Run(ctx context.Context, options RunOptions) (completion CompletionRecord, runErr error) {
	output := ""
	attemptConsumed := false
	attempt := AttemptLedgerEntry{}
	defer func() {
		if attemptConsumed && runErr != nil && output != "" {
			record := AbortedRecord{
				SchemaVersion: SchemaVersion + "/aborted", AbortedAt: time.Now().UTC(),
				AuthorizationSHA256:      attempt.AuthorizationSHA256,
				AttemptLedgerEntrySHA256: attempt.EntrySHA256, Error: runErr.Error(),
			}
			data, _ := canonicalJSONFile(record)
			_ = writeExclusive(filepath.Join(output, "aborted.json"), data, 0o600)
			_ = syncDirectory(output)
		}
	}()
	pkg, err := ValidatePackage(options.PackageDirectory)
	if err != nil {
		return CompletionRecord{}, err
	}
	authorization, _, modelManifest, runtimeManifest, _, err := LoadAuthorization(options.AuthorizationDirectory)
	if err != nil {
		return CompletionRecord{}, err
	}
	if authorization.PackageManifestSHA256 != DigestBytes(pkg.Files["package-manifest.json"]) ||
		authorization.ScheduleSHA256 != pkg.Manifest.ScheduleSHA256 {
		return CompletionRecord{}, fmt.Errorf("authorization does not bind the package")
	}
	if err := ValidateModelManifest(options.ModelDirectory, modelManifest); err != nil {
		return CompletionRecord{}, err
	}
	commit, tree, err := verifyMergedCleanRepository(options.RepositoryRoot, authorization.MainRef, options.AdapterPath)
	if err != nil || commit != authorization.ImplementationCommit || tree != authorization.ImplementationTree {
		return CompletionRecord{}, fmt.Errorf("merged implementation identity changed")
	}
	build, err := verifyExecutableBuild(commit)
	if err != nil || build.Package != authorization.BuildPackage ||
		build.GoVersion != authorization.BuildGoVersion || build.Revision != authorization.BuildVCSRevision ||
		build.Modified != authorization.BuildVCSModified {
		return CompletionRecord{}, fmt.Errorf("runner build provenance changed")
	}
	adapterBytes, err := os.ReadFile(options.AdapterPath)
	if err != nil || DigestBytes(adapterBytes) != authorization.AdapterSHA256 {
		return CompletionRecord{}, fmt.Errorf("adapter identity changed")
	}
	toolPath, err := os.Executable()
	if err != nil {
		return CompletionRecord{}, err
	}
	toolInfo, err := os.Stat(toolPath)
	if err != nil {
		return CompletionRecord{}, err
	}
	toolSHA, err := digestRegularFile(toolPath, toolInfo)
	if err != nil || toolSHA != authorization.ToolSHA256 {
		return CompletionRecord{}, fmt.Errorf("runner binary identity changed")
	}
	output, err = safeAbsentOutput(options.RunDirectory, options.RepositoryRoot, options.ModelDirectory, filepath.Dir(filepath.Dir(options.RuntimePython)))
	if err != nil || DigestBytes([]byte(output)) != authorization.OutputNamespaceSHA256 {
		return CompletionRecord{}, fmt.Errorf("run output namespace does not match authorization")
	}
	verifyOptions := RuntimeManifestOptions{
		RuntimePython: options.RuntimePython, AdapterPath: options.AdapterPath,
		RepositoryRoot: options.RepositoryRoot, ImplementationCommit: commit,
		RuntimeTreeManifestPath: options.RuntimeTreeManifestPath,
		BaseTreeManifestPath:    options.BaseTreeManifestPath,
		WheelVerificationPath:   options.WheelVerificationPath,
	}
	verifyContext, cancel := runtimeManifestTimeoutContext(ctx)
	if err := VerifyRuntimeManifest(verifyContext, verifyOptions, runtimeManifest); err != nil {
		cancel()
		return CompletionRecord{}, err
	}
	cancel()
	host, err := CollectHostPreflight(ctx, filepath.Dir(output), pkg.Protocol.Isolation)
	if err != nil || !host.Eligible {
		return CompletionRecord{}, fmt.Errorf("execution host preflight failed")
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		return CompletionRecord{}, err
	}
	runCreated := true
	defer func() {
		if runCreated {
			_ = os.RemoveAll(output)
		}
	}()
	if err := os.Mkdir(filepath.Join(output, "calls"), 0o700); err != nil {
		return CompletionRecord{}, err
	}
	attempt, err = consumeAuthorization(options.AuthorizationDirectory, authorization)
	if err != nil {
		return CompletionRecord{}, err
	}
	attemptConsumed = true
	runCreated = false
	hostBytes, _ := encodeHostSnapshot(host)
	authBytes, _ := canonicalJSONFile(authorization)
	modelBytes, _ := canonicalJSONFile(modelManifest)
	runtimeBytes, _ := canonicalJSONFile(runtimeManifest)
	runManifest := RunManifest{
		SchemaVersion: SchemaVersion + "/run-manifest", StartedAt: time.Now().UTC(),
		ImplementationCommit: commit, AuthorizationSHA256: authorization.AuthorizationSHA256,
		AttemptNonce: authorization.AttemptNonce, AttemptLedgerEntrySHA256: attempt.EntrySHA256,
		PackageManifestSHA256: authorization.PackageManifestSHA256,
		ScheduleSHA256:        authorization.ScheduleSHA256, ObservationsExpected: len(pkg.Schedule),
		AttemptsPerObservation: 1, ModelID: TargetModelID, ModelRevision: TargetModelRevision,
		ScalingControlEnabled: false, NetworkAllowed: false,
	}
	runManifestBytes, _ := canonicalJSONFile(runManifest)
	for name, data := range map[string][]byte{
		"authorization.json": authBytes, "host-preflight.json": hostBytes,
		"model-manifest.json": modelBytes, "runtime-manifest.json": runtimeBytes,
		"run-manifest.json": runManifestBytes,
	} {
		if err := writeExclusive(filepath.Join(output, name), data, 0o600); err != nil {
			return CompletionRecord{}, err
		}
	}
	caffeinate, err := startCaffeinate()
	if err != nil {
		return CompletionRecord{}, err
	}
	defer stopCaffeinate(caffeinate)
	adapter, err := StartCommandAdapter(ctx, CommandAdapterOptions{
		RuntimePython: options.RuntimePython, AdapterPath: options.AdapterPath,
		RepositoryRoot: options.RepositoryRoot, ImplementationCommit: commit,
		ModelDirectory:          options.ModelDirectory,
		ModelManifestPath:       filepath.Join(output, "model-manifest.json"),
		RuntimeManifestPath:     filepath.Join(output, "runtime-manifest.json"),
		RuntimeTreeManifestPath: options.RuntimeTreeManifestPath,
		BaseTreeManifestPath:    options.BaseTreeManifestPath,
		WheelVerificationPath:   options.WheelVerificationPath,
		OutputNamespace:         filepath.Join(output, "adapter-workspace"),
	})
	if err != nil {
		return CompletionRecord{}, err
	}
	defer func() {
		closeContext, closeCancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer closeCancel()
		_ = adapter.Close(closeContext)
	}()
	ready := adapter.Ready()
	if ready.ModelManifestSHA256 != modelManifest.ManifestSHA256 ||
		ready.RuntimeManifestHash != runtimeManifest.ManifestSHA256 {
		return CompletionRecord{}, fmt.Errorf("adapter ready bindings mismatch")
	}
	completion, err = executeSchedule(ctx, pkg, executionBinding{authorization, attempt, host, modelManifest, runtimeManifest}, output, adapter, monitorRuntime)
	if err != nil {
		return completion, err
	}
	closeContext, closeCancel := context.WithTimeout(ctx, 5*time.Second)
	err = adapter.Close(closeContext)
	closeCancel()
	if err != nil {
		return completion, fmt.Errorf("adapter cleanup: %w", err)
	}
	if err := os.RemoveAll(filepath.Join(output, "tmp")); err != nil {
		return completion, fmt.Errorf("remove isolated runtime cache: %w", err)
	}
	if err := ValidateModelManifest(options.ModelDirectory, modelManifest); err != nil {
		return completion, fmt.Errorf("post-run model verification: %w", err)
	}
	verifyContext, cancel = runtimeManifestTimeoutContext(ctx)
	err = VerifyRuntimeManifest(verifyContext, verifyOptions, runtimeManifest)
	cancel()
	if err != nil {
		return completion, fmt.Errorf("post-run runtime verification: %w", err)
	}
	completion.RuntimeReverified, completion.ModelReverified = true, true
	completionBytes, _ := canonicalJSONFile(completion)
	if err := writeExclusive(filepath.Join(output, "completion.json"), completionBytes, 0o600); err != nil {
		return completion, err
	}
	return completion, nil
}

func executeSchedule(
	ctx context.Context,
	pkg Package,
	binding executionBinding,
	runDirectory string,
	adapter Adapter,
	monitor observationMonitor,
) (CompletionRecord, error) {
	contexts := map[string]Context{}
	conditions := map[string]Condition{}
	for _, context := range pkg.Contexts {
		contexts[context.ConditionID+"\x00"+context.Arm] = context
	}
	for _, condition := range pkg.Corpus.Conditions {
		conditions[condition.ID] = condition
	}
	completion := CompletionRecord{
		SchemaVersion: SchemaVersion + "/completion", ObservationsExpected: len(pkg.Schedule),
	}
	receiptDigests := make([]string, 0, len(pkg.Schedule))
	for _, entry := range pkg.Schedule {
		request, err := BuildAdapterRequest(entry, conditions[entry.ConditionID], contexts[entry.ConditionID+"\x00"+entry.Arm], pkg.Protocol.Generation)
		if err != nil {
			return completion, err
		}
		requestBytes, _ := canonicalJSONFile(request)
		observationContext, cancel := context.WithTimeout(ctx, time.Duration(pkg.Protocol.Isolation.ObservationTimeoutSeconds)*time.Second)
		monitorContext, stopMonitor := context.WithCancel(observationContext)
		type monitorResult struct {
			summary ObservationResourceSummary
			err     error
		}
		monitorDone := make(chan monitorResult, 1)
		monitorReady := make(chan error, 1)
		go func() {
			summary, monitorErr := monitor(monitorContext, adapter.PID(), binding.Host.SwapUsedBytes, runDirectory, pkg.Protocol.Isolation, monitorReady)
			if monitorErr != nil {
				cancel()
			}
			monitorDone <- monitorResult{summary, monitorErr}
		}()
		preflightErr := <-monitorReady
		startedClock := time.Now()
		started := startedClock.UTC()
		var response AdapterResponse
		var envelopeBytes, stderrBytes []byte
		var generateErr error
		if preflightErr == nil {
			response, envelopeBytes, stderrBytes, generateErr = adapter.Generate(observationContext, request)
		} else {
			generateErr = preflightErr
		}
		finishedClock := time.Now()
		finished := finishedClock.UTC()
		stopMonitor()
		monitored := <-monitorDone
		cancel()
		receipt := ExecutionReceipt{
			SchemaVersion: SchemaVersion + "/execution-receipt",
			ObservationID: entry.ObservationID, ScheduleIndex: entry.Index,
			ConditionID: entry.ConditionID, BaseID: entry.BaseID, Dataset: entry.Dataset,
			Arm: entry.Arm, Replicate: entry.Replicate, RequestSHA256: DigestBytes(requestBytes),
			AdapterStderrSHA256: DigestBytes(stderrBytes), ResourceSummary: monitored.summary,
			RequestLatencyNanos: finishedClock.Sub(startedClock).Nanoseconds(), StartedAt: started, FinishedAt: finished,
			AuthorizationSHA256:      binding.Authorization.AuthorizationSHA256,
			AttemptNonce:             binding.Authorization.AttemptNonce,
			AttemptLedgerEntrySHA256: binding.Attempt.EntrySHA256,
			ModelManifestSHA256:      binding.ModelManifest.ManifestSHA256,
			RuntimeManifestSHA256:    binding.RuntimeManifest.ManifestSHA256,
		}
		stopAfter := false
		if generateErr != nil || monitored.err != nil {
			receipt.Status = "adapter_error"
			receipt.Error = errors.Join(generateErr, monitored.err).Error()
			completion.AdapterErrors++
			stopAfter = true
		} else {
			receipt.AdapterEnvelopeSHA256 = DigestBytes(envelopeBytes)
			receipt.Status = "valid"
			for _, result := range response.Questions {
				raw, _ := base64.StdEncoding.DecodeString(result.RawResponseBase64)
				questionReceipt := QuestionReceipt{
					QuestionID: result.QuestionID, RawResponseSHA256: result.RawResponseSHA256,
					RawResponseBytes: len(raw), PromptTokenIDs: append([]int(nil), result.PromptTokenIDs...),
					GeneratedTokenIDs: append([]int(nil), result.GeneratedTokenIDs...),
					PromptTokens:      result.PromptTokens, GeneratedTokens: result.GeneratedTokens,
					Projection: result.Projection, Timing: result.Timing, Memory: result.Memory,
				}
				if result.Timing.DecodeNanoseconds > 0 && result.GeneratedTokens > 1 {
					rate := float64(result.GeneratedTokens-1) / (float64(result.Timing.DecodeNanoseconds) / float64(time.Second))
					questionReceipt.TokensPerSecond = &rate
				}
				if result.Projection.ParseStatus != ParseValid {
					receipt.Status = "malformed"
				}
				if result.Memory.PeakBytes > pkg.Protocol.Isolation.DuringMaximumMLXPeakBytes {
					receipt.Status = "adapter_error"
					receipt.Error = "MLX allocator peak exceeded the frozen bound"
					stopAfter = true
				}
				receipt.Questions = append(receipt.Questions, questionReceipt)
			}
			switch receipt.Status {
			case "valid":
				completion.Valid++
			case "malformed":
				completion.Malformed++
			default:
				completion.AdapterErrors++
			}
		}
		receipt.ReceiptSHA256, _ = DigestDomain("execution-receipt", receiptProjection(receipt))
		if err := writeObservation(runDirectory, entry, requestBytes, response, envelopeBytes, stderrBytes, receipt); err != nil {
			return completion, err
		}
		completion.ObservationsRecorded++
		receiptDigests = append(receiptDigests, receipt.ReceiptSHA256)
		if stopAfter {
			return completion, fmt.Errorf("execution stopped after terminal adapter or resource failure at %s", entry.ObservationID)
		}
	}
	completion.CompletedAt = time.Now().UTC()
	completion.ReceiptCollectionSHA256, _ = DigestDomain("receipt-collection", receiptDigests)
	return completion, nil
}

func receiptProjection(receipt ExecutionReceipt) ExecutionReceipt {
	receipt.ReceiptSHA256 = ""
	return receipt
}

func writeObservation(
	runDirectory string,
	entry ScheduleEntry,
	requestBytes []byte,
	response AdapterResponse,
	envelopeBytes, stderrBytes []byte,
	receipt ExecutionReceipt,
) error {
	calls := filepath.Join(runDirectory, "calls")
	stage := filepath.Join(calls, fmt.Sprintf(".stage-%03d", entry.Index))
	final := filepath.Join(calls, fmt.Sprintf("%03d-%s", entry.Index, entry.ObservationID))
	if err := os.Mkdir(stage, 0o700); err != nil {
		return fmt.Errorf("create observation stage: %w", err)
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stage)
		}
	}()
	receiptBytes, _ := canonicalJSONFile(receipt)
	files := map[string][]byte{
		"request.json": requestBytes, "adapter-stdout.bin": envelopeBytes,
		"adapter-stderr.bin": stderrBytes, "receipt.json": receiptBytes,
	}
	for _, result := range response.Questions {
		raw, err := base64.StdEncoding.DecodeString(result.RawResponseBase64)
		if err != nil {
			return err
		}
		files["raw-"+result.QuestionID+".bin"] = raw
	}
	names := make([]string, 0, len(files))
	for name := range files {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		if err := writeExclusive(filepath.Join(stage, name), files[name], 0o600); err != nil {
			return err
		}
	}
	if err := syncDirectory(stage); err != nil {
		return err
	}
	if err := renameNoReplace(stage, final); err != nil {
		return err
	}
	published = true
	return syncDirectory(calls)
}

func monitorRuntime(ctx context.Context, pid int, initialSwap int64, outputDirectory string, limits IsolationLimits, ready chan<- error) (ObservationResourceSummary, error) {
	summary := ObservationResourceSummary{
		MinimumVMAvailablePct: 101, MinimumPressureFreePct: 101,
		MinimumDiskFreeBytes:    1<<63 - 1,
		RSSMeasurementSemantics: "sampled process RSS every two seconds; not a true high-water mark",
	}
	probe := realSystemProbe{}
	sample := func() error {
		value, err := collectRuntimeSample(ctx, probe, pid, outputDirectory)
		if err != nil {
			return err
		}
		summary.SampleCount++
		if value.VMAvailablePercent < summary.MinimumVMAvailablePct {
			summary.MinimumVMAvailablePct = value.VMAvailablePercent
		}
		if value.MemoryPressureFreePercent < summary.MinimumPressureFreePct {
			summary.MinimumPressureFreePct = value.MemoryPressureFreePercent
		}
		if value.SwapUsedBytes > summary.MaximumSwapUsedBytes {
			summary.MaximumSwapUsedBytes = value.SwapUsedBytes
		}
		growth := value.SwapUsedBytes - initialSwap
		if growth > summary.MaximumSwapGrowthBytes {
			summary.MaximumSwapGrowthBytes = growth
		}
		if value.DiskFreeBytes < summary.MinimumDiskFreeBytes {
			summary.MinimumDiskFreeBytes = value.DiskFreeBytes
		}
		if value.ProcessRSSBytes > summary.MaximumSampledRSSBytes {
			summary.MaximumSampledRSSBytes = value.ProcessRSSBytes
		}
		summary.Samples = append(summary.Samples, value)
		if value.VMAvailablePercent < float64(limits.DuringMinimumFreeMemoryPercent) ||
			value.MemoryPressureFreePercent < float64(limits.DuringMinimumFreeMemoryPercent) ||
			value.SwapUsedBytes > limits.DuringMaximumSwapUsedBytes ||
			growth > limits.DuringMaximumSwapGrowthBytes ||
			value.DiskFreeBytes < limits.DuringMinimumDiskFreeBytes ||
			value.ProcessRSSBytes > limits.DuringMaximumProcessRSSBytes {
			return fmt.Errorf("runtime resource bound exceeded")
		}
		return nil
	}
	if err := sample(); err != nil {
		ready <- err
		return summary, err
	}
	ready <- nil
	ticker := time.NewTicker(time.Duration(limits.ProcessSampleIntervalSeconds) * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			finalContext, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			original := ctx
			ctx = finalContext
			err := sample()
			ctx = original
			cancel()
			if err != nil {
				return summary, fmt.Errorf("post-observation resource sample: %w", err)
			}
			return summary, nil
		case <-ticker.C:
			if err := sample(); err != nil {
				return summary, err
			}
		}
	}
}

func collectRuntimeSample(ctx context.Context, probe systemProbe, pid int, outputDirectory string) (RuntimeSample, error) {
	run := func(name string, args ...string) (string, error) {
		output, err := probe.Run(ctx, name, args...)
		return strings.TrimSpace(string(output)), err
	}
	memoryBytes, err := run("/usr/sbin/sysctl", "-n", "hw.memsize")
	if err != nil {
		return RuntimeSample{}, err
	}
	total, err := strconv.ParseInt(memoryBytes, 10, 64)
	if err != nil {
		return RuntimeSample{}, err
	}
	vm, err := run("/usr/bin/vm_stat")
	if err != nil {
		return RuntimeSample{}, err
	}
	available, err := parseVMAvailablePercent(vm, total)
	if err != nil {
		return RuntimeSample{}, err
	}
	pressure, err := run("/usr/bin/memory_pressure", "-Q")
	if err != nil {
		return RuntimeSample{}, err
	}
	pressurePercent, err := parseMemoryPressureFreePercent(pressure)
	if err != nil {
		return RuntimeSample{}, err
	}
	swap, err := run("/usr/sbin/sysctl", "-n", "vm.swapusage")
	if err != nil {
		return RuntimeSample{}, err
	}
	swapBytes, err := parseSwapUsedBytes(swap)
	if err != nil {
		return RuntimeSample{}, err
	}
	rss, err := run("/bin/ps", "-o", "rss=", "-p", strconv.Itoa(pid))
	if err != nil {
		return RuntimeSample{}, err
	}
	rssKB, err := strconv.ParseInt(strings.TrimSpace(rss), 10, 64)
	if err != nil {
		return RuntimeSample{}, err
	}
	disk, err := probe.StatFS(outputDirectory)
	if err != nil {
		return RuntimeSample{}, err
	}
	return RuntimeSample{
		CollectedAt: time.Now().UTC(), VMAvailablePercent: available,
		MemoryPressureFreePercent: pressurePercent, SwapUsedBytes: swapBytes,
		DiskFreeBytes: disk, ProcessRSSBytes: rssKB * 1024,
	}, nil
}

func startCaffeinate() (*exec.Cmd, error) {
	command := exec.Command("/usr/bin/caffeinate", "-dimsu", "-w", strconv.Itoa(os.Getpid()))
	command.Env = []string{"PATH=/usr/bin:/bin:/usr/sbin:/sbin", "HOME=/dev/null", "LC_ALL=C", "LANG=C"}
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	if err := command.Start(); err != nil {
		return nil, fmt.Errorf("start sleep inhibition: %w", err)
	}
	time.Sleep(50 * time.Millisecond)
	if err := command.Process.Signal(syscall.Signal(0)); err != nil {
		_ = command.Wait()
		return nil, fmt.Errorf("sleep inhibition exited during startup: %w", err)
	}
	return command, nil
}

func stopCaffeinate(command *exec.Cmd) {
	if command == nil || command.Process == nil {
		return
	}
	_ = syscall.Kill(-command.Process.Pid, syscall.SIGTERM)
	done := make(chan struct{})
	go func() {
		_ = command.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		_ = syscall.Kill(-command.Process.Pid, syscall.SIGKILL)
		<-done
	}
}
