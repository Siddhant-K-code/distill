package cache

import (
	"testing"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

func makeChunk(id, text string, cacheControl interface{}) types.Chunk {
	c := types.Chunk{ID: id, Text: text, Metadata: map[string]interface{}{}}
	if cacheControl != nil {
		c.Metadata["cache_control"] = cacheControl
	}
	return c
}

func TestPartitionForCacheAwareDedup_NoMarkers(t *testing.T) {
	chunks := []types.Chunk{
		makeChunk("1", "system prompt", nil),
		makeChunk("2", "user message", nil),
		makeChunk("3", "assistant reply", nil),
	}
	p := PartitionForCacheAwareDedup(chunks)

	if len(p.Prefix) != 0 {
		t.Errorf("expected empty prefix, got %d chunks", len(p.Prefix))
	}
	if len(p.Suffix) != 3 {
		t.Errorf("expected 3-chunk suffix, got %d", len(p.Suffix))
	}
	if p.FrozenPrefixTokens != 0 {
		t.Errorf("expected 0 frozen tokens, got %d", p.FrozenPrefixTokens)
	}
}

func TestPartitionForCacheAwareDedup_SingleMarker(t *testing.T) {
	chunks := []types.Chunk{
		makeChunk("1", "You are a helpful assistant.", map[string]interface{}{"type": "ephemeral"}),
		makeChunk("2", "What is the capital of France?", nil),
		makeChunk("3", "Paris is the capital.", nil),
	}
	p := PartitionForCacheAwareDedup(chunks)

	if len(p.Prefix) != 1 {
		t.Errorf("expected 1-chunk prefix, got %d", len(p.Prefix))
	}
	if p.Prefix[0].ID != "1" {
		t.Errorf("expected prefix chunk id=1, got %s", p.Prefix[0].ID)
	}
	if len(p.Suffix) != 2 {
		t.Errorf("expected 2-chunk suffix, got %d", len(p.Suffix))
	}
	if p.MarkerCount != 1 {
		t.Errorf("expected 1 marker, got %d", p.MarkerCount)
	}
	if p.PrefixHash == "" {
		t.Error("expected non-empty PrefixHash")
	}
	if p.FrozenPrefixTokens <= 0 {
		t.Error("expected positive FrozenPrefixTokens")
	}
}

func TestPartitionForCacheAwareDedup_MultipleMarkers(t *testing.T) {
	chunks := []types.Chunk{
		makeChunk("1", "System prompt text here.", map[string]interface{}{"type": "ephemeral"}),
		makeChunk("2", "Tool definitions JSON schema.", map[string]interface{}{"type": "ephemeral"}),
		makeChunk("3", "Dynamic user message.", nil),
		makeChunk("4", "Another dynamic message.", nil),
	}
	p := PartitionForCacheAwareDedup(chunks)

	// Prefix should include everything up to and including the last marker (chunk 2).
	if len(p.Prefix) != 2 {
		t.Errorf("expected 2-chunk prefix, got %d", len(p.Prefix))
	}
	if len(p.Suffix) != 2 {
		t.Errorf("expected 2-chunk suffix, got %d", len(p.Suffix))
	}
	if p.MarkerCount != 2 {
		t.Errorf("expected 2 markers, got %d", p.MarkerCount)
	}
}

func TestPartitionForCacheAwareDedup_MarkerAtEnd(t *testing.T) {
	chunks := []types.Chunk{
		makeChunk("1", "chunk one", nil),
		makeChunk("2", "chunk two", nil),
		makeChunk("3", "chunk three", "ephemeral"),
	}
	p := PartitionForCacheAwareDedup(chunks)

	// All chunks are in the prefix; suffix is empty.
	if len(p.Prefix) != 3 {
		t.Errorf("expected 3-chunk prefix, got %d", len(p.Prefix))
	}
	if len(p.Suffix) != 0 {
		t.Errorf("expected empty suffix, got %d chunks", len(p.Suffix))
	}
}

func TestPartitionForCacheAwareDedup_HashStability(t *testing.T) {
	chunks := []types.Chunk{
		makeChunk("1", "stable system prompt", map[string]interface{}{"type": "ephemeral"}),
		makeChunk("2", "dynamic content", nil),
	}

	p1 := PartitionForCacheAwareDedup(chunks)
	p2 := PartitionForCacheAwareDedup(chunks)

	if p1.PrefixHash != p2.PrefixHash {
		t.Errorf("hash not stable across calls: %s != %s", p1.PrefixHash, p2.PrefixHash)
	}
}

func TestPartitionForCacheAwareDedup_HashChangesOnTextChange(t *testing.T) {
	chunks1 := []types.Chunk{
		makeChunk("1", "system prompt v1", map[string]interface{}{"type": "ephemeral"}),
	}
	chunks2 := []types.Chunk{
		makeChunk("1", "system prompt v2", map[string]interface{}{"type": "ephemeral"}),
	}

	p1 := PartitionForCacheAwareDedup(chunks1)
	p2 := PartitionForCacheAwareDedup(chunks2)

	if p1.PrefixHash == p2.PrefixHash {
		t.Error("expected different hashes for different prefix text")
	}
}
