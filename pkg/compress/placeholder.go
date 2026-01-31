package compress

import (
	"context"
	"encoding/json"
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/Siddhant-K-code/distill/pkg/types"
)

// PlaceholderCompressor replaces verbose tool outputs with compact summaries.
// It detects JSON, XML, tables, and other structured content and replaces them
// with descriptive placeholders.
type PlaceholderCompressor struct {
	// PreserveKeys keeps these JSON keys in the output.
	PreserveKeys []string

	// MaxArrayItems limits array elements shown before truncation.
	MaxArrayItems int

	// MaxObjectDepth limits nested object depth.
	MaxObjectDepth int
}

// NewPlaceholderCompressor creates a placeholder compressor with defaults.
func NewPlaceholderCompressor() *PlaceholderCompressor {
	return &PlaceholderCompressor{
		PreserveKeys:   []string{"id", "name", "title", "error", "message", "status"},
		MaxArrayItems:  3,
		MaxObjectDepth: 2,
	}
}

// Compress replaces verbose structured content with placeholders.
func (p *PlaceholderCompressor) Compress(ctx context.Context, chunks []types.Chunk, opts Options) ([]types.Chunk, Stats, error) {
	start := time.Now()
	stats := Stats{}

	result := make([]types.Chunk, 0, len(chunks))

	for _, chunk := range chunks {
		inputTokens := estimateTokens(chunk.Text)
		stats.InputTokens += inputTokens

		if len(chunk.Text) < opts.MinChunkLength {
			stats.ChunksSkipped++
			stats.OutputTokens += inputTokens
			result = append(result, chunk)
			continue
		}

		compressed := p.compressStructured(chunk.Text, opts.PreserveStructure)
		stats.ChunksProcessed++
		stats.OutputTokens += estimateTokens(compressed)

		newChunk := chunk.Clone()
		newChunk.Text = compressed
		result = append(result, *newChunk)
	}

	stats.Latency = time.Since(start)
	if stats.InputTokens > 0 {
		stats.ReductionPercent = float64(stats.InputTokens-stats.OutputTokens) / float64(stats.InputTokens) * 100
	}

	return result, stats, nil
}

// compressStructured detects and compresses structured content.
func (p *PlaceholderCompressor) compressStructured(text string, preserveStructure bool) string {
	// Try JSON compression
	if compressed, ok := p.tryCompressJSON(text, preserveStructure); ok {
		return compressed
	}

	// Try XML compression
	if compressed, ok := p.tryCompressXML(text); ok {
		return compressed
	}

	// Try table compression
	if compressed, ok := p.tryCompressTable(text); ok {
		return compressed
	}

	return text
}

// tryCompressJSON attempts to parse and compress JSON content.
func (p *PlaceholderCompressor) tryCompressJSON(text string, preserveStructure bool) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "{") && !strings.HasPrefix(trimmed, "[") {
		return "", false
	}

	var data interface{}
	if err := json.Unmarshal([]byte(trimmed), &data); err != nil {
		return "", false
	}

	if preserveStructure {
		compressed := p.compressJSONValue(data, 0)
		result, err := json.Marshal(compressed)
		if err != nil {
			return "", false
		}
		return string(result), true
	}

	return p.summarizeJSON(data), true
}

// compressJSONValue recursively compresses JSON while preserving structure.
func (p *PlaceholderCompressor) compressJSONValue(v interface{}, depth int) interface{} {
	if depth >= p.MaxObjectDepth {
		return "[...]"
	}

	switch val := v.(type) {
	case map[string]interface{}:
		result := make(map[string]interface{})
		for k, v := range val {
			if p.shouldPreserveKey(k) {
				result[k] = p.compressJSONValue(v, depth+1)
			}
		}
		if len(result) == 0 && len(val) > 0 {
			return fmt.Sprintf("{...%d keys}", len(val))
		}
		return result

	case []interface{}:
		if len(val) <= p.MaxArrayItems {
			result := make([]interface{}, len(val))
			for i, item := range val {
				result[i] = p.compressJSONValue(item, depth+1)
			}
			return result
		}
		result := make([]interface{}, p.MaxArrayItems+1)
		for i := 0; i < p.MaxArrayItems; i++ {
			result[i] = p.compressJSONValue(val[i], depth+1)
		}
		result[p.MaxArrayItems] = fmt.Sprintf("...+%d more", len(val)-p.MaxArrayItems)
		return result

	default:
		return val
	}
}

// shouldPreserveKey checks if a key should be kept in compressed output.
func (p *PlaceholderCompressor) shouldPreserveKey(key string) bool {
	lower := strings.ToLower(key)
	for _, k := range p.PreserveKeys {
		if strings.ToLower(k) == lower {
			return true
		}
	}
	return false
}

// summarizeJSON creates a text summary of JSON structure.
func (p *PlaceholderCompressor) summarizeJSON(v interface{}) string {
	switch val := v.(type) {
	case map[string]interface{}:
		keys := make([]string, 0, len(val))
		for k := range val {
			keys = append(keys, k)
		}
		if len(keys) > 5 {
			return fmt.Sprintf("[JSON object with %d keys: %s, ...]", len(keys), strings.Join(keys[:5], ", "))
		}
		return fmt.Sprintf("[JSON object with keys: %s]", strings.Join(keys, ", "))

	case []interface{}:
		if len(val) == 0 {
			return "[empty JSON array]"
		}
		return fmt.Sprintf("[JSON array with %d items]", len(val))

	default:
		return fmt.Sprintf("[JSON value: %v]", val)
	}
}

// tryCompressXML attempts to detect and compress XML content.
func (p *PlaceholderCompressor) tryCompressXML(text string) (string, bool) {
	trimmed := strings.TrimSpace(text)
	if !strings.HasPrefix(trimmed, "<") {
		return "", false
	}

	// Simple XML detection
	xmlPattern := regexp.MustCompile(`<(\w+)[^>]*>.*</\1>`)
	if !xmlPattern.MatchString(trimmed) {
		return "", false
	}

	// Count elements
	elementPattern := regexp.MustCompile(`<(\w+)[^/>]*>`)
	matches := elementPattern.FindAllStringSubmatch(trimmed, -1)

	elements := make(map[string]int)
	for _, m := range matches {
		elements[m[1]]++
	}

	var summary strings.Builder
	summary.WriteString("[XML with elements: ")
	i := 0
	for elem, count := range elements {
		if i > 0 {
			summary.WriteString(", ")
		}
		if i >= 5 {
			summary.WriteString("...")
			break
		}
		if count > 1 {
			summary.WriteString(fmt.Sprintf("%s(×%d)", elem, count))
		} else {
			summary.WriteString(elem)
		}
		i++
	}
	summary.WriteString("]")

	return summary.String(), true
}

// tryCompressTable attempts to detect and compress tabular data.
func (p *PlaceholderCompressor) tryCompressTable(text string) (string, bool) {
	lines := strings.Split(text, "\n")
	if len(lines) < 3 {
		return "", false
	}

	// Detect delimiter-separated values
	delimiters := []string{"\t", "|", ","}
	for _, delim := range delimiters {
		if cols := strings.Count(lines[0], delim); cols >= 2 {
			// Likely a table
			consistent := true
			for _, line := range lines[1:] {
				if strings.TrimSpace(line) == "" {
					continue
				}
				if strings.Count(line, delim) != cols {
					consistent = false
					break
				}
			}
			if consistent {
				headers := strings.Split(lines[0], delim)
				for i := range headers {
					headers[i] = strings.TrimSpace(headers[i])
				}
				return fmt.Sprintf("[Table with %d rows, columns: %s]", len(lines)-1, strings.Join(headers, ", ")), true
			}
		}
	}

	return "", false
}
