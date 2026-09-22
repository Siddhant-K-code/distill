package studylocal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"path"
	"strings"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"golang.org/x/text/unicode/norm"
)

const (
	maxArtifactBytes = 64 << 20
	maxJSONLRecords  = 1000
)

func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func DigestDomain(domain string, value any) (string, error) {
	canonical, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	preimage := append([]byte("context-build-artifact/local-context-control/"+domain+"/v1\n"), canonical...)
	return DigestBytes(preimage), nil
}

func canonicalJSON(value any) ([]byte, error) {
	raw, err := json.Marshal(value)
	if err != nil {
		return nil, fmt.Errorf("marshal JSON: %w", err)
	}
	data, err := jcs.Transform(raw)
	if err != nil {
		return nil, fmt.Errorf("canonicalize JSON: %w", err)
	}
	return data, nil
}

func canonicalJSONFile(value any) ([]byte, error) {
	data, err := canonicalJSON(value)
	if err != nil {
		return nil, err
	}
	return append(data, '\n'), nil
}

func canonicalJSONL[T any](records []T) ([]byte, error) {
	var out bytes.Buffer
	for i, record := range records {
		line, err := canonicalJSON(record)
		if err != nil {
			return nil, fmt.Errorf("record %d: %w", i, err)
		}
		out.Write(line)
		out.WriteByte('\n')
	}
	return out.Bytes(), nil
}

func strictDecode(data []byte, value any) error {
	if len(data) > maxArtifactBytes {
		return fmt.Errorf("artifact exceeds %d bytes", maxArtifactBytes)
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
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

func parseJSONL[T any](data []byte) ([]T, error) {
	if len(data) == 0 || len(data) > maxArtifactBytes || data[len(data)-1] != '\n' || bytes.ContainsRune(data, '\r') {
		return nil, fmt.Errorf("invalid JSONL envelope")
	}
	lines := bytes.Split(data[:len(data)-1], []byte{'\n'})
	if len(lines) > maxJSONLRecords {
		return nil, fmt.Errorf("JSONL exceeds %d records", maxJSONLRecords)
	}
	out := make([]T, 0, len(lines))
	for i, line := range lines {
		canonical, err := jcs.Transform(line)
		if err != nil || !bytes.Equal(canonical, line) {
			return nil, fmt.Errorf("record %d is not RFC 8785 canonical JSON", i)
		}
		var record T
		if err := strictDecode(line, &record); err != nil {
			return nil, fmt.Errorf("record %d: %w", i, err)
		}
		out = append(out, record)
	}
	return out, nil
}

func safeRelative(value string) bool {
	if value == "" || !utf8.ValidString(value) || strings.ContainsRune(value, 0) ||
		strings.Contains(value, "\\") || strings.HasPrefix(value, "/") ||
		path.Clean(value) != value || !norm.NFC.IsNormalString(value) {
		return false
	}
	for _, part := range strings.Split(value, "/") {
		if part == "" || part == "." || part == ".." {
			return false
		}
	}
	return true
}
