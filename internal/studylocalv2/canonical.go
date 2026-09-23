package studylocalv2

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math"
	"math/big"
	"path"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/gowebpki/jcs"
	"golang.org/x/text/unicode/norm"
)

const (
	maxArtifactBytes = 64 << 20
	maxJSONLRecords  = 1000
)

const (
	FramingValid             = "valid"
	FramingMissingLF         = "missing_terminal_lf"
	FramingMultipleLF        = "multiple_terminal_lf"
	FramingCarriageReturn    = "carriage_return"
	FramingLeadingWhitespace = "leading_whitespace"
	FramingTrailingBytes     = "trailing_bytes"
	FramingDuplicateKey      = "duplicate_key"
	FramingNonFinite         = "nonfinite_number"
	FramingNumberRange       = "number_out_of_range"
	FramingNonObject         = "non_object"
	FramingInvalidJSON       = "invalid_json"
	FramingNonCanonical      = "non_canonical"
)

type framingError struct {
	status string
	err    error
}

func (err *framingError) Error() string {
	if err.err == nil {
		return err.status
	}
	return err.status + ": " + err.err.Error()
}

func (err *framingError) Unwrap() error {
	return err.err
}

func framingStatus(err error) string {
	if err == nil {
		return FramingValid
	}
	var target *framingError
	if errors.As(err, &target) {
		return target.status
	}
	return FramingInvalidJSON
}

func DigestBytes(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func DigestDomain(domain string, value any) (string, error) {
	canonical, err := canonicalJSON(value)
	if err != nil {
		return "", err
	}
	preimage := append([]byte("context-build-artifact/local-context-control/"+domain+"/v2\n"), canonical...)
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

func strictJSONFileDecode(data []byte, value any) error {
	payload, err := strictEnvelopePayload(data)
	if err != nil {
		return err
	}
	return strictJSONPayloadDecode(payload, value, false)
}

func strictJSONObjectFileDecode(data []byte, value any) error {
	payload, err := strictEnvelopePayload(data)
	if err != nil {
		return err
	}
	return strictJSONPayloadDecode(payload, value, true)
}

func strictJSONFrameDecode(frame []byte, value any) error {
	payload, err := strictEnvelopePayload(frame)
	if err != nil {
		return err
	}
	return strictJSONPayloadDecode(payload, value, true)
}

func strictEnvelopePayload(data []byte) ([]byte, error) {
	if len(data) == 0 || data[len(data)-1] != '\n' {
		return nil, &framingError{status: FramingMissingLF}
	}
	if len(data) >= 2 && data[len(data)-2] == '\n' {
		return nil, &framingError{status: FramingMultipleLF}
	}
	if bytes.ContainsRune(data, '\r') {
		return nil, &framingError{status: FramingCarriageReturn}
	}
	return data[:len(data)-1], nil
}

func strictJSONPayloadDecode(data []byte, value any, requireObject bool) error {
	if len(data) == 0 || len(data) > maxArtifactBytes {
		return &framingError{status: FramingInvalidJSON, err: fmt.Errorf("payload size is invalid")}
	}
	if data[0] == ' ' || data[0] == '\t' || data[0] == '\n' || data[0] == '\r' {
		return &framingError{status: FramingLeadingWhitespace}
	}
	if bytes.ContainsRune(data, '\r') {
		return &framingError{status: FramingCarriageReturn}
	}
	if !utf8.Valid(data) {
		return &framingError{status: FramingInvalidJSON, err: fmt.Errorf("payload is not UTF-8")}
	}
	if containsNonFiniteToken(data) {
		return &framingError{status: FramingNonFinite}
	}
	if requireObject && data[0] != '{' {
		return &framingError{status: FramingNonObject}
	}
	if err := validateJSONTokens(data); err != nil {
		return err
	}
	canonical, err := jcs.Transform(data)
	if err != nil {
		return classifyJCSFailure(err)
	}
	if !bytes.Equal(canonical, data) {
		return &framingError{status: FramingNonCanonical}
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	decoder.UseNumber()
	if err := decoder.Decode(value); err != nil {
		return &framingError{status: FramingInvalidJSON, err: err}
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		if err == nil {
			return &framingError{status: FramingTrailingBytes}
		}
		return &framingError{status: FramingTrailingBytes, err: err}
	}
	return nil
}

func containsNonFiniteToken(data []byte) bool {
	inString := false
	escaped := false
	for index := 0; index < len(data); index++ {
		switch {
		case inString && escaped:
			escaped = false
		case inString && data[index] == '\\':
			escaped = true
		case data[index] == '"':
			inString = !inString
		case !inString:
			for _, token := range [][]byte{[]byte("NaN"), []byte("Infinity"), []byte("-Infinity")} {
				if bytes.HasPrefix(data[index:], token) {
					return true
				}
			}
		}
	}
	return false
}

func validateJSONTokens(data []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.UseNumber()
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	if _, err := decoder.Token(); err != io.EOF {
		if err == nil {
			return &framingError{status: FramingTrailingBytes}
		}
		return &framingError{status: FramingTrailingBytes, err: err}
	}
	return nil
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return &framingError{status: FramingInvalidJSON, err: err}
	}
	switch item := token.(type) {
	case json.Delim:
		switch item {
		case '{':
			keys := map[string]struct{}{}
			for decoder.More() {
				keyToken, err := decoder.Token()
				if err != nil {
					return &framingError{status: FramingInvalidJSON, err: err}
				}
				key, ok := keyToken.(string)
				if !ok {
					return &framingError{status: FramingInvalidJSON, err: fmt.Errorf("object key is not a string")}
				}
				if _, duplicate := keys[key]; duplicate {
					return &framingError{status: FramingDuplicateKey, err: fmt.Errorf("%q", key)}
				}
				keys[key] = struct{}{}
				if err := consumeJSONValue(decoder); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim('}') {
				return &framingError{status: FramingInvalidJSON, err: err}
			}
		case '[':
			for decoder.More() {
				if err := consumeJSONValue(decoder); err != nil {
					return err
				}
			}
			end, err := decoder.Token()
			if err != nil || end != json.Delim(']') {
				return &framingError{status: FramingInvalidJSON, err: err}
			}
		default:
			return &framingError{status: FramingInvalidJSON}
		}
	case json.Number:
		if err := validateJSONNumber(item.String()); err != nil {
			return err
		}
	case float64:
		if math.IsInf(item, 0) || math.IsNaN(item) {
			return &framingError{status: FramingNonFinite}
		}
	}
	return nil
}

func validateJSONNumber(raw string) error {
	if !strings.ContainsAny(raw, ".eE") {
		integer, ok := new(big.Int).SetString(raw, 10)
		if !ok {
			return &framingError{status: FramingInvalidJSON}
		}
		limit := big.NewInt(9_007_199_254_740_991)
		if new(big.Int).Abs(integer).Cmp(limit) > 0 {
			return &framingError{status: FramingNumberRange}
		}
	}
	if strings.ContainsAny(raw, "eE") {
		parts := strings.FieldsFunc(raw, func(r rune) bool { return r == 'e' || r == 'E' })
		if len(parts) != 2 {
			return &framingError{status: FramingInvalidJSON}
		}
		exponent, err := strconv.Atoi(parts[1])
		if err != nil || exponent > 308 || exponent < -308 {
			return &framingError{status: FramingNumberRange}
		}
	}
	value, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		if errors.Is(err, strconv.ErrRange) {
			return &framingError{status: FramingNumberRange}
		}
		return &framingError{status: FramingInvalidJSON, err: err}
	}
	if math.IsInf(value, 0) || math.IsNaN(value) {
		return &framingError{status: FramingNonFinite}
	}
	return nil
}

func classifyJCSFailure(err error) error {
	message := err.Error()
	switch {
	case strings.Contains(message, "invalid character") && (strings.Contains(message, "N") || strings.Contains(message, "I")):
		return &framingError{status: FramingNonFinite, err: err}
	default:
		return &framingError{status: FramingInvalidJSON, err: err}
	}
}

func parseJSONL[T any](data []byte) ([]T, error) {
	if len(data) == 0 || len(data) > maxArtifactBytes || data[len(data)-1] != '\n' ||
		bytes.ContainsRune(data, '\r') || (len(data) >= 2 && data[len(data)-2] == '\n') {
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
		if err := strictJSONPayloadDecode(line, &record, true); err != nil {
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
