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
	PatternTypeUnknown    PatternType = "unknown"
	PatternTypeSystem     PatternType = "system_prompt"
	PatternTypeTool       PatternType = "tool_definition"
	PatternTypeCode       PatternType = "code_block"
	PatternTypeDocument   PatternType = "document"
)

// DetectedPattern represents a detected cacheable pattern.
type DetectedPattern struct {
	Type     PatternType
	Hash     string
	Text     string
	Metadata map[string]string
}

// DetectPattern analyzes text and returns pattern information.
func (d *PatternDetector) DetectPattern(text string) *DetectedPattern {
	if len(text) < d.MinLength {
		return nil
	}

	lower := strings.ToLower(text)
	patternType := d.classifyPattern(lower, text)

	return &DetectedPattern{
		Type:     patternType,
		Hash:     HashText(text),
		Text:     text,
		Metadata: make(map[string]string),
	}
}

// DetectChunkPatterns analyzes chunks and returns cacheable patterns.
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
