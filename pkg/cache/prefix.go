package cache

import (
	"crypto/sha256"
	"encoding/hex"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// PrefixPartition splits a chunk slice into a frozen cache prefix and a
// dedup-eligible suffix. The split point is the position immediately after
// the last chunk that carries a cache_control marker in its Metadata.
//
// If no cache_control markers are present, the entire slice is returned as
// the suffix (prefix is empty) and FrozenPrefixTokens is 0.
type PrefixPartition struct {
	// Prefix contains chunks that must not be reordered or removed.
	Prefix []types.Chunk

	// Suffix contains chunks eligible for the full dedup pipeline.
	Suffix []types.Chunk

	// PrefixHash is the SHA-256 of the concatenated text of all prefix chunks.
	// Stable across requests when the prefix is unchanged.
	PrefixHash string

	// FrozenPrefixTokens is the estimated token count of the frozen prefix.
	FrozenPrefixTokens int

	// MarkerCount is the number of cache_control markers found in the prefix.
	MarkerCount int
}

// PartitionForCacheAwareDedup partitions chunks for cache-aware deduplication.
// It detects cache_control markers in chunk metadata and freezes everything
// up to and including the last marked chunk.
//
// The caller should run the dedup pipeline only on Partition.Suffix, then
// prepend Partition.Prefix to the result before returning to the client.
func PartitionForCacheAwareDedup(chunks []types.Chunk) PrefixPartition {
	lastMarkerIdx := -1
	markerCount := 0

	for i, c := range chunks {
		if hasCacheControl(c) {
			lastMarkerIdx = i
			markerCount++
		}
	}

	if lastMarkerIdx < 0 {
		// No markers — entire slice is the suffix.
		return PrefixPartition{
			Prefix: nil,
			Suffix: chunks,
		}
	}

	prefix := chunks[:lastMarkerIdx+1]
	suffix := chunks[lastMarkerIdx+1:]

	return PrefixPartition{
		Prefix:             prefix,
		Suffix:             suffix,
		PrefixHash:         hashPrefix(prefix),
		FrozenPrefixTokens: estimatePrefixTokens(prefix),
		MarkerCount:        markerCount,
	}
}

// hasCacheControl returns true when a chunk carries a cache_control marker.
func hasCacheControl(c types.Chunk) bool {
	if c.Metadata == nil {
		return false
	}
	v, ok := c.Metadata["cache_control"]
	if !ok {
		return false
	}
	switch val := v.(type) {
	case string:
		return val != ""
	case map[string]interface{}:
		return len(val) > 0
	case bool:
		return val
	default:
		return v != nil
	}
}

// hashPrefix returns a stable SHA-256 hex digest of the prefix text.
func hashPrefix(chunks []types.Chunk) string {
	h := sha256.New()
	for _, c := range chunks {
		h.Write([]byte(c.Text))
		h.Write([]byte{0}) // null separator
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// estimatePrefixTokens approximates the token count of the prefix using the
// 4-chars-per-token heuristic.
func estimatePrefixTokens(chunks []types.Chunk) int {
	total := 0
	for _, c := range chunks {
		total += (len(c.Text) + 3) / 4
	}
	return total
}

// PrefixAwareStats extends DedupeStats with cache prefix information.
type PrefixAwareStats struct {
	// CachePrefixFrozen is true when a cache prefix was detected and frozen.
	CachePrefixFrozen bool `json:"cache_prefix_frozen,omitempty"`

	// CachePrefixTokens is the estimated token count of the frozen prefix.
	CachePrefixTokens int `json:"cache_prefix_tokens,omitempty"`

	// CachePrefixHash is the stable hash of the frozen prefix.
	CachePrefixHash string `json:"cache_prefix_hash,omitempty"`

	// SuffixInputCount is the number of chunks in the dedup-eligible suffix.
	SuffixInputCount int `json:"suffix_input_count,omitempty"`

	// SuffixOutputCount is the number of chunks after deduplication.
	SuffixOutputCount int `json:"suffix_output_count,omitempty"`
}
