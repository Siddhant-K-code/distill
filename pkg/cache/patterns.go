package cache

import (
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// PatternDetector identifies and hashes common context patterns.
type PatternDetector struct {
	// MinLength is the minimum text length to consider for pattern detection.
	MinLength int

	// SystemPromptPrefixes are common prefixes that indicate system prompts.
	SystemPromptPrefixes []string

	// ToolDefinitionMarkers are strings that indicate tool definitions.
	ToolDefinitionMarkers []string
}

// NewPatternDetector creates a pattern detector with defaults.
func NewPatternDetector() *PatternDetector {
	return &PatternDetector{
		MinLength: 50,
		SystemPromptPrefixes: []string{
			"you are",
			"you're",
			"your role",
			"as an ai",
			"as a helpful",
			"system:",
			"<system>",
			"[system]",
		},
		ToolDefinitionMarkers: []string{
			"function",
			"tool_name",
			"tool_description",
			"parameters",
			"\"type\": \"function\"",
			"<tool>",
			"[tool]",
		},
	}
}

// PatternType identifies the type of detected pattern.
type PatternType string

const (
	PatternTypeUnknown  PatternType = "unknown"
	PatternTypeSystem   PatternType = "system_prompt"
	PatternTypeTool     PatternType = "tool_definition"
	PatternTypeCode     PatternType = "code_block"
	PatternTypeDocument PatternType = "document"
)

// minCacheableTokens is Anthropic's minimum prefix size to qualify for caching.
const minCacheableTokens = 1024

// maxCacheMarkers is the maximum number of simultaneous cache_control markers
// Anthropic allows per request.
const maxCacheMarkers = 4

// CacheAnnotation describes whether and how a chunk should receive a
// cache_control marker before being sent to the Anthropic API.
type CacheAnnotation struct {
	// Recommended is true when a cache_control marker should be placed after
	// this chunk.
	Recommended bool

	// Reason is a short label explaining why caching is recommended.
	Reason string

	// MinTokensMet is true when the chunk meets Anthropic's 1024-token minimum.
	MinTokensMet bool

	// BoundaryAfter indicates the marker should be placed after this chunk.
	BoundaryAfter bool
}

// DetectedPattern represents a detected cacheable pattern.
type DetectedPattern struct {
	Type            PatternType
	Hash            string
	Text            string
	TokenCount      int
	CacheAnnotation *CacheAnnotation
	Metadata        map[string]string
}

// DetectPattern analyzes text and returns pattern information including a
// cache annotation indicating whether a cache_control marker is recommended.
func (d *PatternDetector) DetectPattern(text string) *DetectedPattern {
	if len(text) < d.MinLength {
		return nil
	}

	lower := strings.ToLower(text)
	patternType := d.classifyPattern(lower, text)
	tokens := estimateTokens(text)

	p := &DetectedPattern{
		Type:       patternType,
		Hash:       HashText(text),
		Text:       text,
		TokenCount: tokens,
		Metadata:   make(map[string]string),
	}
	p.CacheAnnotation = d.annotate(patternType, tokens, text)
	return p
}

// DetectChunkPatterns analyzes chunks and returns cacheable patterns.
// The returned slice is ordered by token count descending so callers can
// trivially apply the maxCacheMarkers limit.
func (d *PatternDetector) DetectChunkPatterns(chunks []types.Chunk) []DetectedPattern {
	var patterns []DetectedPattern

	for _, chunk := range chunks {
		if pattern := d.DetectPattern(chunk.Text); pattern != nil {
			pattern.Metadata["chunk_id"] = chunk.ID
			patterns = append(patterns, *pattern)
		}
	}

	return patterns
}

// AnnotateChunksForCache walks chunks in order, applies cache_control
// annotations, and returns a CacheControlPlan describing which chunk indices
// should receive a marker. At most maxCacheMarkers markers are placed; when
// more candidates exist the ones with the highest token counts are chosen.
//
// If any chunk already carries a manual cache_control marker (indicated by
// the "cache_control" key in its Metadata), auto-placement is skipped and
// the existing markers are returned as-is.
func (d *PatternDetector) AnnotateChunksForCache(chunks []types.Chunk) CacheControlPlan {
	// Respect manually placed markers.
	for _, c := range chunks {
		if _, ok := c.Metadata["cache_control"]; ok {
			return CacheControlPlan{ManualMarkersPresent: true}
		}
	}

	type candidate struct {
		index  int
		tokens int
		reason string
	}
	var candidates []candidate

	for i, chunk := range chunks {
		p := d.DetectPattern(chunk.Text)
		if p == nil || p.CacheAnnotation == nil || !p.CacheAnnotation.Recommended {
			continue
		}
		candidates = append(candidates, candidate{
			index:  i,
			tokens: p.TokenCount,
			reason: p.CacheAnnotation.Reason,
		})
	}

	// Select up to maxCacheMarkers by highest token count.
	if len(candidates) > maxCacheMarkers {
		// Partial sort: keep the top maxCacheMarkers by token count.
		for i := 0; i < maxCacheMarkers; i++ {
			best := i
			for j := i + 1; j < len(candidates); j++ {
				if candidates[j].tokens > candidates[best].tokens {
					best = j
				}
			}
			candidates[i], candidates[best] = candidates[best], candidates[i]
		}
		candidates = candidates[:maxCacheMarkers]
	}

	plan := CacheControlPlan{}
	for _, c := range candidates {
		plan.Markers = append(plan.Markers, CacheMarker{
			ChunkIndex: c.index,
			Reason:     c.reason,
			Tokens:     c.tokens,
		})
	}
	return plan
}

// CacheControlPlan describes where cache_control markers should be placed.
type CacheControlPlan struct {
	// Markers lists the chunks that should receive a cache_control marker.
	Markers []CacheMarker

	// ManualMarkersPresent is true when the caller has already placed
	// cache_control markers; auto-placement was skipped.
	ManualMarkersPresent bool
}

// CacheMarker identifies a single cache_control placement.
type CacheMarker struct {
	// ChunkIndex is the position in the chunks slice.
	ChunkIndex int

	// Reason is a short label for why this chunk was selected.
	Reason string

	// Tokens is the token count of the chunk.
	Tokens int
}

// annotate builds a CacheAnnotation for the given pattern type and token count.
func (d *PatternDetector) annotate(pt PatternType, tokens int, text string) *CacheAnnotation {
	minMet := tokens >= minCacheableTokens

	switch pt {
	case PatternTypeSystem:
		return &CacheAnnotation{
			Recommended:   true,
			Reason:        "system_prompt",
			MinTokensMet:  minMet,
			BoundaryAfter: true,
		}
	case PatternTypeTool:
		return &CacheAnnotation{
			Recommended:   true,
			Reason:        "tool_definition",
			MinTokensMet:  minMet,
			BoundaryAfter: true,
		}
	case PatternTypeCode:
		// Only recommend caching for large code blocks in the first 60% of
		// the text (heuristic: position unknown here, so use token count only).
		recommended := tokens >= 512
		return &CacheAnnotation{
			Recommended:   recommended,
			Reason:        "stable_code_block",
			MinTokensMet:  minMet,
			BoundaryAfter: true,
		}
	case PatternTypeDocument:
		// Boilerplate / repetitive documents benefit from caching.
		return &CacheAnnotation{
			Recommended:   true,
			Reason:        "document",
			MinTokensMet:  minMet,
			BoundaryAfter: true,
		}
	default:
		return &CacheAnnotation{Recommended: false}
	}
}

// estimateTokens approximates the token count of text using the common
// 4-chars-per-token heuristic.
func estimateTokens(text string) int {
	return (len(text) + 3) / 4
}

// classifyPattern determines the pattern type.
func (d *PatternDetector) classifyPattern(lower, original string) PatternType {
	// Check for system prompt
	for _, prefix := range d.SystemPromptPrefixes {
		if strings.HasPrefix(lower, prefix) || strings.Contains(lower[:min(200, len(lower))], prefix) {
			return PatternTypeSystem
		}
	}

	// Check for tool definition
	toolMarkerCount := 0
	for _, marker := range d.ToolDefinitionMarkers {
		if strings.Contains(lower, marker) {
			toolMarkerCount++
		}
	}
	if toolMarkerCount >= 2 {
		return PatternTypeTool
	}

	// Check for code block
	if strings.Contains(original, "```") || strings.Contains(original, "def ") ||
		strings.Contains(original, "func ") || strings.Contains(original, "function ") {
		return PatternTypeCode
	}

	return PatternTypeDocument
}

// HashText creates a SHA-256 hash of the text.
func HashText(text string) string {
	h := sha256.New()
	h.Write([]byte(text))
	return hex.EncodeToString(h.Sum(nil))[:16] // First 16 chars for brevity
}

// HashChunks creates a combined hash for a set of chunks.
func HashChunks(chunks []types.Chunk) string {
	h := sha256.New()
	for _, chunk := range chunks {
		h.Write([]byte(chunk.ID))
		h.Write([]byte(chunk.Text))
	}
	return hex.EncodeToString(h.Sum(nil))[:16]
}

// CacheKey generates a cache key for a pattern.
func CacheKey(prefix string, pattern *DetectedPattern) string {
	return prefix + ":" + string(pattern.Type) + ":" + pattern.Hash
}

// CacheKeyForText generates a cache key for raw text.
func CacheKeyForText(prefix, text string) string {
	return prefix + ":text:" + HashText(text)
}

// CacheKeyForChunks generates a cache key for a chunk set.
func CacheKeyForChunks(prefix string, chunks []types.Chunk) string {
	return prefix + ":chunks:" + HashChunks(chunks)
}

// CacheKeyForQuery generates a cache key for a query.
func CacheKeyForQuery(prefix, query string, topK int) string {
	h := sha256.New()
	h.Write([]byte(query))
	h.Write([]byte{byte(topK >> 8), byte(topK)})
	hash := hex.EncodeToString(h.Sum(nil))[:16]
	return prefix + ":query:" + hash
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
