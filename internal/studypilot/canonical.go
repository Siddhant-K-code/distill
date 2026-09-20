package studypilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"strings"

	"github.com/gowebpki/jcs"
	"golang.org/x/text/unicode/norm"
)

const (
	maxArtifactBytes = 16 << 20
	maxJSONLRecords  = 1000
	maxJSONDepth     = 64
	maxJSONNodes     = 100000
)

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func digestText(value string) string {
	return digestBytes([]byte(value))
}

func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON: %w", err)
	}
	canonical, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("canonicalize JSON: %w", err)
	}
	return canonical, nil
}

func canonicalJSONFile(value any) ([]byte, error) {
	data, err := canonicalJSON(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func canonicalJSONL(values []any) ([]byte, error) {
	var out bytes.Buffer
	for index, value := range values {
		line, err := canonicalJSON(value)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", index, err)
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func structuredDigest(domain string, value any) (string, error) {
	canonical, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	preimage := append([]byte("context-build-artifact/"+domain+"/v1\n"), canonical...)
	return digestBytes(preimage), nil
}

func decodeStrict(data []byte, target any) error {
	if len(data) > maxArtifactBytes {
		return fmt.Errorf("artifact exceeds %d-byte limit", maxArtifactBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("unexpected trailing JSON value")
		}
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return nil
}

func checkJSONValue(value any, depth int) error {
	if depth > maxJSONDepth {
		return fmt.Errorf("JSON nesting exceeds %d", maxJSONDepth)
	}
	switch typed := value.(type) {
	case map[string]any:
		for key, child := range typed {
			if !norm.NFC.IsNormalString(key) {
				return fmt.Errorf("object key is not UTF-8 NFC")
			}
			if err := checkJSONValue(child, depth+1); err != nil {
				return err
			}
		}
	case []any:
		for _, child := range typed {
			if err := checkJSONValue(child, depth+1); err != nil {
				return err
			}
		}
	case string:
		if !norm.NFC.IsNormalString(typed) {
			return fmt.Errorf("JSON string is not UTF-8 NFC")
		}
	case float64:
		if math.IsNaN(typed) || math.IsInf(typed, 0) {
			return fmt.Errorf("JSON contains a non-finite number")
		}
	}
	return nil
}

func validateCanonicalLine(line []byte) error {
	if len(line) == 0 {
		return fmt.Errorf("empty JSONL record")
	}
	if err := validateJSONStructuralLimits(line); err != nil {
		return err
	}
	var value any
	decoder := json.NewDecoder(bytes.NewReader(line))
	decoder.UseNumber()
	if err := decoder.Decode(&value); err != nil {
		return err
	}
	if decoder.More() {
		return fmt.Errorf("trailing JSON data")
	}
	if err := checkJSONValue(value, 0); err != nil {
		return err
	}
	canonical, err := jcs.Transform(line)
	if err != nil {
		return err
	}
	if !bytes.Equal(line, canonical) {
		return fmt.Errorf("record is not RFC 8785 canonical JSON")
	}
	return nil
}

func validateJSONStructuralLimits(data []byte) error {
	depth := 0
	nodes := 1
	inString := false
	escaped := false
	for _, value := range data {
		if inString {
			if escaped {
				escaped = false
				continue
			}
			if value == '\\' {
				escaped = true
			} else if value == '"' {
				inString = false
			}
			continue
		}
		switch value {
		case '"':
			inString = true
		case '{', '[':
			depth++
			nodes++
			if depth > maxJSONDepth {
				return fmt.Errorf("JSON nesting exceeds %d", maxJSONDepth)
			}
		case '}', ']':
			depth--
			if depth < 0 {
				return fmt.Errorf("JSON structure is unbalanced")
			}
		case ',':
			nodes++
			if nodes > maxJSONNodes {
				return fmt.Errorf("JSON node count exceeds %d", maxJSONNodes)
			}
		}
	}
	if inString || depth != 0 {
		return fmt.Errorf("JSON structure is incomplete")
	}
	return nil
}

func parseJSONL[T any](data []byte) ([]T, error) {
	if len(data) > maxArtifactBytes {
		return nil, fmt.Errorf("JSONL exceeds %d-byte limit", maxArtifactBytes)
	}
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, fmt.Errorf("JSONL must end in LF")
	}
	if bytes.Contains(data, []byte{'\r'}) {
		return nil, fmt.Errorf("JSONL contains CR")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) > maxJSONLRecords {
		return nil, fmt.Errorf("JSONL exceeds %d-record limit", maxJSONLRecords)
	}
	result := make([]T, 0, len(lines))
	for index, line := range lines {
		if err := validateCanonicalLine(line); err != nil {
			return nil, fmt.Errorf("record %d: %w", index, err)
		}
		var record T
		if err := decodeStrict(line, &record); err != nil {
			return nil, fmt.Errorf("record %d: %w", index, err)
		}
		result = append(result, record)
	}
	return result, nil
}

func validDigest(value string) bool {
	if len(value) != 64 || strings.ToLower(value) != value {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size
}
