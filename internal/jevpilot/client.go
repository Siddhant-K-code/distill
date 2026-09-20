package jevpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"strings"
	"time"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
)

type HTTPDoer interface {
	Do(*http.Request) (*http.Response, error)
}

type Client struct {
	apiKey    string
	endpoint  string
	model     string
	transport HTTPDoer
	now       func() time.Time
}

func NewClient(apiKey string, transport HTTPDoer) (*Client, error) {
	if strings.TrimSpace(apiKey) == "" {
		return nil, fmt.Errorf("TYPESAFE_API_KEY is required")
	}
	if transport == nil {
		transport = &http.Client{
			Timeout: RequestTimeout,
			CheckRedirect: func(*http.Request, []*http.Request) error {
				return http.ErrUseLastResponse
			},
		}
	}
	return &Client{
		apiKey: apiKey, endpoint: APIEndpoint, model: ModelID,
		transport: transport, now: func() time.Time { return time.Now().UTC() },
	}, nil
}

func (client *Client) buildRequest(record studypilot.RequestRecord) (APIRequest, []byte, error) {
	if movingModelAlias(client.model) {
		return APIRequest{}, nil, fmt.Errorf("moving model aliases are forbidden")
	}
	if client.model != ModelID {
		return APIRequest{}, nil, fmt.Errorf("model %q does not match authorization", client.model)
	}
	questions := make(map[string]APIQuestion, len(record.Questions))
	for _, question := range record.Questions {
		if question.Kind != "choice" {
			return APIRequest{}, nil, fmt.Errorf("question %q has unsupported kind %q", question.Field, question.Kind)
		}
		if _, exists := questions[question.Field]; exists {
			return APIRequest{}, nil, fmt.Errorf("duplicate question %q", question.Field)
		}
		criteria := make(map[string]any, len(question.AllowedChoices))
		for _, choice := range question.AllowedChoices {
			criteria[choice] = nil
		}
		questions[question.Field] = APIQuestion{
			Type: "choice", Instructions: question.Question, Criteria: criteria,
		}
	}
	request := APIRequest{State: record.State.Content, Model: client.model, Questions: questions}
	body, err := canonicalJSON(request)
	if err != nil {
		return APIRequest{}, nil, fmt.Errorf("encode provider request: %w", err)
	}
	return request, body, nil
}

func (client *Client) call(ctx context.Context, record studypilot.RequestRecord) callResult {
	_, body, err := client.buildRequest(record)
	if err != nil {
		return callResult{RequestBody: body, Err: err, ErrorStage: "preflight", ErrorCode: "invalid_request_mapping"}
	}
	request, err := http.NewRequestWithContext(ctx, http.MethodPost, client.endpoint, bytes.NewReader(body))
	if err != nil {
		return callResult{RequestBody: body, Err: fmt.Errorf("construct request"), ErrorStage: "transport", ErrorCode: "request_construction_failed"}
	}
	request.Header.Set("Authorization", "Bearer "+client.apiKey)
	request.Header.Set("Accept", "application/json")
	request.Header.Set("Content-Type", "application/json")
	request.Header.Set("User-Agent", "distill-jev-provider-pilot/1")
	started := client.now()
	response, err := client.transport.Do(request)
	finished := client.now()
	result := callResult{RequestBody: body, StartedAt: started, FinishedAt: finished}
	if err != nil {
		result.Err = fmt.Errorf("provider transport failed")
		result.ErrorStage = "transport"
		result.ErrorCode = "transport_failure"
		return result
	}
	result.Metadata = ResponseMetadata{
		HTTPStatus: response.StatusCode, ProviderRequestID: response.Header.Get("x-typesafe-request-id"),
	}
	raw, readErr := io.ReadAll(io.LimitReader(response.Body, ResponseLimitBytes+1))
	closeErr := response.Body.Close()
	result.RawBody = raw
	if readErr != nil {
		result.Err = fmt.Errorf("read provider response")
		result.ErrorStage = "transport"
		result.ErrorCode = "response_read_failed"
		return result
	}
	if closeErr != nil {
		result.Err = fmt.Errorf("close provider response")
		result.ErrorStage = "transport"
		result.ErrorCode = "response_close_failed"
		return result
	}
	if len(raw) > ResponseLimitBytes {
		result.Err = fmt.Errorf("provider response exceeds limit")
		result.ErrorStage = "parse"
		result.ErrorCode = "response_too_large"
		return result
	}
	if usage, usageErr := parseUsageEnvelope(raw); usageErr == nil {
		result.ObservedUsage = &usage
	}
	if response.StatusCode != http.StatusOK {
		result.Err = fmt.Errorf("provider returned HTTP %d", response.StatusCode)
		result.ErrorStage = "provider"
		result.ErrorCode = fmt.Sprintf("http_%d", response.StatusCode)
		return result
	}
	if result.Metadata.ProviderRequestID == "" {
		result.Err = fmt.Errorf("provider response lacks request identity")
		result.ErrorStage = "parse"
		result.ErrorCode = "missing_provider_request_id"
		return result
	}
	parsed, err := parseAPIResponse(raw, record)
	if err != nil {
		result.Err = err
		result.ErrorStage = "response"
		result.ErrorCode = "invalid_provider_response"
		return result
	}
	result.Response = &parsed
	result.ObservedUsage = &parsed.Usage
	return result
}

func parseUsageEnvelope(raw []byte) (Usage, error) {
	var envelope struct {
		Usage json.RawMessage `json:"usage"`
	}
	if err := json.Unmarshal(raw, &envelope); err != nil {
		return Usage{}, err
	}
	if len(envelope.Usage) == 0 || bytes.Equal(bytes.TrimSpace(envelope.Usage), []byte("null")) {
		return Usage{}, fmt.Errorf("provider response omits usage")
	}
	var usage Usage
	if err := json.Unmarshal(envelope.Usage, &usage); err != nil {
		return Usage{}, err
	}
	if !usage.inputPresent || !usage.outputPresent ||
		usage.InputTokens < 0 || int64(usage.InputTokens) > MaxInputTokens ||
		usage.OutputTokens < 0 || int64(usage.OutputTokens) > MaxOutputTokens {
		return Usage{}, fmt.Errorf("provider response has invalid usage")
	}
	return usage, nil
}

func parseAPIResponse(raw []byte, request studypilot.RequestRecord) (APIResponse, error) {
	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	var response APIResponse
	if err := decoder.Decode(&response); err != nil {
		return APIResponse{}, fmt.Errorf("decode provider response: %w", err)
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return APIResponse{}, fmt.Errorf("provider response contains trailing JSON")
	}
	if response.Model != ModelID {
		return APIResponse{}, fmt.Errorf("resolved model %q does not match authorization", response.Model)
	}
	if len(response.Answers) != len(request.Questions) {
		return APIResponse{}, fmt.Errorf("provider answer cardinality mismatch")
	}
	for _, question := range request.Questions {
		answer, ok := response.Answers[question.Field]
		if !ok {
			return APIResponse{}, fmt.Errorf("provider omitted answer %q", question.Field)
		}
		if answer.Type != "choice" || !contains(question.AllowedChoices, answer.Choice) {
			return APIResponse{}, fmt.Errorf("provider answer %q has invalid type or selection", question.Field)
		}
		if len(answer.Probabilities) != len(question.AllowedChoices) {
			return APIResponse{}, fmt.Errorf("provider answer %q has incomplete probabilities", question.Field)
		}
		sum := 0.0
		maximum := -1.0
		for _, choice := range question.AllowedChoices {
			probability, ok := answer.Probabilities[choice]
			if !ok || !finiteProbability(probability) {
				return APIResponse{}, fmt.Errorf("provider answer %q has invalid probability", question.Field)
			}
			sum += probability
			maximum = math.Max(maximum, probability)
		}
		if math.Abs(sum-1) > question.AnswerContract.SumTolerance {
			return APIResponse{}, fmt.Errorf("provider answer %q probabilities do not sum to one", question.Field)
		}
		if math.Abs(answer.Probabilities[answer.Choice]-maximum) > 1e-12 {
			return APIResponse{}, fmt.Errorf("provider answer %q selection is not maximal", question.Field)
		}
		if !answer.confidencePresent || !finiteProbability(answer.Confidence) {
			return APIResponse{}, fmt.Errorf("provider answer %q has invalid confidence", question.Field)
		}
	}
	if _, err := parseUsageEnvelope(raw); err != nil {
		return APIResponse{}, fmt.Errorf("provider response has invalid usage")
	}
	return response, nil
}

func movingModelAlias(value string) bool {
	lower := strings.ToLower(value)
	for _, token := range []string{"latest", "stable", "preview", "newest"} {
		for _, delimiter := range []string{"-", "_", "/", ":", "@", "."} {
			if lower == token || strings.HasPrefix(lower, token+delimiter) ||
				strings.HasSuffix(lower, delimiter+token) || strings.Contains(lower, delimiter+token+delimiter) {
				return true
			}
		}
	}
	return false
}
