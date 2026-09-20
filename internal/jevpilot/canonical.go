package jevpilot

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"math"

	"github.com/gowebpki/jcs"
)

func digest(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func canonicalJSON(value any) ([]byte, error) {
	data, err := json.Marshal(value)
	if err != nil {
		return nil, err
	}
	return jcs.Transform(data)
}

func canonicalJSONL(values any) ([]byte, error) {
	data, err := json.Marshal(values)
	if err != nil {
		return nil, err
	}
	var records []json.RawMessage
	if err := json.Unmarshal(data, &records); err != nil {
		return nil, err
	}
	var output bytes.Buffer
	for _, record := range records {
		line, err := jcs.Transform(record)
		if err != nil {
			return nil, err
		}
		output.Write(line)
		output.WriteByte('\n')
	}
	return output.Bytes(), nil
}

func parseJSONL[T any](data []byte) ([]T, error) {
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	var records []T
	for {
		var record T
		if err := decoder.Decode(&record); err != nil {
			if err == io.EOF {
				break
			}
			return nil, err
		}
		records = append(records, record)
	}
	return records, nil
}

func finiteProbability(value float64) bool {
	return !math.IsNaN(value) && !math.IsInf(value, 0) && value >= 0 && value <= 1
}

func formatNanoUSD(value int64) string {
	whole := value / 1_000_000_000
	fraction := value % 1_000_000_000
	return fmt.Sprintf("%d.%09d", whole, fraction)
}
