package lock

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
)

func canonicalJSON(value any) ([]byte, error) {
	var buffer bytes.Buffer
	encoder := json.NewEncoder(&buffer)
	encoder.SetEscapeHTML(false)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return nil, fmt.Errorf("encode canonical JSON: %w", err)
	}
	return buffer.Bytes(), nil
}

func decodeCanonicalJSON(data []byte, value any) error {
	if !json.Valid(data) {
		return fmt.Errorf("invalid JSON")
	}

	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(value); err != nil {
		return fmt.Errorf("decode JSON: %w", err)
	}
	if err := requireEOF(decoder); err != nil {
		return err
	}

	canonical, err := canonicalJSON(value)
	if err != nil {
		return err
	}
	if !bytes.Equal(data, canonical) {
		return fmt.Errorf("JSON is not canonical")
	}
	return nil
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	if err := decoder.Decode(&extra); err == io.EOF {
		return nil
	} else if err != nil {
		return fmt.Errorf("decode trailing JSON: %w", err)
	}
	return fmt.Errorf("unexpected trailing JSON value")
}

func digestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func validDigest(value string) bool {
	if len(value) != sha256.Size*2 {
		return false
	}
	decoded, err := hex.DecodeString(value)
	return err == nil && len(decoded) == sha256.Size && value == hex.EncodeToString(decoded)
}
