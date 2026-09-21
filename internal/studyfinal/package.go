package studyfinal

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	contextartifact "github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact"
)

var packageFiles = []string{"corpus.json", "source-registry.json", "receipt-schema.json", "schedule.json", "receipt-registry.json", "authorization-template.json", "agenttrace-contamination-artifact.json", "package-manifest.json", "SHA256SUMS"}

type AuthorizationTemplate struct {
	SchemaVersion            string   `json:"schema_version"`
	Model                    string   `json:"model"`
	ModelSHA256              string   `json:"model_sha256"`
	PricingVersion           string   `json:"pricing_version"`
	PricingSHA256            string   `json:"pricing_sha256"`
	PricingNanoUSDPerToken   int64    `json:"pricing_nano_usd_per_token"`
	OutputPriceNanoUSD       int64    `json:"output_price_nano_usd_per_token"`
	TotalCapUSD              string   `json:"total_cap_usd"`
	PriorSpendUSD            string   `json:"prior_spend_usd"`
	RemainingCapUSD          string   `json:"remaining_cap_usd"`
	TotalCapNanoUSD          int64    `json:"total_cap_nano_usd"`
	PriorSpendNanoUSD        int64    `json:"prior_spend_nano_usd"`
	RemainingCapNanoUSD      int64    `json:"remaining_cap_nano_usd"`
	APIKeyPolicy             string   `json:"api_key_policy"`
	GateAttestationRequired  bool     `json:"gate_attestation_required"`
	RequiredGates            []string `json:"required_gates"`
	RequiredReviews          []string `json:"required_reviews"`
	DefaultExecutionStatus   string   `json:"default_execution_status"`
	CompiledTrustRootVersion string   `json:"compiled_trust_root_version"`
	CompiledTrustRootSHA256  string   `json:"compiled_trust_root_sha256"`
	LedgerWitnessRequired    bool     `json:"ledger_witness_required"`
}

type PackageManifest struct {
	SchemaVersion                         string `json:"schema_version"`
	CorpusSHA256                          string `json:"corpus_sha256"`
	SplitSHA256                           string `json:"split_sha256"`
	ScheduleSHA256                        string `json:"schedule_sha256"`
	QuestionSchemaSHA256                  string `json:"question_schema_sha256"`
	ReceiptRegistrySHA256                 string `json:"receipt_registry_sha256"`
	ReceiptSchemaSHA256                   string `json:"receipt_schema_sha256"`
	SourceRegistrySHA256                  string `json:"source_registry_sha256"`
	Executable                            bool   `json:"executable"`
	ExecutionStatus                       string `json:"execution_status"`
	AgentTraceContaminationStatus         string `json:"agenttrace_contamination_status"`
	AgentTraceContaminationEvidenceSHA256 string `json:"agenttrace_contamination_evidence_sha256"`
	AgentTraceContaminationArtifactPath   string `json:"agenttrace_contamination_artifact_path"`
}

func expectedAuthorizationTemplate() AuthorizationTemplate {
	return AuthorizationTemplate{
		SchemaVersion: SchemaVersion + "/authorization-template", Model: JevModel,
		ModelSHA256: digestBytes([]byte(JevModel)), PricingVersion: PricingVersion,
		PricingSHA256:          digestBytes([]byte(PricingVersion)),
		PricingNanoUSDPerToken: InputPriceNanoUSD, TotalCapNanoUSD: TotalCapNanoUSD,
		OutputPriceNanoUSD: 0, TotalCapUSD: TotalCapUSD, PriorSpendUSD: PriorSpendUSD, RemainingCapUSD: RemainingCapUSD,
		PriorSpendNanoUSD: PriorSpendNanoUSD, RemainingCapNanoUSD: RemainingCapNanoUSD,
		APIKeyPolicy:            "in-memory-only; never serialize, log, or hash",
		GateAttestationRequired: true, RequiredGates: append([]string(nil), requiredExternalGates...),
		RequiredReviews: requiredReviews(), DefaultExecutionStatus: "no-go-without-signed-passed-attestation",
		CompiledTrustRootVersion: compiledTrustRootVersion, CompiledTrustRootSHA256: compiledTrustRootSHA256,
		LedgerWitnessRequired: true,
	}
}

func PreparePackage(destination string, d Dataset, schedule []ScheduleEntry) error {
	if err := ValidateCorpus(d); err != nil {
		return err
	}
	if err := ValidateSchedule(d, schedule); err != nil {
		return err
	}
	if !safeDestination(destination) {
		return fmt.Errorf("unsafe destination")
	}
	parent := filepath.Dir(destination)
	if err := rejectSymlinkPath(parent); err != nil {
		return err
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	stage := filepath.Join(parent, "."+filepath.Base(destination)+".stage-"+hex.EncodeToString(random))
	if err := os.Mkdir(stage, 0o700); err != nil {
		return err
	}
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.RemoveAll(stage)
		}
	}()
	writeJSON := func(name string, value any) error {
		data, err := json.MarshalIndent(value, "", "  ")
		if err != nil {
			return err
		}
		data = append(data, '\n')
		return writeExclusive(filepath.Join(stage, name), data, 0o600)
	}
	if err := writeJSON("corpus.json", d); err != nil {
		return err
	}
	if err := writeExclusive(filepath.Join(stage, "source-registry.json"), d.SourceRegistry, 0o600); err != nil {
		return err
	}
	if err := writeExclusive(filepath.Join(stage, "receipt-schema.json"), contextartifact.FinalReceiptSchema, 0o600); err != nil {
		return err
	}
	if err := writeJSON("schedule.json", schedule); err != nil {
		return err
	}
	receiptRegistry, err := BuildReceiptRegistry(d, schedule)
	if err != nil {
		return err
	}
	if err := writeJSON("receipt-registry.json", receiptRegistry); err != nil {
		return err
	}
	template := expectedAuthorizationTemplate()
	if err := writeJSON("authorization-template.json", template); err != nil {
		return err
	}
	if len(d.AgentTraceContaminationArtifact) > 0 {
		if err := writeExclusive(filepath.Join(stage, "agenttrace-contamination-artifact.json"), append(append([]byte(nil), d.AgentTraceContaminationArtifact...), '\n'), 0o600); err != nil {
			return err
		}
	} else if err := writeJSON("agenttrace-contamination-artifact.json", map[string]string{
		"schema_version": SchemaVersion + "/agenttrace-contamination-absence",
		"status":         "not-provided-default-downgrade",
	}); err != nil {
		return err
	}
	scheduleSHA256, _ := DigestDomain("run-schedule", schedule)
	manifest := PackageManifest{
		SchemaVersion: SchemaVersion + "/package-manifest", CorpusSHA256: d.CorpusSHA256,
		SplitSHA256: d.SplitSHA256, ScheduleSHA256: scheduleSHA256,
		QuestionSchemaSHA256: d.QuestionSchemaSHA256, ReceiptRegistrySHA256: receiptRegistry.RegistrySHA256,
		ReceiptSchemaSHA256: contextartifact.FinalReceiptSchemaSHA256, SourceRegistrySHA256: d.SourceRegistrySHA256,
		Executable: d.Executable, ExecutionStatus: "no-go-without-signed-passed-attestation",
		AgentTraceContaminationStatus:         d.AgentTraceContamination.Status,
		AgentTraceContaminationEvidenceSHA256: d.AgentTraceContamination.EvidenceSHA256,
		AgentTraceContaminationArtifactPath:   d.AgentTraceContamination.ArtifactPath,
	}
	if err := writeJSON("package-manifest.json", manifest); err != nil {
		return err
	}
	var checksum strings.Builder
	for _, name := range packageFiles[:len(packageFiles)-1] {
		data, err := os.ReadFile(filepath.Join(stage, name))
		if err != nil {
			return err
		}
		fmt.Fprintf(&checksum, "%s  %s\n", digestBytes(data), name)
	}
	if err := writeExclusive(filepath.Join(stage, "SHA256SUMS"), []byte(checksum.String()), 0o600); err != nil {
		return err
	}
	if err := syncTree(stage); err != nil {
		return err
	}
	parentDirectory, err := os.Open(parent)
	if err != nil {
		return err
	}
	if err := parentDirectory.Sync(); err != nil {
		_ = parentDirectory.Close()
		return err
	}
	if err := parentDirectory.Close(); err != nil {
		return err
	}
	if err := renameNoReplace(stage, destination); err != nil {
		return err
	}
	cleanup = false
	if dir, err := os.Open(parent); err == nil {
		_ = dir.Sync()
		_ = dir.Close()
	}
	return nil
}

func ValidatePackage(dir string) (Dataset, []ScheduleEntry, error) {
	if !safeDestination(dir) {
		return Dataset{}, nil, fmt.Errorf("unsafe package path")
	}
	if err := rejectSymlinkPath(dir); err != nil {
		return Dataset{}, nil, err
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return Dataset{}, nil, fmt.Errorf("package directory must be owner-only")
	}
	if err := requireCurrentOwner(info); err != nil {
		return Dataset{}, nil, err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Dataset{}, nil, err
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.Type()&os.ModeSymlink != 0 || !entry.Type().IsRegular() {
			return Dataset{}, nil, fmt.Errorf("unexpected non-regular package entry")
		}
		info, err := entry.Info()
		if err != nil || info.Mode().Perm()&0o077 != 0 {
			return Dataset{}, nil, fmt.Errorf("package file is not owner-only")
		}
		if err := requireCurrentOwner(info); err != nil {
			return Dataset{}, nil, err
		}
		names = append(names, entry.Name())
	}
	sort.Strings(names)
	expected := append([]string(nil), packageFiles...)
	sort.Strings(expected)
	if fmt.Sprint(names) != fmt.Sprint(expected) {
		return Dataset{}, nil, fmt.Errorf("unexpected package files")
	}
	checksums, err := readChecksums(filepath.Join(dir, "SHA256SUMS"))
	if err != nil {
		return Dataset{}, nil, err
	}
	for _, name := range packageFiles[:len(packageFiles)-1] {
		data, err := os.ReadFile(filepath.Join(dir, name))
		if err != nil || checksums[name] != digestBytes(data) {
			return Dataset{}, nil, fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	var d Dataset
	file, err := os.Open(filepath.Join(dir, "corpus.json"))
	if err != nil {
		return Dataset{}, nil, err
	}
	err = strictDecode(file, &d)
	_ = file.Close()
	if err != nil {
		return Dataset{}, nil, fmt.Errorf("invalid corpus: %w", err)
	}
	if err := ValidateCorpus(d); err != nil {
		return Dataset{}, nil, fmt.Errorf("invalid corpus: %w", err)
	}
	sourceRegistryBytes, err := os.ReadFile(filepath.Join(dir, "source-registry.json"))
	if err != nil || !bytes.Equal(sourceRegistryBytes, contextartifact.FinalSourceRegistry) ||
		digestBytes(sourceRegistryBytes) != contextartifact.FinalSourceRegistrySHA256 ||
		!bytes.Equal(sourceRegistryBytes, d.SourceRegistry) {
		return Dataset{}, nil, fmt.Errorf("source registry artifact mismatch")
	}
	receiptSchemaBytes, err := os.ReadFile(filepath.Join(dir, "receipt-schema.json"))
	if err != nil || !bytes.Equal(receiptSchemaBytes, contextartifact.FinalReceiptSchema) ||
		digestBytes(receiptSchemaBytes) != contextartifact.FinalReceiptSchemaSHA256 {
		return Dataset{}, nil, fmt.Errorf("receipt schema artifact mismatch")
	}
	var schedule []ScheduleEntry
	file, err = os.Open(filepath.Join(dir, "schedule.json"))
	if err != nil {
		return Dataset{}, nil, err
	}
	err = strictDecode(file, &schedule)
	_ = file.Close()
	if err != nil {
		return Dataset{}, nil, err
	}
	if err := ValidateSchedule(d, schedule); err != nil {
		return Dataset{}, nil, err
	}
	var receiptRegistry ReceiptRegistry
	file, err = os.Open(filepath.Join(dir, "receipt-registry.json"))
	if err != nil {
		return Dataset{}, nil, err
	}
	err = strictDecode(file, &receiptRegistry)
	_ = file.Close()
	if err != nil {
		return Dataset{}, nil, err
	}
	if err := ValidateReceiptRegistry(receiptRegistry, d, schedule); err != nil {
		return Dataset{}, nil, err
	}
	var authorizationTemplate AuthorizationTemplate
	file, err = os.Open(filepath.Join(dir, "authorization-template.json"))
	if err != nil {
		return Dataset{}, nil, err
	}
	err = strictDecode(file, &authorizationTemplate)
	_ = file.Close()
	expectedTemplateBytes, _ := canonicalJSON(expectedAuthorizationTemplate())
	actualTemplateBytes, _ := canonicalJSON(authorizationTemplate)
	if err != nil || !bytes.Equal(actualTemplateBytes, expectedTemplateBytes) ||
		authorizationTemplate.RemainingCapNanoUSD != authorizationTemplate.TotalCapNanoUSD-authorizationTemplate.PriorSpendNanoUSD {
		return Dataset{}, nil, fmt.Errorf("authorization template constants mismatch")
	}
	contaminationBytes, err := os.ReadFile(filepath.Join(dir, "agenttrace-contamination-artifact.json"))
	if err != nil {
		return Dataset{}, nil, err
	}
	contaminationBytes = bytes.TrimSuffix(contaminationBytes, []byte{'\n'})
	if len(d.AgentTraceContaminationArtifact) > 0 {
		if !bytes.Equal(contaminationBytes, d.AgentTraceContaminationArtifact) {
			return Dataset{}, nil, fmt.Errorf("AgentTrace contamination artifact mismatch")
		}
	} else {
		var absence map[string]string
		if err := strictDecode(bytes.NewReader(contaminationBytes), &absence); err != nil ||
			absence["schema_version"] != SchemaVersion+"/agenttrace-contamination-absence" ||
			absence["status"] != "not-provided-default-downgrade" || len(absence) != 2 {
			return Dataset{}, nil, fmt.Errorf("invalid AgentTrace contamination absence record")
		}
	}
	var packageManifest PackageManifest
	file, err = os.Open(filepath.Join(dir, "package-manifest.json"))
	if err != nil {
		return Dataset{}, nil, err
	}
	err = strictDecode(file, &packageManifest)
	_ = file.Close()
	scheduleSHA256, _ := DigestDomain("run-schedule", schedule)
	expectedManifest := PackageManifest{
		SchemaVersion: SchemaVersion + "/package-manifest", CorpusSHA256: d.CorpusSHA256,
		SplitSHA256: d.SplitSHA256, ScheduleSHA256: scheduleSHA256,
		QuestionSchemaSHA256: d.QuestionSchemaSHA256, ReceiptRegistrySHA256: receiptRegistry.RegistrySHA256,
		ReceiptSchemaSHA256: contextartifact.FinalReceiptSchemaSHA256, SourceRegistrySHA256: d.SourceRegistrySHA256,
		Executable: d.Executable, ExecutionStatus: "no-go-without-signed-passed-attestation",
		AgentTraceContaminationStatus:         d.AgentTraceContamination.Status,
		AgentTraceContaminationEvidenceSHA256: d.AgentTraceContamination.EvidenceSHA256,
		AgentTraceContaminationArtifactPath:   d.AgentTraceContamination.ArtifactPath,
	}
	if err != nil || packageManifest != expectedManifest {
		return Dataset{}, nil, fmt.Errorf("package manifest mismatch")
	}
	return d, schedule, nil
}

func readChecksums(path string) (map[string]string, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = file.Close() }()
	out := map[string]string{}
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "  ", 2)
		if len(parts) != 2 || len(parts[0]) != 64 || !safeRelative(parts[1]) {
			return nil, fmt.Errorf("malformed checksum")
		}
		if _, exists := out[parts[1]]; exists {
			return nil, fmt.Errorf("duplicate checksum")
		}
		out[parts[1]] = parts[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) != len(packageFiles)-1 {
		return nil, fmt.Errorf("unexpected checksum count")
	}
	return out, nil
}

func writeExclusive(path string, data []byte, mode os.FileMode) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		return err
	}
	return closeErr
}

func WriteAuthorization(path string, authorization Authorization) error {
	if !safeDestination(path) {
		return fmt.Errorf("unsafe authorization path")
	}
	if err := rejectSymlinkPath(filepath.Dir(path)); err != nil {
		return err
	}
	data, err := json.MarshalIndent(authorization, "", "  ")
	if err != nil {
		return err
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return err
	}
	stage := filepath.Join(filepath.Dir(path), "."+filepath.Base(path)+".stage-"+hex.EncodeToString(random))
	if err := writeExclusive(stage, append(data, '\n'), 0o600); err != nil {
		return err
	}
	defer func() { _ = os.Remove(stage) }()
	if err := os.Link(stage, path); err != nil {
		return err
	}
	dir, err := os.Open(filepath.Dir(path))
	if err != nil {
		return err
	}
	if err := dir.Sync(); err != nil {
		_ = dir.Close()
		return err
	}
	return dir.Close()
}

func syncTree(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		file, err := os.Open(filepath.Join(dir, entry.Name()))
		if err != nil {
			return err
		}
		if err := file.Sync(); err != nil {
			_ = file.Close()
			return err
		}
		if err := file.Close(); err != nil {
			return err
		}
	}
	root, err := os.Open(dir)
	if err != nil {
		return err
	}
	if err := root.Sync(); err != nil {
		_ = root.Close()
		return err
	}
	return root.Close()
}

func safeDestination(p string) bool {
	if p == "" || strings.ContainsRune(p, 0) {
		return false
	}
	clean := filepath.Clean(p)
	if clean != p || clean == "." || clean == string(filepath.Separator) {
		return false
	}
	for _, part := range strings.Split(filepath.ToSlash(clean), "/") {
		if part == ".." {
			return false
		}
	}
	return true
}

func rejectSymlinkPath(p string) error {
	abs, err := filepath.Abs(p)
	if err != nil {
		return err
	}
	current := string(filepath.Separator)
	parts := strings.Split(strings.TrimPrefix(abs, string(filepath.Separator)), string(filepath.Separator))
	for _, part := range parts {
		current = filepath.Join(current, part)
		info, err := os.Lstat(current)
		if os.IsNotExist(err) {
			return nil
		}
		if err != nil {
			return err
		}
		if info.Mode()&os.ModeSymlink != 0 {
			return fmt.Errorf("symlink path rejected")
		}
	}
	return nil
}
