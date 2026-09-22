package studylocal

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	distilllock "github.com/Siddhant-K-code/distill/pkg/lock"
)

func BuildContexts(corpus Corpus, temporaryParent string) ([]Context, error) {
	if err := ValidateCorpus(corpus); err != nil {
		return nil, err
	}
	if temporaryParent != "" {
		resolved, err := filepath.EvalSymlinks(temporaryParent)
		if err != nil {
			return nil, fmt.Errorf("resolve temporary parent: %w", err)
		}
		temporaryParent = resolved
	}
	contexts := make([]Context, 0, len(corpus.Conditions)*2)
	for _, condition := range corpus.Conditions {
		raw, err := compileRaw(condition)
		if err != nil {
			return nil, err
		}
		contexts = append(contexts, raw)
		compiled, err := compileDistillLock(condition, temporaryParent)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", condition.ID, err)
		}
		contexts = append(contexts, compiled)
	}
	sort.Slice(contexts, func(i, j int) bool {
		if contexts[i].ConditionID == contexts[j].ConditionID {
			return contexts[i].Arm < contexts[j].Arm
		}
		return contexts[i].ConditionID < contexts[j].ConditionID
	})
	if err := ValidateContexts(corpus, contexts); err != nil {
		return nil, err
	}
	return contexts, nil
}

func compileRaw(condition Condition) (Context, error) {
	var out strings.Builder
	for _, source := range condition.Sources {
		if !safeRelative(source.Path) || DigestBytes([]byte(source.Bytes)) != source.SHA256 {
			return Context{}, fmt.Errorf("invalid source %s", source.ID)
		}
		fmt.Fprintf(&out, "--- BEGIN SOURCE: %s ---\n", source.Path)
		out.WriteString(source.Bytes)
		if !strings.HasSuffix(source.Bytes, "\n") {
			out.WriteByte('\n')
		}
		fmt.Fprintf(&out, "--- END SOURCE: %s ---\n", source.Path)
	}
	content := out.String()
	return Context{
		ConditionID: condition.ID, Arm: ArmRaw,
		Compiler: "raw-deterministic-concatenation/v1",
		Content:  content, ContentSHA256: DigestBytes([]byte(content)),
	}, nil
}

func compileDistillLock(condition Condition, temporaryParent string) (Context, error) {
	root, err := os.MkdirTemp(temporaryParent, "local-control-lock-")
	if err != nil {
		return Context{}, err
	}
	defer func() { _ = os.RemoveAll(root) }()
	sourceRoot := filepath.Join(root, "sources")
	if err := os.Mkdir(sourceRoot, 0o755); err != nil {
		return Context{}, err
	}
	paths := make([]string, 0, len(condition.Sources))
	for _, source := range condition.Sources {
		if !safeRelative(source.Path) || DigestBytes([]byte(source.Bytes)) != source.SHA256 {
			return Context{}, fmt.Errorf("invalid source %s", source.ID)
		}
		destination := filepath.Join(sourceRoot, filepath.FromSlash(source.Path))
		if err := os.MkdirAll(filepath.Dir(destination), 0o755); err != nil {
			return Context{}, err
		}
		if err := os.WriteFile(destination, []byte(source.Bytes), 0o644); err != nil {
			return Context{}, err
		}
		paths = append(paths, source.Path)
	}
	sort.Strings(paths)
	config := distilllock.Config{
		SchemaVersion: distilllock.ConfigSchemaVersion, SourceRoot: "sources",
		Sources: paths, Exclude: []string{}, ChunkBytes: 1024, TokenBudget: 8192,
		MetadataRemoval: "none",
	}
	configBytes, err := json.MarshalIndent(config, "", "  ")
	if err != nil {
		return Context{}, err
	}
	configBytes = append(configBytes, '\n')
	configPath := filepath.Join(root, "config.json")
	if err := os.WriteFile(configPath, configBytes, 0o644); err != nil {
		return Context{}, err
	}
	lockPath := filepath.Join(root, distilllock.LockFileName)
	if _, err := distilllock.Create(configPath, lockPath); err != nil {
		return Context{}, err
	}
	output := filepath.Join(root, "output")
	summary, err := distilllock.Build(lockPath, output)
	if err != nil {
		return Context{}, err
	}
	bundle, err := os.ReadFile(filepath.Join(output, distilllock.BundleFileName))
	if err != nil {
		return Context{}, err
	}
	checksums, err := os.ReadFile(filepath.Join(output, distilllock.ChecksumsFileName))
	if err != nil {
		return Context{}, err
	}
	manifestBytes, err := os.ReadFile(filepath.Join(output, distilllock.ManifestFileName))
	if err != nil {
		return Context{}, err
	}
	var manifest distilllock.Manifest
	if err := json.Unmarshal(manifestBytes, &manifest); err != nil {
		return Context{}, err
	}
	return Context{
		ConditionID: condition.ID, Arm: ArmDistillLock, Compiler: "distill-lock/v0",
		Content: string(bundle), ContentSHA256: DigestBytes(bundle),
		LockSHA256: summary.LockSHA256, ManifestSHA256: summary.ManifestSHA256,
		ChecksumsSHA256:    DigestBytes(checksums),
		ConfigurationHash:  DigestBytes(configBytes),
		LockSourceCount:    manifest.Stats.SourceCount,
		LockChunkCount:     manifest.Stats.ChunkCount,
		LockDuplicateCount: manifest.Stats.DuplicateCount,
		LockSelectedCount:  manifest.Stats.SelectedCount,
		LockSelectedTokens: manifest.Stats.SelectedTokens,
	}, nil
}

func ValidateContexts(corpus Corpus, contexts []Context) error {
	if len(contexts) != len(corpus.Conditions)*2 {
		return fmt.Errorf("expected %d contexts, got %d", len(corpus.Conditions)*2, len(contexts))
	}
	conditions := make(map[string]Condition, len(corpus.Conditions))
	for _, condition := range corpus.Conditions {
		conditions[condition.ID] = condition
	}
	seen := map[string]bool{}
	for _, context := range contexts {
		condition, ok := conditions[context.ConditionID]
		key := context.ConditionID + "\x00" + context.Arm
		if !ok || seen[key] || (context.Arm != ArmRaw && context.Arm != ArmDistillLock) ||
			DigestBytes([]byte(context.Content)) != context.ContentSHA256 {
			return fmt.Errorf("invalid context %q", key)
		}
		if context.Arm == ArmRaw {
			expected, err := compileRaw(condition)
			if err != nil || expected != context {
				return fmt.Errorf("raw context drift for %s", condition.ID)
			}
		} else if context.Compiler != "distill-lock/v0" || len(context.LockSHA256) != 64 ||
			len(context.ManifestSHA256) != 64 || len(context.ChecksumsSHA256) != 64 ||
			len(context.ConfigurationHash) != 64 || context.LockSourceCount < 1 ||
			context.LockChunkCount < context.LockSelectedCount ||
			context.LockDuplicateCount < 0 || context.LockSelectedTokens < 0 {
			return fmt.Errorf("Distill Lock identity missing for %s", condition.ID)
		}
		seen[key] = true
	}
	return nil
}
