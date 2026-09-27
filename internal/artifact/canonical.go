// Package artifact provides byte-stable, offline artifact primitives shared by
// Distill's deterministic workflows.
package artifact

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

func CanonicalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode canonical JSON: %w", err)
	}
	return buffer.Bytes(), nil
}

func DecodeCanonicalJSON(data []byte, value any) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		// Expected.
	} else if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	} else {
		return fmt.Errorf("unexpected trailing JSON value")
	}

	canonical, err := CanonicalJSON(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("JSON is not canonical")
	}
	return nil
}

func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func ValidDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == hex.EncodeToString(decoded)
}
