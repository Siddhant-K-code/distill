package studylocalv2

import (
	"encoding/base64"
	"fmt"
)

const (
	FramingConformanceSchema            = SchemaVersion + "/framing-conformance"
	FramingConformanceKnownAnswerSHA256 = "d864b23a23ff10402bc44a9261b31903077f6f7915a2374db7c4e122caaedae3"
)

type FramingVector struct {
	Name           string `json:"name"`
	ArtifactClass  string `json:"artifact_class"`
	DataBase64     string `json:"data_base64"`
	RequireObject  bool   `json:"require_object"`
	ExpectedStatus string `json:"expected_status"`
}

type FramingConformanceRequest struct {
	SchemaVersion string          `json:"schema_version"`
	Vectors       []FramingVector `json:"vectors"`
}

type FramingConformanceResult struct {
	Name   string `json:"name"`
	Status string `json:"status"`
}

type FramingConformanceResponse struct {
	SchemaVersion        string                     `json:"schema_version"`
	RequestSHA256        string                     `json:"request_sha256"`
	Results              []FramingConformanceResult `json:"results"`
	ConformanceSHA256    string                     `json:"conformance_sha256"`
	Implementation       string                     `json:"implementation"`
	ModelRuntimeImported bool                       `json:"model_runtime_imported"`
}

func framingConformanceRequest() FramingConformanceRequest {
	vector := func(name, class, raw, status string) FramingVector {
		return FramingVector{
			Name: name, ArtifactClass: class, DataBase64: base64.StdEncoding.EncodeToString([]byte(raw)),
			RequireObject: true, ExpectedStatus: status,
		}
	}
	return FramingConformanceRequest{
		SchemaVersion: FramingConformanceSchema + "/request",
		Vectors: []FramingVector{
			vector("valid_file", "file", "{\"a\":1}\n", FramingValid),
			vector("valid_frame", "frame", "{\"a\":1}\n", FramingValid),
			vector("missing_lf", "file", "{\"a\":1}", FramingMissingLF),
			vector("double_lf", "file", "{\"a\":1}\n\n", FramingMultipleLF),
			vector("crlf", "file", "{\"a\":1}\r\n", FramingCarriageReturn),
			vector("trailing_space", "file", "{\"a\":1} \n", FramingNonCanonical),
			vector("trailing_prose", "file", "{\"a\":1}prose\n", FramingTrailingBytes),
			vector("second_value", "file", "{\"a\":1}{\"b\":2}\n", FramingTrailingBytes),
			vector("leading_space", "file", " {\"a\":1}\n", FramingLeadingWhitespace),
			vector("duplicate_key", "file", "{\"a\":1,\"a\":2}\n", FramingDuplicateKey),
			vector("nested_duplicate_key", "file", "{\"a\":{\"b\":1,\"b\":2}}\n", FramingDuplicateKey),
			vector("nan", "file", "{\"a\":NaN}\n", FramingNonFinite),
			vector("infinity", "file", "{\"a\":Infinity}\n", FramingNonFinite),
			vector("negative_infinity", "file", "{\"a\":-Infinity}\n", FramingNonFinite),
			vector("out_of_range_exponent", "file", "{\"a\":1e10000}\n", FramingNumberRange),
			vector("invalid_utf8", "file", "{\"a\":\"\xff\"}\n", FramingInvalidJSON),
			vector("nan_string", "file", "{\"a\":\":NaN\"}\n", FramingValid),
			vector("negative_infinity_string", "file", "{\"a\":\"x:-Infinity\"}\n", FramingValid),
			vector("small_exponent", "file", "{\"a\":1e-7}\n", FramingValid),
			vector("negative_zero", "file", "{\"a\":-0.0}\n", FramingNonCanonical),
			vector("large_decimal_form", "file", "{\"a\":1e+16}\n", FramingNonCanonical),
			vector("array", "file", "[]\n", FramingNonObject),
			vector("string", "file", "\"x\"\n", FramingNonObject),
			vector("number", "file", "1\n", FramingNonObject),
			vector("boolean", "file", "true\n", FramingNonObject),
			vector("null", "file", "null\n", FramingNonObject),
			vector("frame_missing_lf", "frame", "{\"a\":1}", FramingMissingLF),
			vector("frame_double_lf", "frame", "{\"a\":1}\n\n", FramingMultipleLF),
			vector("frame_crlf", "frame", "{\"a\":1}\r\n", FramingCarriageReturn),
			vector("frame_trailing_space", "frame", "{\"a\":1} \n", FramingNonCanonical),
			vector("frame_leading_space", "frame", " {\"a\":1}\n", FramingLeadingWhitespace),
		},
	}
}

func evaluateFramingConformance(request FramingConformanceRequest) (FramingConformanceResponse, error) {
	if request.SchemaVersion != FramingConformanceSchema+"/request" || len(request.Vectors) == 0 {
		return FramingConformanceResponse{}, fmt.Errorf("invalid framing conformance request")
	}
	results := make([]FramingConformanceResult, 0, len(request.Vectors))
	for _, vector := range request.Vectors {
		raw, err := base64.StdEncoding.DecodeString(vector.DataBase64)
		if err != nil {
			return FramingConformanceResponse{}, fmt.Errorf("decode vector %s: %w", vector.Name, err)
		}
		var value any
		switch vector.ArtifactClass {
		case "file":
			if vector.RequireObject {
				err = strictJSONObjectFileDecode(raw, &value)
			} else {
				err = strictJSONFileDecode(raw, &value)
			}
		case "frame":
			err = strictJSONFrameDecode(raw, &value)
		default:
			return FramingConformanceResponse{}, fmt.Errorf("unknown artifact class %q", vector.ArtifactClass)
		}
		status := framingStatus(err)
		if status != vector.ExpectedStatus {
			return FramingConformanceResponse{}, fmt.Errorf(
				"framing vector %s: got %s, want %s", vector.Name, status, vector.ExpectedStatus,
			)
		}
		results = append(results, FramingConformanceResult{Name: vector.Name, Status: status})
	}
	requestDigest, err := DigestDomain("framing-conformance-request", request)
	if err != nil {
		return FramingConformanceResponse{}, err
	}
	response := FramingConformanceResponse{
		SchemaVersion: FramingConformanceSchema + "/response",
		RequestSHA256: requestDigest, Results: results, Implementation: "go",
		ModelRuntimeImported: false,
	}
	response.ConformanceSHA256, err = DigestDomain("framing-conformance", framingConformanceProjection(response))
	if err != nil {
		return FramingConformanceResponse{}, err
	}
	return response, nil
}

func framingConformanceProjection(response FramingConformanceResponse) FramingConformanceResponse {
	response.ConformanceSHA256 = ""
	response.Implementation = ""
	return response
}
