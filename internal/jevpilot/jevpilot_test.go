package jevpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
)

type fakeTransport struct {
	t          *testing.T
	responses  map[string]APIResponse
	calls      int
	seenBodies [][]byte
	err        error
	fixedID    string
}

func (transport *fakeTransport) Do(request *http.Request) (*http.Response, error) {
	transport.calls++
	body, err := io.ReadAll(request.Body)
	if err != nil {
		transport.t.Fatal(err)
	}
	transport.seenBodies = append(transport.seenBodies, body)
	if request.Header.Get("Authorization") != "Bearer test-secret-key" {
		transport.t.Fatal("authorization header was not mapped correctly")
	}
	if transport.err != nil {
		return nil, transport.err
	}
	var input APIRequest
	if err := json.Unmarshal(body, &input); err != nil {
		transport.t.Fatal(err)
	}
	response, ok := transport.responses[input.State]
	if !ok {
		transport.t.Fatalf("unexpected state")
	}
	raw, err := canonicalJSON(response)
	if err != nil {
		transport.t.Fatal(err)
	}
	requestID := transport.fixedID
	if requestID == "" {
		requestID = "fake-request-" + response.Model + "-" + strconv.Itoa(transport.calls)
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Header:     http.Header{"X-Typesafe-Request-Id": []string{requestID}},
		Body:       io.NopCloser(bytes.NewReader(raw)),
	}, nil
}

func TestPrepareRunAndValidateWithFakeTransport(t *testing.T) {
	root := realTempDir(t)
	pilotDirectory := filepath.Join(root, "pilot")
	if _, err := studypilot.Prepare(pilotDirectory); err != nil {
		t.Fatal(err)
	}
	pilot, _, err := loadPilot(pilotDirectory)
	if err != nil {
		t.Fatal(err)
	}
	modelListPath := filepath.Join(root, "models.json")
	modelList := []byte(`{"models":[{"description":"stable alias","name":"jev-latest","release_date":"2026-09-10T18:38:01Z"},{"description":"preview alias","name":"jev-preview","release_date":"2026-09-10T18:39:06Z"}]}`)
	if err := os.WriteFile(modelListPath, modelList, 0o600); err != nil {
		t.Fatal(err)
	}
	requestIDPath := filepath.Join(root, "models-request-id.txt")
	if err := os.WriteFile(requestIDPath, []byte("model-list-request"), 0o600); err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(root, "run")
	authorization, err := Prepare(PrepareOptions{
		PilotDirectory: pilotDirectory, OutputDirectory: runDirectory,
		ModelListPath: modelListPath, ModelListRequestIDPath: requestIDPath,
		CreatedAt: time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
	})
	if err != nil {
		t.Fatal(err)
	}
	if authorization.ScheduledCalls != 28 || authorization.AuthorizedBudgetUSD != "5.000000" {
		t.Fatalf("unexpected authorization: %+v", authorization)
	}
	responses := make(map[string]APIResponse, len(pilot.Requests))
	cases := make(map[string]studypilot.CaseRecord, len(pilot.Cases))
	for _, record := range pilot.Cases {
		cases[record.CaseID] = record
	}
	for _, request := range pilot.Requests {
		responses[request.State.Content] = fakeResponse(request, cases[request.CaseID])
	}
	transport := &fakeTransport{t: t, responses: responses}
	summary, err := Run(context.Background(), RunOptions{
		PilotDirectory: pilotDirectory, RunDirectory: runDirectory,
		APIKey: "test-secret-key", Transport: transport,
	})
	if err != nil {
		t.Fatal(err)
	}
	if summary.CompletedCalls != 28 || summary.FailedCalls != 0 || transport.calls != 28 {
		t.Fatalf("unexpected run summary: %+v calls=%d", summary, transport.calls)
	}
	if summary.InferredCostNanoUSD != int64(summary.InputTokens)*InputNanoUSD ||
		summary.InferredCostNanoUSD >= AuthorizedNanoUSD {
		t.Fatalf("invalid budget accounting: %+v", summary)
	}
	validated, err := ValidateRun(pilotDirectory, runDirectory)
	if err != nil {
		t.Fatal(err)
	}
	if validated.ReceiptCollectionSHA256 != summary.ReceiptCollectionSHA256 {
		t.Fatal("validated receipt collection digest changed")
	}
	err = filepath.Walk(runDirectory, func(path string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.Mode().IsRegular() {
			data, readErr := os.ReadFile(path)
			if readErr != nil {
				return readErr
			}
			if bytes.Contains(data, []byte("test-secret-key")) || info.Mode().Perm()&0o077 != 0 {
				t.Fatalf("credential leak or permissive mode in %s", path)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestMappingRejectsAliasesAndAdversarialProbabilities(t *testing.T) {
	pilotDirectory := filepath.Join(realTempDir(t), "pilot")
	if _, err := studypilot.Prepare(pilotDirectory); err != nil {
		t.Fatal(err)
	}
	pilot, _, err := loadPilot(pilotDirectory)
	if err != nil {
		t.Fatal(err)
	}
	client, err := NewClient("test-secret-key", &fakeTransport{t: t})
	if err != nil {
		t.Fatal(err)
	}
	client.model = "jev-latest"
	if _, _, err := client.buildRequest(pilot.Requests[0]); err == nil {
		t.Fatal("moving alias unexpectedly accepted")
	}
	response := fakeResponse(pilot.Requests[0], pilot.Cases[0])
	response.Answers[pilot.Requests[0].Questions[0].Field].Probabilities["yes"] = 1.1
	raw, _ := json.Marshal(response)
	if _, err := parseAPIResponse(raw, pilot.Requests[0]); err == nil {
		t.Fatal("out-of-range probability unexpectedly accepted")
	}
	response = fakeResponse(pilot.Requests[0], pilot.Cases[0])
	delete(response.Answers[pilot.Requests[0].Questions[0].Field].Probabilities, "unknown")
	raw, _ = json.Marshal(response)
	if _, err := parseAPIResponse(raw, pilot.Requests[0]); err == nil {
		t.Fatal("incomplete probability vector unexpectedly accepted")
	}
}

func TestZeroRetryTransportFailureAndBudgetGuard(t *testing.T) {
	pilotDirectory := filepath.Join(realTempDir(t), "pilot")
	if _, err := studypilot.Prepare(pilotDirectory); err != nil {
		t.Fatal(err)
	}
	pilot, _, err := loadPilot(pilotDirectory)
	if err != nil {
		t.Fatal(err)
	}
	transport := &fakeTransport{t: t, err: context.DeadlineExceeded}
	client, err := NewClient("test-secret-key", transport)
	if err != nil {
		t.Fatal(err)
	}
	result := client.call(context.Background(), pilot.Requests[0])
	if result.Err == nil || result.ErrorCode != "transport_failure" || transport.calls != 1 {
		t.Fatalf("transport was retried or not preserved: %+v calls=%d", result, transport.calls)
	}
	if strings.Contains(result.Err.Error(), "test-secret-key") {
		t.Fatal("transport error leaked key")
	}
	if err := reserveBudget(AuthorizedNanoUSD-WorstCallNanoUSD, AuthorizedNanoUSD); err != nil {
		t.Fatal(err)
	}
	if err := reserveBudget(AuthorizedNanoUSD-WorstCallNanoUSD+1, AuthorizedNanoUSD); err == nil {
		t.Fatal("budget guard allowed an unsafe reservation")
	}
}

func TestFailedCallStopsWithoutRetriesAndRecordsRemainder(t *testing.T) {
	root := realTempDir(t)
	pilotDirectory := filepath.Join(root, "pilot")
	if _, err := studypilot.Prepare(pilotDirectory); err != nil {
		t.Fatal(err)
	}
	runDirectory := prepareTestAuthorization(t, root, pilotDirectory)
	transport := &fakeTransport{t: t, err: context.DeadlineExceeded}
	summary, err := Run(context.Background(), RunOptions{
		PilotDirectory: pilotDirectory, RunDirectory: runDirectory,
		APIKey: "test-secret-key", Transport: transport,
	})
	if err != nil {
		t.Fatal(err)
	}
	if transport.calls != 1 || summary.FailedCalls != 1 || summary.NotAttemptedCalls != 27 {
		t.Fatalf("failed call was retried or remainder was not preserved: %+v calls=%d", summary, transport.calls)
	}
	if _, err := Run(context.Background(), RunOptions{
		PilotDirectory: pilotDirectory, RunDirectory: runDirectory,
		APIKey: "test-secret-key", Transport: transport,
	}); err == nil {
		t.Fatal("duplicate execution unexpectedly accepted")
	}
}

func TestExecutionScheduleRejectsDuplicateCall(t *testing.T) {
	pilotDirectory := filepath.Join(realTempDir(t), "pilot")
	if _, err := studypilot.Prepare(pilotDirectory); err != nil {
		t.Fatal(err)
	}
	pilot, _, err := loadPilot(pilotDirectory)
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := buildExecutionSchedule(pilot)
	if err != nil {
		t.Fatal(err)
	}
	schedule[1].ScheduledCallID = schedule[0].ScheduledCallID
	cases, requests, _, err := indexPilot(pilot)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateExecutionSchedule(schedule, cases, requests); err == nil {
		t.Fatal("duplicate scheduled call unexpectedly accepted")
	}
}

func TestDuplicateProviderRequestIDInvalidatesRun(t *testing.T) {
	root := realTempDir(t)
	pilotDirectory := filepath.Join(root, "pilot")
	if _, err := studypilot.Prepare(pilotDirectory); err != nil {
		t.Fatal(err)
	}
	pilot, _, err := loadPilot(pilotDirectory)
	if err != nil {
		t.Fatal(err)
	}
	runDirectory := prepareTestAuthorization(t, root, pilotDirectory)
	cases := make(map[string]studypilot.CaseRecord, len(pilot.Cases))
	for _, record := range pilot.Cases {
		cases[record.CaseID] = record
	}
	responses := make(map[string]APIResponse, len(pilot.Requests))
	for _, request := range pilot.Requests {
		responses[request.State.Content] = fakeResponse(request, cases[request.CaseID])
	}
	transport := &fakeTransport{t: t, responses: responses, fixedID: "replayed-request-id"}
	summary, err := Run(context.Background(), RunOptions{
		PilotDirectory: pilotDirectory, RunDirectory: runDirectory,
		APIKey: "test-secret-key", Transport: transport,
	})
	if err == nil || summary.CompletedCalls != 1 || summary.FailedCalls != 1 ||
		summary.NotAttemptedCalls != 26 || transport.calls != 2 {
		t.Fatalf("duplicate request ID did not fail closed: summary=%+v calls=%d err=%v", summary, transport.calls, err)
	}
	if _, err := ValidateRun(pilotDirectory, runDirectory); err == nil {
		t.Fatal("offline validation accepted duplicate provider request IDs")
	}
}

func TestPrepareRejectsDuplicateOutputAndMissingModelEvidence(t *testing.T) {
	root := realTempDir(t)
	pilotDirectory := filepath.Join(root, "pilot")
	if _, err := studypilot.Prepare(pilotDirectory); err != nil {
		t.Fatal(err)
	}
	modelListPath := filepath.Join(root, "models.json")
	if err := os.WriteFile(modelListPath, []byte(`{"models":[]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	requestIDPath := filepath.Join(root, "request-id")
	if err := os.WriteFile(requestIDPath, []byte("request"), 0o600); err != nil {
		t.Fatal(err)
	}
	output := filepath.Join(root, "run")
	if _, err := Prepare(PrepareOptions{
		PilotDirectory: pilotDirectory, OutputDirectory: output,
		ModelListPath: modelListPath, ModelListRequestIDPath: requestIDPath,
	}); err == nil {
		t.Fatal("empty model list unexpectedly accepted")
	}
	if err := os.Mkdir(output, 0o700); err != nil {
		t.Fatal(err)
	}
	if _, err := Prepare(PrepareOptions{
		PilotDirectory: pilotDirectory, OutputDirectory: output,
		ModelListPath: modelListPath, ModelListRequestIDPath: requestIDPath,
	}); err == nil {
		t.Fatal("existing output directory unexpectedly accepted")
	}
}

func prepareTestAuthorization(t *testing.T, root, pilotDirectory string) string {
	t.Helper()
	modelListPath := filepath.Join(root, "models-valid.json")
	modelList := []byte(`{"models":[{"description":"stable alias","name":"jev-latest","release_date":"2026-09-10T18:38:01Z"},{"description":"preview alias","name":"jev-preview","release_date":"2026-09-10T18:39:06Z"}]}`)
	if err := os.WriteFile(modelListPath, modelList, 0o600); err != nil {
		t.Fatal(err)
	}
	requestIDPath := filepath.Join(root, "models-valid-request-id")
	if err := os.WriteFile(requestIDPath, []byte("model-list-request"), 0o600); err != nil {
		t.Fatal(err)
	}
	runDirectory := filepath.Join(root, "run")
	if _, err := Prepare(PrepareOptions{
		PilotDirectory: pilotDirectory, OutputDirectory: runDirectory,
		ModelListPath: modelListPath, ModelListRequestIDPath: requestIDPath,
		CreatedAt: time.Date(2026, 9, 20, 12, 0, 0, 0, time.UTC),
	}); err != nil {
		t.Fatal(err)
	}
	return runDirectory
}

func realTempDir(t *testing.T) string {
	t.Helper()
	directory, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	return directory
}

func fakeResponse(request studypilot.RequestRecord, record studypilot.CaseRecord) APIResponse {
	expected := map[string]string{
		"evidence_complete":         record.Evidence.EvidenceComplete,
		"observed_tests_support":    record.Evidence.ObservedTestsSupport,
		"verifier_support":          record.Evidence.VerifierSupport,
		"cleanup_complete":          record.Evidence.CleanupComplete,
		"external_effects_resolved": record.Evidence.ExternalEffectsResolved,
		"patch_risk":                record.Evidence.PatchRisk,
		"recommended_disposition":   record.Evidence.RecommendedDisposition,
	}
	answers := make(map[string]ChoiceAnswer, len(request.Questions))
	for _, question := range request.Questions {
		selected := expected[question.Field]
		probabilities := make(map[string]float64, len(question.AllowedChoices))
		remaining := 0.2 / float64(len(question.AllowedChoices)-1)
		for _, choice := range question.AllowedChoices {
			probabilities[choice] = remaining
		}
		probabilities[selected] = 0.8
		answers[question.Field] = ChoiceAnswer{
			Type: "choice", Choice: selected, Probabilities: probabilities, Confidence: 0.7,
		}
	}
	return APIResponse{Model: ModelID, Answers: answers, Usage: Usage{InputTokens: 100, OutputTokens: 25}}
}
