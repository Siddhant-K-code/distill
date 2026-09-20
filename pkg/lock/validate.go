package lock

import (
	"fmt"
	"reflect"
	"sort"
)

func validateLock(lockFile LockFile) error {
	if lockFile.SchemaVersion != LockSchemaVersion {
		return fmt.Errorf("schema_version: got %q, want %q", lockFile.SchemaVersion, LockSchemaVersion)
	}
	if !reflect.DeepEqual(lockFile.Identities, currentIdentities()) {
		return fmt.Errorf("unsupported tool, runtime, or algorithm identities")
	}
	if err := validateConfig(lockFile.Configuration); err != nil {
		return fmt.Errorf("configuration: %w", err)
	}
	configBytes, err := canonicalJSON(lockFile.Configuration)
	if err != nil {
		return err
	}
	if lockFile.ConfigurationSHA256 != digestBytes(configBytes) {
		return fmt.Errorf("configuration_sha256 mismatch")
	}

	includedConfig := stringSet(lockFile.Configuration.Sources)
	excludedConfig := stringSet(lockFile.Configuration.Exclude)
	seenSources := make(map[string]Source, len(lockFile.Sources))
	expectedStats := Stats{SourceCount: len(lockFile.Sources), ChunkCount: len(lockFile.Chunks)}
	for index, source := range lockFile.Sources {
		canonical, err := canonicalPortablePath(source.Path)
		if err != nil || canonical != source.Path {
			return fmt.Errorf("sources[%d].path is not canonical", index)
		}
		if index > 0 && lockFile.Sources[index-1].Path >= source.Path {
			return fmt.Errorf("sources are not strictly sorted by path")
		}
		if !validDigest(source.OriginalSHA256) || !validDigest(source.NormalizedSHA256) {
			return fmt.Errorf("source %q has invalid digest", source.Path)
		}
		if source.OriginalBytes < 0 || source.NormalizedBytes < 0 {
			return fmt.Errorf("source %q has negative byte length", source.Path)
		}
		_, configuredSource := includedConfig[source.Path]
		_, configuredExclude := excludedConfig[source.Path]
		switch {
		case configuredSource && source.Included && source.Reason == "configured_source":
			expectedStats.IncludedCount++
		case configuredExclude && !source.Included && source.Reason == "configured_exclusion":
			expectedStats.ExcludedCount++
		case !configuredSource && !configuredExclude && !source.Included && source.Reason == "not_listed":
			expectedStats.ExcludedCount++
		default:
			return fmt.Errorf("source %q inclusion reason is inconsistent with configuration", source.Path)
		}
		seenSources[source.Path] = source
	}
	for configured := range includedConfig {
		if _, ok := seenSources[configured]; !ok {
			return fmt.Errorf("configured source %q is absent from inventory", configured)
		}
	}
	for configured := range excludedConfig {
		if _, ok := seenSources[configured]; !ok {
			return fmt.Errorf("configured exclusion %q is absent from inventory", configured)
		}
	}

	previousEnd := make(map[string]int64)
	seenIncluded := make(map[string]bool)
	representatives := make(map[string]Chunk)
	remainingTokens := lockFile.Configuration.TokenBudget
	nextSelectionOrder := 0
	for index, chunk := range lockFile.Chunks {
		source, ok := seenSources[chunk.Path]
		if !ok || !source.Included {
			return fmt.Errorf("chunk %d references missing or excluded source %q", index, chunk.Path)
		}
		if index > 0 && !chunkLess(lockFile.Chunks[index-1], chunk) {
			return fmt.Errorf("chunks are not strictly sorted by path and range")
		}
		if chunk.StartByte != previousEnd[chunk.Path] || chunk.EndByte < chunk.StartByte || chunk.EndByte > source.NormalizedBytes {
			return fmt.Errorf("chunk %d has invalid or non-contiguous byte range", index)
		}
		length := chunk.EndByte - chunk.StartByte
		if length > int64(lockFile.Configuration.ChunkBytes) && length > 4 {
			return fmt.Errorf("chunk %d exceeds configured byte limit", index)
		}
		if !validDigest(chunk.SHA256) {
			return fmt.Errorf("chunk %d has invalid digest", index)
		}
		if chunk.EstimatedTokens != estimateTokens(int(length)) {
			return fmt.Errorf("chunk %d token estimate mismatch", index)
		}
		previousEnd[chunk.Path] = chunk.EndByte
		seenIncluded[chunk.Path] = true

		identity := fmt.Sprintf("%s:%d", chunk.SHA256, length)
		if representative, duplicate := representatives[identity]; duplicate {
			expectedReason := fmt.Sprintf("exact_duplicate_of:%s:%d-%d", representative.Path, representative.StartByte, representative.EndByte)
			if chunk.Selected || chunk.SelectionOrder != -1 || chunk.Reason != expectedReason {
				return fmt.Errorf("chunk %d exact-duplicate selection mismatch", index)
			}
			expectedStats.DuplicateCount++
			continue
		}
		representatives[identity] = chunk
		if chunk.EstimatedTokens > remainingTokens {
			if chunk.Selected || chunk.SelectionOrder != -1 || chunk.Reason != "token_budget_exceeded" {
				return fmt.Errorf("chunk %d budget selection mismatch", index)
			}
			continue
		}
		if !chunk.Selected || chunk.SelectionOrder != nextSelectionOrder || chunk.Reason != "selected" {
			return fmt.Errorf("chunk %d selected state mismatch", index)
		}
		expectedStats.SelectedCount++
		expectedStats.SelectedTokens += chunk.EstimatedTokens
		remainingTokens -= chunk.EstimatedTokens
		nextSelectionOrder++
	}
	for path, source := range seenSources {
		if source.Included {
			if !seenIncluded[path] || previousEnd[path] != source.NormalizedBytes {
				return fmt.Errorf("chunks do not cover included source %q", path)
			}
		}
	}
	if !reflect.DeepEqual(lockFile.Stats, expectedStats) {
		return fmt.Errorf("stats mismatch: got %+v, want %+v", lockFile.Stats, expectedStats)
	}
	return nil
}

func chunkLess(left, right Chunk) bool {
	if left.Path != right.Path {
		return left.Path < right.Path
	}
	if left.StartByte != right.StartByte {
		return left.StartByte < right.StartByte
	}
	if left.EndByte != right.EndByte {
		return left.EndByte < right.EndByte
	}
	return left.SHA256 < right.SHA256
}

func selectedChunks(chunks []Chunk) []Chunk {
	result := make([]Chunk, 0)
	for _, chunk := range chunks {
		if chunk.Selected {
			result = append(result, chunk)
		}
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].SelectionOrder < result[j].SelectionOrder
	})
	return result
}
