package jevpilot

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
)

type RunOptions struct {
	PilotDirectory string
	RunDirectory   string
	APIKey         string
	Transport      HTTPDoer
}

type RunSummary struct {
	ScheduledCalls          int
	AttemptedCalls          int
	CompletedCalls          int
	FailedCalls             int
	NotAttemptedCalls       int
	InputTokens             int
	OutputTokens            int
	InferredCostNanoUSD     int64
	ProviderReportedCostUSD *string
	AuthorizationSHA256     string
	ScheduleSHA256          string
	ReceiptCollectionSHA256 string
}

func Run(ctx context.Context, options RunOptions) (RunSummary, error) {
	pilot, _, err := loadPilot(options.PilotDirectory)
	if err != nil {
		return RunSummary{}, err
	}
	authorizationBytes, providerBytes, scheduleBytes, authorization, provider, schedule, err := loadAuthorization(options.RunDirectory)
	if err != nil {
		return RunSummary{}, err
	}
	if err := validateAuthorizationAgainstPilot(pilot, authorizationBytes, providerBytes, scheduleBytes, authorization, provider, schedule); err != nil {
		return RunSummary{}, err
	}
	if err := validateModelEvidence(options.RunDirectory, provider); err != nil {
		return RunSummary{}, err
	}
	cases, requests, _, err := indexPilot(pilot)
	if err != nil {
		return RunSummary{}, err
	}
	if err := validateExecutionSchedule(schedule, cases, requests); err != nil {
		return RunSummary{}, err
	}
	client, err := NewClient(options.APIKey, options.Transport)
	if err != nil {
		return RunSummary{}, err
	}
	callsDirectory := filepath.Join(options.RunDirectory, "calls")
	if err := os.Mkdir(callsDirectory, 0o700); err != nil {
		return RunSummary{}, fmt.Errorf("create fresh calls directory: %w", err)
	}
	ledgerPath := filepath.Join(options.RunDirectory, "call-ledger.jsonl")
	ledger, err := os.OpenFile(ledgerPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL|os.O_APPEND, 0o600)
	if err != nil {
		return RunSummary{}, fmt.Errorf("create call ledger: %w", err)
	}
	defer func() {
		_ = ledger.Close()
	}()

	summary := RunSummary{
		ScheduledCalls: len(schedule), AuthorizationSHA256: digest(authorizationBytes),
		ScheduleSHA256: digest(scheduleBytes),
	}
	receipts := make([][]byte, 0, len(schedule))
	registries := make(map[string]studypilot.ReceiptRegistry, len(schedule))
	providerRequestIDs := make(map[string]bool, len(schedule))
	duplicateProviderID := false
	stopped := false
	stopCode := ""
	stopMessage := ""
	for _, entry := range schedule {
		caseRecord := cases[entry.CaseID]
		requestRecord := requests[entry.RequestID]
		baseSchedule, ok := scheduleForCase(pilot.Schedules, entry.CaseID)
		if !ok {
			return summary, fmt.Errorf("missing base schedule for %q", entry.CaseID)
		}
		_, requestBody, mapErr := client.buildRequest(requestRecord)
		if mapErr != nil {
			return summary, mapErr
		}
		callDirectory := filepath.Join(callsDirectory, fmt.Sprintf("%03d-%s", entry.ScheduleIndex, entry.ScheduledCallID))
		if err := os.Mkdir(callDirectory, 0o700); err != nil {
			return summary, fmt.Errorf("create call directory: %w", err)
		}
		if err := writeExclusive(filepath.Join(callDirectory, "request.json"), requestBody); err != nil {
			return summary, err
		}

		var result callResult
		var reservationBytes []byte
		var encodeErr error
		if stopped {
			result = callResult{
				RequestBody: requestBody, Err: fmt.Errorf("%s", stopMessage),
				ErrorStage: "scheduler", ErrorCode: stopCode, NotAttempted: true,
			}
			summary.NotAttemptedCalls++
		} else if err := reserveBudget(summary.InferredCostNanoUSD, authorization.AuthorizedBudgetNanoUSD); err != nil {
			stopped, stopCode, stopMessage = true, "budget_reservation_failed", err.Error()
			result = callResult{
				RequestBody: requestBody, Err: fmt.Errorf("%s", stopMessage),
				ErrorStage: "scheduler", ErrorCode: stopCode, NotAttempted: true,
			}
			summary.NotAttemptedCalls++
		} else {
			reservationBytes, encodeErr = canonicalJSON(map[string]any{
				"scheduled_call_id": entry.ScheduledCallID,
				"request_sha256":    digest(requestBody),
				"reserved_nano_usd": WorstCallNanoUSD,
				"reserved_at":       time.Now().UTC().Format(time.RFC3339Nano),
			})
			if encodeErr != nil {
				return summary, encodeErr
			}
			if err := writeExclusive(filepath.Join(callDirectory, "attempt-reservation.json"), reservationBytes); err != nil {
				return summary, err
			}
			if err := syncDirectory(callDirectory); err != nil {
				return summary, err
			}
			if err := syncDirectory(callsDirectory); err != nil {
				return summary, err
			}
			result = client.call(ctx, requestRecord)
			summary.AttemptedCalls++
			if result.Metadata.ProviderRequestID != "" && providerRequestIDs[result.Metadata.ProviderRequestID] {
				duplicateProviderID = true
				result.Response = nil
				result.Err = fmt.Errorf("provider request identity was reused")
				result.ErrorStage = "parse"
				result.ErrorCode = "duplicate_provider_request_id"
			}
			if result.Metadata.ProviderRequestID != "" {
				providerRequestIDs[result.Metadata.ProviderRequestID] = true
			}
			if result.ObservedUsage != nil {
				inferredCost := int64(result.ObservedUsage.InputTokens) * InputNanoUSD
				if inferredCost > authorization.AuthorizedBudgetNanoUSD-summary.InferredCostNanoUSD {
					result.Response = nil
					result.Err = fmt.Errorf("provider usage exceeded the reserved budget")
					result.ErrorStage = "receipt"
					result.ErrorCode = "provider_usage_exceeded_budget"
				} else {
					summary.InputTokens += result.ObservedUsage.InputTokens
					summary.OutputTokens += result.ObservedUsage.OutputTokens
					summary.InferredCostNanoUSD += inferredCost
				}
			}
			if result.Response != nil {
				summary.CompletedCalls++
			} else {
				summary.FailedCalls++
				if shouldStopAfterFailure(result) {
					stopped, stopCode, stopMessage = true, "prior_call_failed", "execution stopped after an integrity or authorization failure; no retry or replacement was attempted"
				}
			}
		}
		if len(result.RawBody) > 0 {
			if err := writeExclusive(filepath.Join(callDirectory, "raw-response.bin"), result.RawBody); err != nil {
				return summary, err
			}
		}
		if !result.NotAttempted {
			metadataBytes, encodeErr := canonicalJSON(result.Metadata)
			if encodeErr != nil {
				return summary, encodeErr
			}
			if err := writeExclusive(filepath.Join(callDirectory, "response-metadata.json"), metadataBytes); err != nil {
				return summary, err
			}
		}
		material, err := buildReceipt(receiptInput{
			Case: caseRecord, Request: requestRecord, BaseSchedule: baseSchedule,
			ExecutionSchedule: entry, Authorization: authorization, AuthorizationBytes: authorizationBytes,
			ProviderRecord: provider, ProviderBytes: providerBytes, ScheduleBytes: scheduleBytes,
			ReservationBytes: reservationBytes, Result: result,
		})
		if err != nil {
			return summary, fmt.Errorf("%s: %w", entry.ScheduledCallID, err)
		}
		if result.ObservedUsage != nil {
			if data := material.Artifacts[entry.ScheduledCallID+"-usage"]; len(data) > 0 {
				if err := writeExclusive(filepath.Join(callDirectory, "usage.json"), data); err != nil {
					return summary, err
				}
			}
		}
		if result.Err != nil {
			if data := material.Artifacts[entry.ScheduledCallID+"-sanitized-error"]; len(data) > 0 {
				if err := writeExclusive(filepath.Join(callDirectory, "error.json"), data); err != nil {
					return summary, err
				}
			}
		}
		if err := writeExclusive(filepath.Join(callDirectory, "receipt.json"), material.Receipt); err != nil {
			return summary, err
		}
		receipts = append(receipts, material.Receipt)
		registries[entry.ScheduledCallID] = material.Registry
		ledgerEntry := makeLedgerEntry(entry, result, material.Receipt, summary.InferredCostNanoUSD)
		ledgerBytes, err := canonicalJSON(ledgerEntry)
		if err != nil {
			return summary, err
		}
		if _, err := ledger.Write(append(ledgerBytes, '\n')); err != nil {
			return summary, fmt.Errorf("append call ledger: %w", err)
		}
		if err := ledger.Sync(); err != nil {
			return summary, fmt.Errorf("sync call ledger: %w", err)
		}
	}
	if err := studypilot.ValidateReceiptStageCollection(receipts, registries, "runner"); err != nil {
		return summary, fmt.Errorf("validate receipt collection: %w", err)
	}
	collection := make([]any, 0, len(receipts))
	for _, receipt := range receipts {
		collection = append(collection, digest(receipt))
	}
	collectionBytes, err := canonicalJSON(collection)
	if err != nil {
		return summary, err
	}
	summary.ReceiptCollectionSHA256 = digest(collectionBytes)
	summaryBytes, err := canonicalJSON(summary)
	if err != nil {
		return summary, err
	}
	if err := writeExclusive(filepath.Join(options.RunDirectory, "integration-summary.json"), summaryBytes); err != nil {
		return summary, err
	}
	if duplicateProviderID {
		return summary, fmt.Errorf("duplicate provider request ID invalidated the run")
	}
	return summary, nil
}

func loadAuthorization(directory string) ([]byte, []byte, []byte, Authorization, ProviderRecord, []ExecutionScheduleEntry, error) {
	authorizationBytes, err := os.ReadFile(filepath.Join(directory, "authorization.json"))
	if err != nil {
		return nil, nil, nil, Authorization{}, ProviderRecord{}, nil, err
	}
	providerBytes, err := os.ReadFile(filepath.Join(directory, "provider-record.json"))
	if err != nil {
		return nil, nil, nil, Authorization{}, ProviderRecord{}, nil, err
	}
	scheduleBytes, err := os.ReadFile(filepath.Join(directory, "execution-schedule.jsonl"))
	if err != nil {
		return nil, nil, nil, Authorization{}, ProviderRecord{}, nil, err
	}
	var authorization Authorization
	if err := json.Unmarshal(authorizationBytes, &authorization); err != nil {
		return nil, nil, nil, Authorization{}, ProviderRecord{}, nil, err
	}
	var provider ProviderRecord
	if err := json.Unmarshal(providerBytes, &provider); err != nil {
		return nil, nil, nil, Authorization{}, ProviderRecord{}, nil, err
	}
	schedule, err := parseJSONL[ExecutionScheduleEntry](scheduleBytes)
	return authorizationBytes, providerBytes, scheduleBytes, authorization, provider, schedule, err
}

func validateExecutionSchedule(
	schedule []ExecutionScheduleEntry,
	cases map[string]studypilot.CaseRecord,
	requests map[string]studypilot.RequestRecord,
) error {
	seen := make(map[string]bool, len(schedule))
	replicates := make(map[string]map[int]bool)
	for index, entry := range schedule {
		if entry.SchemaVersion != ScheduleSchema || entry.ScheduleIndex != index {
			return fmt.Errorf("execution schedule index or schema mismatch")
		}

		if seen[entry.ScheduledCallID] {
			return fmt.Errorf("duplicate scheduled call %q", entry.ScheduledCallID)
		}
		seen[entry.ScheduledCallID] = true
		caseRecord, caseOK := cases[entry.CaseID]
		requestRecord, requestOK := requests[entry.RequestID]
		if !caseOK || !requestOK || requestRecord.CaseID != entry.CaseID || caseRecord.Category != entry.Category {
			return fmt.Errorf("execution schedule references mismatched frozen record")
		}
		if entry.ReplicateIndex < 1 || entry.ReplicateIndex > 3 {
			return fmt.Errorf("invalid replicate index")
		}
		if replicates[entry.CaseID] == nil {
			replicates[entry.CaseID] = make(map[int]bool)
		}
		if replicates[entry.CaseID][entry.ReplicateIndex] {
			return fmt.Errorf("duplicate case replicate")
		}
		replicates[entry.CaseID][entry.ReplicateIndex] = true
	}
	return nil
}

func validateAuthorizationAgainstPilot(
	pilot pilotData,
	authorizationBytes, providerBytes, scheduleBytes []byte,
	authorization Authorization,
	provider ProviderRecord,
	schedule []ExecutionScheduleEntry,
) error {
	expectedSchedule, err := buildExecutionSchedule(pilot)
	if err != nil {
		return err
	}
	expectedScheduleBytes, err := canonicalJSONL(expectedSchedule)
	if err != nil {
		return err
	}
	if !strings.EqualFold(digest(scheduleBytes), digest(expectedScheduleBytes)) ||
		!strings.EqualFold(authorization.ScheduleSHA256, digest(expectedScheduleBytes)) {
		return fmt.Errorf("execution schedule differs from the fixed pilot schedule")
	}
	expectedProvider := newProviderRecord(provider.ModelListAliases, provider.ModelListResponseSHA256, provider.ModelListRequestIDSHA256)
	expectedProviderBytes, err := canonicalJSON(expectedProvider)
	if err != nil {
		return err
	}

	if !strings.EqualFold(digest(providerBytes), digest(expectedProviderBytes)) {
		return fmt.Errorf("provider record differs from the pinned contract")
	}
	createdAt, err := time.Parse(time.RFC3339Nano, authorization.CreatedAt)
	if err != nil || createdAt.After(time.Now().UTC().Add(time.Minute)) {
		return fmt.Errorf("authorization timestamp is invalid")
	}
	if authorization.SchemaVersion != AuthorizationSchema ||
		authorization.Stage != "excluded_provider_pilot" || !authorization.Excluded ||
		authorization.FinalStudyEligible || authorization.AuthorizedBudgetUSD != "5.000000" ||
		authorization.AuthorizedBudgetNanoUSD != AuthorizedNanoUSD || authorization.ModelID != ModelID ||
		authorization.PricingVersion != PricingVersion || authorization.InputPriceUSDPerMillion != "0.042000" ||
		authorization.MaxInputTokensPerCall != MaxInputTokens ||
		authorization.WorstCaseCostUSDPerCall != formatNanoUSD(WorstCallNanoUSD) ||
		authorization.ScheduledCalls != len(expectedSchedule) ||
		authorization.MaxScheduledCostUSD != formatNanoUSD(int64(len(expectedSchedule))*WorstCallNanoUSD) ||
		authorization.OfflineChecksumsSHA256 != digest(pilot.Files["SHA256SUMS"]) ||
		authorization.OfflineManifestSHA256 != digest(pilot.Files["pilot-manifest.json"]) ||
		authorization.RequestsSHA256 != digest(pilot.Files["requests.jsonl"]) ||
		authorization.ProviderRecordSHA256 != digest(expectedProviderBytes) ||
		!authorization.ZeroAdaptiveExtension ||
		!equalStrings(authorization.RepeatabilityCaseIDs, repeatabilityCaseIDs(pilot)) {
		return fmt.Errorf("authorization identity mismatch")
	}
	if len(authorization.UnresolvedExternalGates) != 4 || len(authorizationBytes) == 0 {
		return fmt.Errorf("authorization omits unresolved external gates")
	}
	if movingModelAlias(authorization.ModelID) || authorization.ModelID != provider.ModelID ||
		int64(len(schedule))*WorstCallNanoUSD > authorization.AuthorizedBudgetNanoUSD {
		return fmt.Errorf("authorization contains a moving model or unsafe budget")
	}
	return nil
}

func validateModelEvidence(directory string, provider ProviderRecord) error {
	response, err := os.ReadFile(filepath.Join(directory, "model-list-response.json"))
	if err != nil {
		return fmt.Errorf("read stored model-list response: %w", err)
	}
	requestID, err := os.ReadFile(filepath.Join(directory, "model-list-request-id.txt"))
	if err != nil {
		return fmt.Errorf("read stored model-list request identity: %w", err)
	}
	aliases, err := validateModelList(response)
	if err != nil {
		return err
	}
	if !equalStrings(aliases, provider.ModelListAliases) ||
		digest(response) != provider.ModelListResponseSHA256 ||
		digest(requestID) != provider.ModelListRequestIDSHA256 ||
		len(strings.TrimSpace(string(requestID))) == 0 {
		return fmt.Errorf("stored model-list evidence does not match provider record")
	}
	return nil
}

func reserveBudget(spentNanoUSD, capNanoUSD int64) error {
	if spentNanoUSD < 0 || capNanoUSD < 0 || spentNanoUSD > capNanoUSD-WorstCallNanoUSD {
		return fmt.Errorf("worst-case reservation %s would exceed remaining cap %s",
			formatNanoUSD(WorstCallNanoUSD), formatNanoUSD(capNanoUSD-spentNanoUSD))
	}

	return nil
}

func shouldStopAfterFailure(result callResult) bool {
	if result.ErrorCode == "duplicate_provider_request_id" ||
		result.ErrorCode == "provider_usage_exceeded_budget" ||
		result.ErrorCode == "missing_provider_request_id" ||
		result.ErrorCode == "invalid_provider_response" ||
		result.ErrorCode == "response_too_large" {
		return true
	}
	return result.Metadata.HTTPStatus == http.StatusUnauthorized || result.Metadata.HTTPStatus == http.StatusForbidden
}

func makeLedgerEntry(entry ExecutionScheduleEntry, result callResult, receipt []byte, cumulative int64) LedgerEntry {
	ledger := LedgerEntry{
		SchemaVersion: LedgerSchema, ScheduleIndex: entry.ScheduleIndex,
		ScheduledCallID: entry.ScheduledCallID, CaseID: entry.CaseID, ReplicateIndex: entry.ReplicateIndex,
		Status: "failed", RequestSHA256: digest(result.RequestBody), ReceiptSHA256: digest(receipt),
		CumulativeInferredUSD: formatNanoUSD(cumulative),
	}
	if result.NotAttempted {
		ledger.Status = "not_attempted"
		return ledger
	}
	ledger.LatencyMilliseconds = float64(result.FinishedAt.Sub(result.StartedAt).Microseconds()) / 1000
	if len(result.RawBody) > 0 {
		responseDigest := digest(result.RawBody)
		ledger.ResponseSHA256 = &responseDigest
	}
	if result.Metadata.ProviderRequestID != "" {
		requestID := result.Metadata.ProviderRequestID
		ledger.ProviderRequestID = &requestID
	}
	if result.ObservedUsage != nil {
		inputTokens, outputTokens := result.ObservedUsage.InputTokens, result.ObservedUsage.OutputTokens
		ledger.InputTokens, ledger.OutputTokens = &inputTokens, &outputTokens
		cost := formatNanoUSD(int64(inputTokens) * InputNanoUSD)
		ledger.InferredCostUSD = &cost
	}
	if result.Response != nil {
		ledger.Status = "valid"
	}
	return ledger
}

func writeExclusive(path string, data []byte) error {
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return fmt.Errorf("create %s: %w", filepath.Base(path), err)
	}
	if _, err := file.Write(data); err != nil {
		return errors.Join(err, file.Close())
	}
	if err := file.Sync(); err != nil {
		return errors.Join(err, file.Close())
	}
	return file.Close()
}

func ValidateRun(pilotDirectory, runDirectory string) (RunSummary, error) {
	pilot, _, err := loadPilot(pilotDirectory)
	if err != nil {
		return RunSummary{}, err
	}
	authorizationBytes, providerBytes, scheduleBytes, authorization, provider, schedule, err := loadAuthorization(runDirectory)
	if err != nil {
		return RunSummary{}, err
	}
	if err := validateAuthorizationAgainstPilot(pilot, authorizationBytes, providerBytes, scheduleBytes, authorization, provider, schedule); err != nil {
		return RunSummary{}, err
	}
	if err := validateModelEvidence(runDirectory, provider); err != nil {
		return RunSummary{}, err
	}
	cases, requests, _, err := indexPilot(pilot)
	if err != nil {
		return RunSummary{}, err
	}
	if err := validateExecutionSchedule(schedule, cases, requests); err != nil {
		return RunSummary{}, err
	}
	ledgerBytes, err := os.ReadFile(filepath.Join(runDirectory, "call-ledger.jsonl"))
	if err != nil {
		return RunSummary{}, err
	}
	ledgerEntries, err := parseJSONL[LedgerEntry](ledgerBytes)
	if err != nil || len(ledgerEntries) != len(schedule) {
		return RunSummary{}, fmt.Errorf("ledger/schedule cardinality mismatch")
	}
	var summary RunSummary
	summary.ScheduledCalls = len(schedule)
	summary.AuthorizationSHA256 = digest(authorizationBytes)
	summary.ScheduleSHA256 = digest(scheduleBytes)
	receipts := make([][]byte, 0, len(schedule))
	registries := make(map[string]studypilot.ReceiptRegistry, len(schedule))
	providerRequestIDs := make(map[string]bool, len(schedule))
	for _, entry := range schedule {
		callDirectory := filepath.Join(runDirectory, "calls", fmt.Sprintf("%03d-%s", entry.ScheduleIndex, entry.ScheduledCallID))
		requestBody, err := os.ReadFile(filepath.Join(callDirectory, "request.json"))
		if err != nil {
			return summary, err
		}
		mappingClient := &Client{model: ModelID}
		_, expectedRequestBody, err := mappingClient.buildRequest(requests[entry.RequestID])
		if err != nil || !bytes.Equal(requestBody, expectedRequestBody) {
			return summary, fmt.Errorf("stored provider request differs from frozen mapping for %s", entry.ScheduledCallID)
		}
		storedReceipt, err := os.ReadFile(filepath.Join(callDirectory, "receipt.json"))
		if err != nil {
			return summary, err
		}
		var receiptDocument map[string]any
		if err := json.Unmarshal(storedReceipt, &receiptDocument); err != nil {
			return summary, err
		}
		measurement, ok := receiptDocument["measurement"].(map[string]any)
		if !ok {
			return summary, fmt.Errorf("receipt measurement is missing or invalid")
		}
		result := callResult{RequestBody: requestBody}
		ledgerEntry := ledgerEntries[entry.ScheduleIndex]
		var reservationBytes []byte
		if ledgerEntry.Status != "not_attempted" {
			reservationBytes, err = os.ReadFile(filepath.Join(callDirectory, "attempt-reservation.json"))
			if err != nil {
				return summary, err
			}
			if err := validateReservation(reservationBytes, entry, requestBody, measurement); err != nil {
				return summary, err
			}
		} else if _, err := os.Lstat(filepath.Join(callDirectory, "attempt-reservation.json")); !os.IsNotExist(err) {
			return summary, fmt.Errorf("not-attempted call has an attempt reservation")
		}
		switch ledgerEntry.Status {
		case "valid":
			result.RawBody, err = os.ReadFile(filepath.Join(callDirectory, "raw-response.bin"))
			if err != nil {
				return summary, err
			}
			parsed, parseErr := parseAPIResponse(result.RawBody, requests[entry.RequestID])
			if parseErr != nil {
				return summary, parseErr
			}
			result.Response = &parsed
			result.ObservedUsage = &parsed.Usage
			if err := loadAttemptMetadata(callDirectory, measurement, &result); err != nil {
				return summary, err
			}
			if result.Metadata.HTTPStatus != http.StatusOK {
				return summary, fmt.Errorf("valid ledger entry has HTTP status %d", result.Metadata.HTTPStatus)
			}
			if providerRequestIDs[result.Metadata.ProviderRequestID] {
				return summary, fmt.Errorf("duplicate provider request ID")
			}
			providerRequestIDs[result.Metadata.ProviderRequestID] = true
			summary.AttemptedCalls++
			summary.CompletedCalls++
		case "failed":
			result.RawBody, _ = os.ReadFile(filepath.Join(callDirectory, "raw-response.bin"))
			if err := loadAttemptMetadata(callDirectory, measurement, &result); err != nil {
				return summary, err
			}
			if err := loadPreservedError(callDirectory, &result); err != nil {
				return summary, err
			}
			if usage, usageErr := parseUsageEnvelope(result.RawBody); usageErr == nil {
				result.ObservedUsage = &usage
			}
			if result.Metadata.ProviderRequestID != "" {
				if providerRequestIDs[result.Metadata.ProviderRequestID] {
					return summary, fmt.Errorf("duplicate provider request ID")
				}
				providerRequestIDs[result.Metadata.ProviderRequestID] = true
			}
			summary.AttemptedCalls++
			summary.FailedCalls++
		case "not_attempted":
			if err := loadPreservedError(callDirectory, &result); err != nil {
				return summary, err
			}
			result.NotAttempted = true
			summary.NotAttemptedCalls++
		default:
			return summary, fmt.Errorf("unknown ledger status")
		}
		if result.ObservedUsage != nil {
			summary.InputTokens += result.ObservedUsage.InputTokens
			summary.OutputTokens += result.ObservedUsage.OutputTokens
			summary.InferredCostNanoUSD += int64(result.ObservedUsage.InputTokens) * InputNanoUSD
			if summary.InferredCostNanoUSD > authorization.AuthorizedBudgetNanoUSD {
				return summary, fmt.Errorf("cumulative inferred cost exceeds authorization")
			}
		}
		baseSchedule, _ := scheduleForCase(pilot.Schedules, entry.CaseID)
		material, err := buildReceipt(receiptInput{
			Case: cases[entry.CaseID], Request: requests[entry.RequestID], BaseSchedule: baseSchedule,
			ExecutionSchedule: entry, Authorization: authorization, AuthorizationBytes: authorizationBytes,
			ProviderRecord: provider, ProviderBytes: providerBytes, ScheduleBytes: scheduleBytes,
			ReservationBytes: reservationBytes, Result: result,
		})
		if err != nil {
			return summary, err
		}
		if err := validateStoredCallArtifacts(callDirectory, result, reservationBytes, material); err != nil {
			return summary, err
		}
		expectedLedger := makeLedgerEntry(entry, result, material.Receipt, summary.InferredCostNanoUSD)
		expectedLedgerBytes, err := canonicalJSON(expectedLedger)
		if err != nil {
			return summary, err
		}
		actualLedgerBytes, err := canonicalJSON(ledgerEntry)
		if err != nil || !strings.EqualFold(digest(expectedLedgerBytes), digest(actualLedgerBytes)) {
			return summary, fmt.Errorf("ledger entry mismatch for %s", entry.ScheduledCallID)
		}
		if !strings.EqualFold(digest(storedReceipt), ledgerEntry.ReceiptSHA256) ||
			!strings.EqualFold(digest(storedReceipt), digest(material.Receipt)) {
			return summary, fmt.Errorf("stored receipt mismatch for %s", entry.ScheduledCallID)
		}

		receipts = append(receipts, storedReceipt)
		registries[entry.ScheduledCallID] = material.Registry
	}
	if err := studypilot.ValidateReceiptStageCollection(receipts, registries, "runner"); err != nil {
		return summary, err
	}
	collection := make([]any, 0, len(receipts))
	for _, receipt := range receipts {
		collection = append(collection, digest(receipt))
	}
	collectionBytes, _ := canonicalJSON(collection)
	summary.ReceiptCollectionSHA256 = digest(collectionBytes)
	if summary.AuthorizationSHA256 != digest(authorizationBytes) || authorization.ProviderRecordSHA256 != digest(providerBytes) {
		return summary, fmt.Errorf("authorization digest mismatch")
	}
	return summary, nil
}

func equalStrings(left, right []string) bool {
	if len(left) != len(right) {
		return false
	}
	for index := range left {
		if left[index] != right[index] {
			return false
		}
	}
	return true
}

func loadAttemptMetadata(callDirectory string, measurement map[string]any, result *callResult) error {
	metadataBytes, err := os.ReadFile(filepath.Join(callDirectory, "response-metadata.json"))
	if err != nil {
		return err
	}

	if err := json.Unmarshal(metadataBytes, &result.Metadata); err != nil {
		return err
	}
	startedText, startedOK := measurement["attempt_started_at"].(string)
	finishedText, finishedOK := measurement["attempt_finished_at"].(string)
	if !startedOK || !finishedOK {
		return fmt.Errorf("receipt attempt timestamps are missing or invalid")
	}
	started, err := time.Parse(time.RFC3339Nano, startedText)
	if err != nil {
		return err
	}
	finished, err := time.Parse(time.RFC3339Nano, finishedText)
	if err != nil {
		return err
	}
	result.StartedAt, result.FinishedAt = started, finished
	return nil
}

func validateReservation(data []byte, entry ExecutionScheduleEntry, requestBody []byte, measurement map[string]any) error {
	canonical, err := canonicalJSON(json.RawMessage(data))
	if err != nil || !bytes.Equal(canonical, data) {
		return fmt.Errorf("attempt reservation is not canonical JSON")
	}
	var reservation struct {
		ScheduledCallID string `json:"scheduled_call_id"`
		RequestSHA256   string `json:"request_sha256"`
		ReservedNanoUSD int64  `json:"reserved_nano_usd"`
		ReservedAt      string `json:"reserved_at"`
	}
	decoder := json.NewDecoder(bytes.NewReader(data))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&reservation); err != nil {
		return fmt.Errorf("decode attempt reservation: %w", err)
	}
	reservedAt, err := time.Parse(time.RFC3339Nano, reservation.ReservedAt)
	if err != nil {
		return fmt.Errorf("invalid reservation timestamp")
	}
	startedText, ok := measurement["attempt_started_at"].(string)
	if !ok {
		return fmt.Errorf("attempt reservation lacks matching attempt timestamp")
	}
	startedAt, err := time.Parse(time.RFC3339Nano, startedText)
	if err != nil || reservedAt.After(startedAt) {
		return fmt.Errorf("attempt reservation was not durable before the call")
	}
	if reservation.ScheduledCallID != entry.ScheduledCallID ||
		reservation.RequestSHA256 != digest(requestBody) ||
		reservation.ReservedNanoUSD != WorstCallNanoUSD {
		return fmt.Errorf("attempt reservation identity mismatch")
	}
	return nil
}

func loadPreservedError(callDirectory string, result *callResult) error {
	errorBytes, err := os.ReadFile(filepath.Join(callDirectory, "error.json"))
	if err != nil {
		return err
	}

	var preserved struct {
		Stage   string `json:"stage"`
		Code    string `json:"code"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(errorBytes, &preserved); err != nil {
		return err
	}
	result.Err = fmt.Errorf("%s", preserved.Message)
	result.ErrorStage = preserved.Stage
	result.ErrorCode = preserved.Code
	return nil
}

func validateStoredCallArtifacts(callDirectory string, result callResult, reservation []byte, material receiptMaterial) error {
	expected := map[string][]byte{
		"request.json": result.RequestBody,
		"receipt.json": material.Receipt,
	}
	if !result.NotAttempted {
		metadata, err := canonicalJSON(result.Metadata)
		if err != nil {
			return err
		}
		expected["attempt-reservation.json"] = reservation
		expected["response-metadata.json"] = metadata
	}
	if len(result.RawBody) > 0 {
		expected["raw-response.bin"] = result.RawBody
	}
	if result.ObservedUsage != nil {
		expected["usage.json"] = material.Artifacts[material.Registry.ScheduledCallID+"-usage"]
	}
	if result.Err != nil {
		expected["error.json"] = material.Artifacts[material.Registry.ScheduledCallID+"-sanitized-error"]
	}
	entries, err := os.ReadDir(callDirectory)
	if err != nil {
		return err
	}
	if len(entries) != len(expected) {
		return fmt.Errorf("call artifact set mismatch")
	}
	for _, entry := range entries {
		want, ok := expected[entry.Name()]
		if !ok || entry.IsDir() {
			return fmt.Errorf("unexpected call artifact %q", entry.Name())
		}
		actual, err := os.ReadFile(filepath.Join(callDirectory, entry.Name()))
		if err != nil {
			return err
		}
		if !bytes.Equal(actual, want) {
			return fmt.Errorf("stored call artifact %q does not match receipt registry", entry.Name())
		}
	}
	return nil
}

func syncDirectory(path string) error {
	directory, err := os.Open(path)
	if err != nil {
		return err
	}
	if err := directory.Sync(); err != nil {
		_ = directory.Close()
		return err
	}
	return directory.Close()
}
