package lock

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"unicode/utf8"
)

// Create reads a canonical configuration and atomically writes its lockfile.
func Create(configPath, outputPath string) (Summary, error) {
	if err := validateSupportedRuntime(); err != nil {
		return Summary{}, err
	}
	configDirectory, err := resolvedDirectory(filepath.Dir(configPath))
	if err != nil {
		return Summary{}, fmt.Errorf("resolve config directory: %w", err)
	}
	configPath = filepath.Join(configDirectory, filepath.Base(configPath))
	config, _, err := loadConfig(configPath)
	if err != nil {
		return Summary{}, err
	}
	sourceRoot := filepath.Join(configDirectory, filepath.FromSlash(config.SourceRoot))
	sourceRoot, err = resolvedDirectory(sourceRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve source root: %w", err)
	}

	outputAbsolute, err := filepath.Abs(outputPath)
	if err != nil {
		return Summary{}, fmt.Errorf("resolve lockfile output: %w", err)
	}
	outputDirectory, err := resolvedDirectory(filepath.Dir(outputAbsolute))
	if err != nil {
		return Summary{}, fmt.Errorf("resolve lockfile directory: %w", err)
	}
	outputAbsolute = filepath.Join(outputDirectory, filepath.Base(outputAbsolute))
	inside, err := isWithin(outputDirectory, sourceRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("compare lockfile and source paths: %w", err)
	}
	if !inside || outputDirectory == sourceRoot {
		return Summary{}, fmt.Errorf("lockfile directory must be a strict ancestor of source root")
	}

	relativeRoot, err := filepath.Rel(outputDirectory, sourceRoot)
	if err != nil {
		return Summary{}, fmt.Errorf("make source root portable: %w", err)
	}
	config.SourceRoot = filepath.ToSlash(relativeRoot)
	if err := validateConfig(config); err != nil {
		return Summary{}, fmt.Errorf("effective configuration: %w", err)
	}

	inputs, err := scanSourceRoot(sourceRoot)
	if err != nil {
		return Summary{}, err
	}
	lockFile, _, err := buildLock(config, inputs)
	if err != nil {
		return Summary{}, err
	}
	lockBytes, err := canonicalJSON(lockFile)
	if err != nil {
		return Summary{}, err
	}
	if err := atomicWriteFile(outputAbsolute, lockBytes); err != nil {
		return Summary{}, fmt.Errorf("publish lockfile: %w", err)
	}

	return summaryFromLock(lockFile, digestBytes(lockBytes)), nil
}

func buildLock(config Config, inputs []sourceInput) (LockFile, map[string][]byte, error) {
	if err := validateConfig(config); err != nil {
		return LockFile{}, nil, err
	}

	sorted := append([]sourceInput(nil), inputs...)
	sort.Slice(sorted, func(i, j int) bool {
		return sorted[i].path < sorted[j].path
	})
	for i := 1; i < len(sorted); i++ {
		if sorted[i-1].path == sorted[i].path {
			return LockFile{}, nil, fmt.Errorf("duplicate canonical source path %q", sorted[i].path)
		}
	}

	included := stringSet(config.Sources)
	excluded := stringSet(config.Exclude)
	found := make(map[string]struct{}, len(sorted))
	normalizedByPath := make(map[string][]byte, len(sorted))
	sources := make([]Source, 0, len(sorted))

	for _, input := range sorted {
		found[input.path] = struct{}{}
		normalizedByPath[input.path] = input.normalized
		source := Source{
			Path:             input.path,
			OriginalSHA256:   digestBytes(input.original),
			OriginalBytes:    int64(len(input.original)),
			NormalizedSHA256: digestBytes(input.normalized),
			NormalizedBytes:  int64(len(input.normalized)),
		}
		if _, ok := included[input.path]; ok {
			source.Included = true
			source.Reason = "configured_source"
		} else if _, ok := excluded[input.path]; ok {
			source.Reason = "configured_exclusion"
		} else {
			source.Reason = "not_listed"
		}
		sources = append(sources, source)
	}
	for _, configured := range append(append([]string(nil), config.Sources...), config.Exclude...) {
		if _, ok := found[configured]; !ok {
			return LockFile{}, nil, fmt.Errorf("configured path %q is missing from source root", configured)
		}
	}

	var chunks []Chunk
	chunkContents := make(map[string][]byte)
	for _, source := range sources {
		if !source.Included {
			continue
		}
		content := normalizedByPath[source.Path]
		ranges := chunkRanges(content, config.ChunkBytes)
		for _, byteRange := range ranges {
			chunkContent := content[byteRange[0]:byteRange[1]]
			chunk := Chunk{
				Path:            source.Path,
				StartByte:       int64(byteRange[0]),
				EndByte:         int64(byteRange[1]),
				SHA256:          digestBytes(chunkContent),
				EstimatedTokens: estimateTokens(len(chunkContent)),
				SelectionOrder:  -1,
			}
			chunks = append(chunks, chunk)
			chunkContents[chunkKey(chunk)] = chunkContent
		}
	}

	stats := Stats{SourceCount: len(sources), ChunkCount: len(chunks)}
	for _, source := range sources {
		if source.Included {
			stats.IncludedCount++
		} else {
			stats.ExcludedCount++
		}
	}

	representatives := make(map[string]Chunk)
	remainingTokens := config.TokenBudget
	for index := range chunks {
		chunk := &chunks[index]
		identity := fmt.Sprintf("%s:%d", chunk.SHA256, chunk.EndByte-chunk.StartByte)
		if representative, ok := representatives[identity]; ok {
			chunk.Reason = fmt.Sprintf("exact_duplicate_of:%s:%d-%d", representative.Path, representative.StartByte, representative.EndByte)
			stats.DuplicateCount++
			continue
		}
		representatives[identity] = *chunk
		if chunk.EstimatedTokens > remainingTokens {
			chunk.Reason = "token_budget_exceeded"
			continue
		}
		chunk.Selected = true
		chunk.SelectionOrder = stats.SelectedCount
		chunk.Reason = "selected"
		stats.SelectedCount++
		stats.SelectedTokens += chunk.EstimatedTokens
		remainingTokens -= chunk.EstimatedTokens
	}

	configBytes, err := canonicalJSON(config)
	if err != nil {
		return LockFile{}, nil, err
	}
	lockFile := LockFile{
		SchemaVersion:       LockSchemaVersion,
		Identities:          currentIdentities(),
		Configuration:       config,
		ConfigurationSHA256: digestBytes(configBytes),
		Sources:             sources,
		Chunks:              chunks,
		Stats:               stats,
	}
	return lockFile, chunkContents, nil
}

func chunkRanges(content []byte, limit int) [][2]int {
	if len(content) == 0 {
		return [][2]int{{0, 0}}
	}

	ranges := make([][2]int, 0, (len(content)+limit-1)/limit)
	for start := 0; start < len(content); {
		end := start + limit
		if end >= len(content) {
			end = len(content)
		} else {
			for end > start && !utf8.RuneStart(content[end]) {
				end--
			}
			if end == start {
				_, size := utf8.DecodeRune(content[start:])
				end = start + size
			}
		}
		ranges = append(ranges, [2]int{start, end})
		start = end
	}
	return ranges
}

func estimateTokens(byteLength int) int {
	return (byteLength + 3) / 4
}

func chunkKey(chunk Chunk) string {
	return fmt.Sprintf("%s\x00%d\x00%d", chunk.Path, chunk.StartByte, chunk.EndByte)
}

func summaryFromLock(lockFile LockFile, lockDigest string) Summary {
	return Summary{
		SourceCount:    lockFile.Stats.SourceCount,
		DuplicateCount: lockFile.Stats.DuplicateCount,
		SelectedCount:  lockFile.Stats.SelectedCount,
		SelectedTokens: lockFile.Stats.SelectedTokens,
		LockSHA256:     lockDigest,
	}
}

func atomicWriteFile(destination string, data []byte) error {
	if info, err := os.Lstat(destination); err == nil && info.IsDir() {
		return fmt.Errorf("destination %q is a directory", destination)
	} else if err != nil && !os.IsNotExist(err) {
		return err
	}

	directory := filepath.Dir(destination)
	temporary, err := os.CreateTemp(directory, "."+filepath.Base(destination)+".tmp-")
	if err != nil {
		return err
	}
	temporaryPath := temporary.Name()
	defer func() {
		_ = os.Remove(temporaryPath)
	}()

	if err := temporary.Chmod(0o644); err != nil {
		_ = temporary.Close()
		return err
	}
	if _, err := temporary.Write(data); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Sync(); err != nil {
		_ = temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Rename(temporaryPath, destination); err != nil {
		return err
	}
	return syncDirectory(directory)
}

func syncDirectory(directory string) error {
	handle, err := os.Open(directory)
	if err != nil {
		return err
	}
	defer func() {
		_ = handle.Close()
	}()
	if err := handle.Sync(); err != nil && !strings.Contains(strings.ToLower(err.Error()), "invalid argument") {
		return err
	}
	return nil
}

func equalLock(left, right LockFile) (bool, error) {
	leftBytes, err := canonicalJSON(left)
	if err != nil {
		return false, err
	}
	rightBytes, err := canonicalJSON(right)
	if err != nil {
		return false, err
	}
	return bytes.Equal(leftBytes, rightBytes), nil
}
