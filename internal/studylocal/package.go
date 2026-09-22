package studylocal

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

var packageFiles = []string{
	"contexts.jsonl",
	"corpus.json",
	"local-control-result-schema-v1.json",
	"package-manifest.json",
	"protocol.json",
	"schedule.jsonl",
	"source-registry.json",
	"SHA256SUMS",
}

func BuildArtifacts(temporaryParent string) (map[string][]byte, Summary, error) {
	corpus, err := BuildCorpus()
	if err != nil {
		return nil, Summary{}, err
	}
	protocol, err := BuildProtocol()
	if err != nil {
		return nil, Summary{}, err
	}
	contexts, err := BuildContexts(corpus, temporaryParent)
	if err != nil {
		return nil, Summary{}, err
	}
	schedule, err := BuildSchedule(corpus, protocol, contexts)
	if err != nil {
		return nil, Summary{}, err
	}
	files := map[string][]byte{}
	if files["corpus.json"], err = canonicalJSONFile(corpus); err != nil {
		return nil, Summary{}, err
	}
	if files["protocol.json"], err = canonicalJSONFile(protocol); err != nil {
		return nil, Summary{}, err
	}
	if files["contexts.jsonl"], err = canonicalJSONL(contexts); err != nil {
		return nil, Summary{}, err
	}
	if files["schedule.jsonl"], err = canonicalJSONL(schedule); err != nil {
		return nil, Summary{}, err
	}
	if files["source-registry.json"], err = canonicalJSONFile(corpus.SourceRegistry); err != nil {
		return nil, Summary{}, err
	}
	files["local-control-result-schema-v1.json"] = resultSchema()
	contextsHash := DigestBytes(files["contexts.jsonl"])
	scheduleHash := DigestBytes(files["schedule.jsonl"])
	sourceRegistryHash := DigestBytes(files["source-registry.json"])
	manifest := PackageManifest{
		SchemaVersion: SchemaVersion + "/package-manifest", CorpusSHA256: corpus.CorpusSHA256,
		ProtocolSHA256: protocol.ProtocolSHA256, ContextsSHA256: contextsHash,
		ScheduleSHA256: scheduleHash, ResultSchemaSHA256: DigestBytes(files["local-control-result-schema-v1.json"]),
		SourceRegistryHash: sourceRegistryHash, PackageFileCount: len(packageFiles),
		ProviderCalls: 0, ExecutionAuthorized: false,
	}
	if files["package-manifest.json"], err = canonicalJSONFile(manifest); err != nil {
		return nil, Summary{}, err
	}
	files["SHA256SUMS"] = renderChecksums(files)
	summary := summarizePackage(corpus, protocol, manifest)
	return files, summary, nil
}

func Prepare(outputDirectory string) (Summary, error) {
	parent := filepath.Dir(outputDirectory)
	if outputDirectory == "" || filepath.Clean(outputDirectory) != outputDirectory ||
		filepath.Base(outputDirectory) == "." || filepath.Base(outputDirectory) == string(filepath.Separator) {
		return Summary{}, fmt.Errorf("unsafe output directory")
	}
	if err := os.MkdirAll(parent, 0o755); err != nil {
		return Summary{}, err
	}
	parentInfo, err := os.Lstat(parent)
	if err != nil || parentInfo.Mode()&os.ModeSymlink != 0 || !parentInfo.IsDir() {
		return Summary{}, fmt.Errorf("output parent must be a real directory")
	}
	if err := requireCurrentOwner(parentInfo); err != nil {
		return Summary{}, err
	}
	if _, err := os.Lstat(outputDirectory); err == nil {
		return Summary{}, fmt.Errorf("output directory already exists")
	} else if !errors.Is(err, os.ErrNotExist) {
		return Summary{}, err
	}
	files, summary, err := BuildArtifacts(parent)
	if err != nil {
		return Summary{}, err
	}
	random := make([]byte, 8)
	if _, err := rand.Read(random); err != nil {
		return Summary{}, err
	}
	stage := filepath.Join(parent, "."+filepath.Base(outputDirectory)+".stage-"+hex.EncodeToString(random))
	if err := os.Mkdir(stage, 0o700); err != nil {
		return Summary{}, err
	}
	published := false
	defer func() {
		if !published {
			_ = os.RemoveAll(stage)
		}
	}()
	for _, name := range packageFiles {
		if err := writeExclusive(filepath.Join(stage, name), files[name], 0o600); err != nil {
			return Summary{}, err
		}
	}
	if _, err := ValidatePackage(stage); err != nil {
		return Summary{}, fmt.Errorf("validate staged package: %w", err)
	}
	if err := syncDirectory(stage); err != nil {
		return Summary{}, err
	}
	if err := renameNoReplace(stage, outputDirectory); err != nil {
		return Summary{}, err
	}
	published = true
	if err := syncDirectory(parent); err != nil {
		return Summary{}, err
	}
	return summary, nil
}

func ValidatePackage(directory string) (Package, error) {
	info, err := os.Lstat(directory)
	if err != nil || !info.IsDir() || info.Mode()&os.ModeSymlink != 0 || info.Mode().Perm()&0o077 != 0 {
		return Package{}, fmt.Errorf("package directory must be a real owner-only directory")
	}
	if err := requireCurrentOwner(info); err != nil {
		return Package{}, err
	}
	entries, err := os.ReadDir(directory)
	if err != nil {
		return Package{}, err
	}
	names := make([]string, 0, len(entries))
	files := make(map[string][]byte, len(entries))
	for _, entry := range entries {
		entryInfo, err := entry.Info()
		if err != nil || !entry.Type().IsRegular() || entry.Type()&os.ModeSymlink != 0 ||
			entryInfo.Mode().Perm()&0o077 != 0 {
			return Package{}, fmt.Errorf("package entries must be owner-only regular files")
		}
		if err := requireCurrentOwner(entryInfo); err != nil {
			return Package{}, err
		}
		names = append(names, entry.Name())
		files[entry.Name()], err = os.ReadFile(filepath.Join(directory, entry.Name()))
		if err != nil {
			return Package{}, err
		}
	}
	sort.Strings(names)
	expected := append([]string(nil), packageFiles...)
	sort.Strings(expected)
	if fmt.Sprint(names) != fmt.Sprint(expected) {
		return Package{}, fmt.Errorf("unexpected package files")
	}
	checksums, err := parseChecksums(files["SHA256SUMS"], len(packageFiles)-1)
	if err != nil {
		return Package{}, err
	}
	for _, name := range packageFiles {
		if name != "SHA256SUMS" && checksums[name] != DigestBytes(files[name]) {
			return Package{}, fmt.Errorf("checksum mismatch for %s", name)
		}
	}
	var corpus Corpus
	if err := strictDecode(files["corpus.json"], &corpus); err != nil {
		return Package{}, fmt.Errorf("corpus: %w", err)
	}
	if err := ValidateCorpus(corpus); err != nil {
		return Package{}, err
	}
	expectedCorpus, err := BuildCorpus()
	if err != nil {
		return Package{}, err
	}
	expectedCorpusBytes, err := canonicalJSONFile(expectedCorpus)
	if err != nil || !bytes.Equal(expectedCorpusBytes, files["corpus.json"]) {
		return Package{}, fmt.Errorf("corpus does not match the compiled non-held-out definitions")
	}
	var protocol Protocol
	if err := strictDecode(files["protocol.json"], &protocol); err != nil {
		return Package{}, fmt.Errorf("protocol: %w", err)
	}
	if err := ValidateProtocol(protocol); err != nil {
		return Package{}, err
	}
	expectedProtocol, err := BuildProtocol()
	if err != nil {
		return Package{}, err
	}
	expectedProtocolBytes, err := canonicalJSONFile(expectedProtocol)
	if err != nil || !bytes.Equal(expectedProtocolBytes, files["protocol.json"]) {
		return Package{}, fmt.Errorf("protocol does not match the compiled prospective definition")
	}
	contexts, err := parseJSONL[Context](files["contexts.jsonl"])
	if err != nil {
		return Package{}, fmt.Errorf("contexts: %w", err)
	}
	if err := ValidateContexts(corpus, contexts); err != nil {
		return Package{}, err
	}
	expectedContexts, err := BuildContexts(expectedCorpus, filepath.Dir(directory))
	if err != nil {
		return Package{}, fmt.Errorf("recompile contexts: %w", err)
	}
	expectedContextBytes, err := canonicalJSONL(expectedContexts)
	if err != nil || !bytes.Equal(expectedContextBytes, files["contexts.jsonl"]) {
		return Package{}, fmt.Errorf("contexts do not match independently regenerated arm artifacts")
	}
	schedule, err := parseJSONL[ScheduleEntry](files["schedule.jsonl"])
	if err != nil {
		return Package{}, fmt.Errorf("schedule: %w", err)
	}
	if err := ValidateSchedule(corpus, protocol, contexts, schedule); err != nil {
		return Package{}, err
	}
	expectedSchedule, err := BuildSchedule(expectedCorpus, expectedProtocol, expectedContexts)
	if err != nil {
		return Package{}, err
	}
	expectedScheduleBytes, err := canonicalJSONL(expectedSchedule)
	if err != nil || !bytes.Equal(expectedScheduleBytes, files["schedule.jsonl"]) {
		return Package{}, fmt.Errorf("schedule does not match the compiled 332-observation definition")
	}
	var registry []SourceAnchor
	if err := strictDecode(files["source-registry.json"], &registry); err != nil ||
		fmt.Sprint(registry) != fmt.Sprint(corpus.SourceRegistry) {
		return Package{}, fmt.Errorf("source registry mismatch")
	}
	if !bytes.Equal(files["local-control-result-schema-v1.json"], resultSchema()) {
		return Package{}, fmt.Errorf("result schema drift")
	}
	var manifest PackageManifest
	if err := strictDecode(files["package-manifest.json"], &manifest); err != nil {
		return Package{}, err
	}
	expectedManifest := PackageManifest{
		SchemaVersion: SchemaVersion + "/package-manifest", CorpusSHA256: corpus.CorpusSHA256,
		ProtocolSHA256: protocol.ProtocolSHA256, ContextsSHA256: DigestBytes(files["contexts.jsonl"]),
		ScheduleSHA256:     DigestBytes(files["schedule.jsonl"]),
		ResultSchemaSHA256: DigestBytes(files["local-control-result-schema-v1.json"]),
		SourceRegistryHash: DigestBytes(files["source-registry.json"]),
		PackageFileCount:   len(packageFiles), ProviderCalls: 0, ExecutionAuthorized: false,
	}
	if manifest != expectedManifest {
		return Package{}, fmt.Errorf("package manifest mismatch")
	}
	return Package{Corpus: corpus, Protocol: protocol, Contexts: contexts, Schedule: schedule, Manifest: manifest, Files: files}, nil
}

func Summarize(directory string) (Summary, error) {
	pkg, err := ValidatePackage(directory)
	if err != nil {
		return Summary{}, err
	}
	return summarizePackage(pkg.Corpus, pkg.Protocol, pkg.Manifest), nil
}

func summarizePackage(corpus Corpus, protocol Protocol, manifest PackageManifest) Summary {
	summary := Summary{
		SchemaVersion: SchemaVersion + "/summary", Bases: len(corpus.Bases),
		Conditions: len(corpus.Conditions), PrimaryObservations: protocol.PrimaryObservations,
		RepeatConditions: protocol.RepeatConditionCount, RepeatObservations: protocol.RepeatObservations,
		ObservationsPerModel: protocol.ObservationsPerModel, CorpusSHA256: corpus.CorpusSHA256,
		ProtocolSHA256: protocol.ProtocolSHA256, ContextsSHA256: manifest.ContextsSHA256,
		ScheduleSHA256: manifest.ScheduleSHA256, ExecutionAuthorized: false,
		ScalingControlEnabled: false, HeldOutRecords: 0, ProviderCalls: 0,
	}
	for _, base := range corpus.Bases {
		if base.Dataset == "distill" {
			summary.DistillBases++
		} else {
			summary.LLMTraceFXBases++
		}
	}
	for _, condition := range corpus.Conditions {
		if condition.Dataset == "distill" {
			summary.DistillConditions++
		} else {
			summary.LLMTraceFXConditions++
		}
	}
	return summary
}

func renderChecksums(files map[string][]byte) []byte {
	names := make([]string, 0, len(files))
	for name := range files {
		if name != "SHA256SUMS" {
			names = append(names, name)
		}
	}
	sort.Strings(names)
	var out strings.Builder
	for _, name := range names {
		fmt.Fprintf(&out, "%s  %d  %s\n", DigestBytes(files[name]), len(files[name]), name)
	}
	return []byte(out.String())
}

func parseChecksums(data []byte, expectedCount int) (map[string]string, error) {
	out := map[string]string{}
	scanner := bufio.NewScanner(bytes.NewReader(data))
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), "  ", 3)
		if len(parts) != 3 || len(parts[0]) != 64 || !safeRelative(parts[2]) {
			return nil, fmt.Errorf("malformed checksum line")
		}
		size, err := strconv.Atoi(parts[1])
		if err != nil || size < 0 {
			return nil, fmt.Errorf("invalid checksum size")
		}
		if _, duplicate := out[parts[2]]; duplicate {
			return nil, fmt.Errorf("duplicate checksum")
		}
		out[parts[2]] = parts[0]
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	if len(out) != expectedCount {
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

func syncDirectory(directory string) error {
	file, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()
	return file.Sync()
}

func resultSchema() []byte {
	var value any
	if err := json.Unmarshal([]byte(`{
	  "$schema": "https://json-schema.org/draft/2020-12/schema",
	  "$id": "https://github.com/Siddhant-K-code/distill/research/context-is-a-build-artifact/local-control-result-schema-v1.json",
	  "title": "Local context-control categorical model projection",
	  "type": "object",
	  "additionalProperties": false,
	  "required": ["answer"],
	  "properties": {
	    "answer": {"type": "string"}
	  }
	}`), &value); err != nil {
		panic(err)
	}
	data, err := canonicalJSONFile(value)
	if err != nil {
		panic(err)
	}
	return data
}
