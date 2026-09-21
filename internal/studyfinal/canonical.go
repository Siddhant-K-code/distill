package studyfinal

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"reflect"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"golang.org/x/text/unicode/norm"
)

func canonicalJSON(v any) ([]byte, error) {
	if err := validateJSONValue(reflect.ValueOf(v), map[visit]bool{}); err != nil {
		return nil, err
	}
	raw, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	value, err := decodeNormalizedJSON(raw)
	if err != nil {
		return nil, err
	}
	normalized, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(normalized)
}

// Digest is retained only for compatibility with callers that do not define a
// persistent identity. Persistent study identities must use DigestDomain.
func Digest(v any) (string, error) {
	return DigestDomain("compatibility", v)
}

func DigestDomain(domain string, v any) (string, error) {
	if domain == "" {
		return "", fmt.Errorf("digest domain is required")
	}
	for _, r := range domain {
		valid := r >= 'a' && r <= 'z' || r >= '0' && r <= '9' || r == '-'
		if r > 0x7f || !valid {
			return "", fmt.Errorf("invalid digest domain %q", domain)
		}
	}
	b, err := canonicalJSON(v)
	if err != nil {
		return "", err
	}
	preimage := append([]byte("context-build-artifact/"+domain+"/v1\n"), b...)
	return digestBytes(preimage), nil
}

func digestBytes(b []byte) string {
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}

func strictDecode(r io.Reader, out any) error {
	raw, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if !utf8.Valid(raw) {
		return fmt.Errorf("JSON is not valid UTF-8")
	}
	if _, err := decodeNormalizedJSON(raw); err != nil {
		return err
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.DisallowUnknownFields()
	if err := d.Decode(out); err != nil {
		return err
	}
	if err := requireJSONEOF(d); err != nil {
		return err
	}
	return nil
}

func decodeNormalizedJSON(raw []byte) (any, error) {
	if !utf8.Valid(raw) {
		return nil, fmt.Errorf("JSON is not valid UTF-8")
	}
	d := json.NewDecoder(bytes.NewReader(raw))
	d.UseNumber()
	value, err := decodeJSONValue(d, 0)
	if err != nil {
		return nil, err
	}
	if err := requireJSONEOF(d); err != nil {
		return nil, err
	}
	return value, nil
}

func requireJSONEOF(d *json.Decoder) error {
	var extra any
	if err := d.Decode(&extra); err != io.EOF {
		if err == nil {
			return fmt.Errorf("trailing JSON value")
		}
		return fmt.Errorf("trailing JSON data: %w", err)
	}
	return nil
}

func decodeJSONValue(d *json.Decoder, depth int) (any, error) {
	if depth > 64 {
		return nil, fmt.Errorf("JSON nesting exceeds 64 levels")
	}
	token, err := d.Token()
	if err != nil {
		return nil, err
	}
	switch token := token.(type) {
	case json.Delim:
		switch token {
		case '{':
			out := map[string]any{}
			original := map[string]bool{}
			for d.More() {
				keyToken, err := d.Token()
				if err != nil {
					return nil, err
				}
				key, ok := keyToken.(string)
				if !ok {
					return nil, fmt.Errorf("JSON object key is not a string")
				}
				if original[key] {
					return nil, fmt.Errorf("duplicate JSON key %q", key)
				}
				original[key] = true
				normalizedKey := norm.NFC.String(key)
				if _, exists := out[normalizedKey]; exists {
					return nil, fmt.Errorf("JSON key normalization collision at %q", normalizedKey)
				}
				value, err := decodeJSONValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				out[normalizedKey] = value
			}
			if end, err := d.Token(); err != nil || end != json.Delim('}') {
				return nil, fmt.Errorf("invalid JSON object")
			}
			return out, nil
		case '[':
			var out []any
			for d.More() {
				value, err := decodeJSONValue(d, depth+1)
				if err != nil {
					return nil, err
				}
				out = append(out, value)
			}
			if end, err := d.Token(); err != nil || end != json.Delim(']') {
				return nil, fmt.Errorf("invalid JSON array")
			}
			return out, nil
		default:
			return nil, fmt.Errorf("unexpected JSON delimiter")
		}
	case string:
		return norm.NFC.String(token), nil
	case json.Number:
		number, err := token.Float64()
		if err != nil || math.IsNaN(number) || math.IsInf(number, 0) {
			return nil, fmt.Errorf("non-finite or invalid JSON number %q", token)
		}
		return token, nil
	case bool, nil:
		return token, nil
	default:
		return nil, fmt.Errorf("unsupported JSON token")
	}
}

type visit struct {
	typ reflect.Type
	ptr uintptr
}

func validateJSONValue(v reflect.Value, seen map[visit]bool) error {
	if !v.IsValid() {
		return nil
	}
	if v.Kind() == reflect.Interface {
		if v.IsNil() {
			return nil
		}
		return validateJSONValue(v.Elem(), seen)
	}
	if v.Kind() == reflect.Pointer {
		if v.IsNil() {
			return nil
		}
		key := visit{v.Type(), v.Pointer()}
		if seen[key] {
			return fmt.Errorf("cyclic JSON value")
		}
		seen[key] = true
		defer delete(seen, key)
		return validateJSONValue(v.Elem(), seen)
	}
	switch v.Kind() {
	case reflect.String:
		if !utf8.ValidString(v.String()) {
			return fmt.Errorf("JSON string is not valid UTF-8")
		}
	case reflect.Float32, reflect.Float64:
		f := v.Float()
		if math.IsNaN(f) || math.IsInf(f, 0) {
			return fmt.Errorf("JSON number must be finite")
		}
	case reflect.Map:
		if v.IsNil() {
			return nil
		}
		key := visit{v.Type(), v.Pointer()}
		if seen[key] {
			return fmt.Errorf("cyclic JSON value")
		}
		seen[key] = true
		defer delete(seen, key)
		keys := map[string]bool{}
		iter := v.MapRange()
		for iter.Next() {
			key := iter.Key()
			if key.Kind() == reflect.String {
				if !utf8.ValidString(key.String()) {
					return fmt.Errorf("JSON key is not valid UTF-8")
				}
				normalized := norm.NFC.String(key.String())
				if keys[normalized] {
					return fmt.Errorf("JSON key normalization collision at %q", normalized)
				}
				keys[normalized] = true
			}
			if err := validateJSONValue(iter.Value(), seen); err != nil {
				return err
			}
		}
	case reflect.Slice:
		if v.IsNil() {
			return nil
		}
		if v.Type().Elem().Kind() == reflect.Uint8 {
			return nil
		}
		key := visit{v.Type(), v.Pointer()}
		if seen[key] {
			return fmt.Errorf("cyclic JSON value")
		}
		seen[key] = true
		defer delete(seen, key)
		fallthrough
	case reflect.Array:
		for i := 0; i < v.Len(); i++ {
			if err := validateJSONValue(v.Index(i), seen); err != nil {
				return err
			}
		}
	case reflect.Struct:
		for i := 0; i < v.NumField(); i++ {
			if v.Type().Field(i).PkgPath == "" {
				if err := validateJSONValue(v.Field(i), seen); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
