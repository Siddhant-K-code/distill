package jevpilot

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"time"

	"github.com/Siddhant-K-code/distill/internal/studypilot"
)

type PrepareOptions struct {
	PilotDirectory         string
	OutputDirectory        string
	ModelListPath          string
	ModelListRequestIDPath string
	CreatedAt              time.Time
}

var evidenceSources = []EvidenceSource{
	{URL: "https://raw.githubusercontent.com/typesafe-ai/skills/main/skills/typesafe-ai/SKILL.md", RetrievedAt: "2026-09-20", SHA256: "71ea90d7906c6554c4f4c460ef7361b2d26f59116ccdae986dc6d997b9389f52"},
	{URL: "https://docs.typesafe.ai/llms.txt", RetrievedAt: "2026-09-20", SHA256: "9bb713532b8b5b836d48bf19733a6649525aff8353e22c5cbebf5524a498ff24"},
	{URL: "https://docs.typesafe.ai/api.md", RetrievedAt: "2026-09-20", SHA256: "7ea1c82d9d16bd06e3d38885a5292548293f91e6d1ade4682c27398ced51a9e5"},
	{URL: "https://docs.typesafe.ai/models.md", RetrievedAt: "2026-09-20", SHA256: "9d20bb3c90a0147532d0b20ddc4c64579391be7842e965543b27bad684eeb4d6"},
	{URL: "https://docs.typesafe.ai/confidence.md", RetrievedAt: "2026-09-20", SHA256: "97dafe98b77979906a2ad456013dd85a70dcecd109d0644b096817d910997ea5"},
	{URL: "https://docs.typesafe.ai/primitives/choice.md", RetrievedAt: "2026-09-20", SHA256: "4c55085131f8f2d7ae1faefdec52b1c7b379d40657d701cd2c786021cfe9dac9"},
	{URL: "https://docs.typesafe.ai/model-jaggedness/jev-1.13.md", RetrievedAt: "2026-09-20", SHA256: "e69329bd32e91ac08f0bd2681ceb4aacc34923b9190e29c15c8f75d8e22950e2"},
	{URL: "https://typesafe.ai/legal/mca", RetrievedAt: "2026-09-20", SHA256: "e8eda228c6da44f61aa09e2563765926393295b1ddefd88cd5a8f83c4f130e82"},
	{URL: "https://typesafe.ai/legal/privacy-policy", RetrievedAt: "2026-09-20", SHA256: "9819f3e11b213ce3e7e96cdaaf2b9d68fe171e436128ed21bd12bf48eefd2dc1"},
	{URL: "https://typesafe.ai/legal/data-processing", RetrievedAt: "2026-09-20", SHA256: "548d326ed3287aa35c4f60d2b03153feeb39704363a5ba1ee0a75bc0dddda3e0"},
	{URL: "https://docs.typesafe.ai/sdk/python/changelog.md", RetrievedAt: "2026-09-20", SHA256: "c49eb12a55807efbbde7dc1779efd555a34d91cd70bde070242ce9f2371094e1"},
	{URL: "https://docs.typesafe.ai/sdk/python/api/retries.md", RetrievedAt: "2026-09-20", SHA256: "bdf94b25ae1d23a0784c9e1affcf0363e80deb56089a0e48d3e3860bb5181cc9"},
	{URL: "https://docs.typesafe.ai/sdk/python/api/types/responses.md", RetrievedAt: "2026-09-20", SHA256: "7b0167d010dc165b6584939a550c8ec35d31c003f0e8095638a46a037b6f6f8a"},
}

func Prepare(options PrepareOptions) (Authorization, error) {
	if options.CreatedAt.IsZero() {
		options.CreatedAt = time.Now().UTC()
	}
	pilot, summary, err := loadPilot(options.PilotDirectory)
	if err != nil {
		return Authorization{}, fmt.Errorf("validate offline pilot: %w", err)
	}
	modelList, err := os.ReadFile(options.ModelListPath)
	if err != nil {
		return Authorization{}, fmt.Errorf("read model-list evidence: %w", err)
	}
	requestID, err := os.ReadFile(options.ModelListRequestIDPath)
	if err != nil {
		return Authorization{}, fmt.Errorf("read model-list request identity: %w", err)
	}
	aliases, err := validateModelList(modelList)
	if err != nil {
		return Authorization{}, err
	}
	if len(strings.TrimSpace(string(requestID))) == 0 {
		return Authorization{}, fmt.Errorf("model-list response lacks provider request identity")
	}

	schedule, err := buildExecutionSchedule(pilot)
	if err != nil {
		return Authorization{}, err
	}
	scheduleBytes, err := canonicalJSONL(schedule)
	if err != nil {
		return Authorization{}, err
	}
	provider := newProviderRecord(aliases, digest(modelList), digest(requestID))
	providerBytes, err := canonicalJSON(provider)
	if err != nil {
		return Authorization{}, err
	}
	maxCost := int64(len(schedule)) * WorstCallNanoUSD
	authorization := Authorization{
		SchemaVersion:           AuthorizationSchema,
		CreatedAt:               options.CreatedAt.UTC().Format(time.RFC3339Nano),
		Stage:                   "excluded_provider_pilot",
		Excluded:                true,
		FinalStudyEligible:      false,
		AuthorizedBudgetUSD:     "5.000000",
		AuthorizedBudgetNanoUSD: AuthorizedNanoUSD,
		ModelID:                 ModelID,
		ExecutionRuntime:        runtime.Version(),
		PricingVersion:          PricingVersion,
		InputPriceUSDPerMillion: "0.042000",
		MaxInputTokensPerCall:   MaxInputTokens,
		WorstCaseCostUSDPerCall: formatNanoUSD(WorstCallNanoUSD),
		ScheduledCalls:          len(schedule),
		MaxScheduledCostUSD:     formatNanoUSD(maxCost),
		ScheduleSHA256:          digest(scheduleBytes),
		OfflineChecksumsSHA256:  summary.ChecksumsSHA256,
		OfflineManifestSHA256:   summary.ManifestSHA256,
		RequestsSHA256:          digest(pilot.Files["requests.jsonl"]),
		ProviderRecordSHA256:    digest(providerBytes),
		ZeroAdaptiveExtension:   true,
		StopBehavior:            "Before every call reserve the documented 64k-token worst case. Pause after infrastructure failures and resume only at the next ledger-confirmed unattempted call. Stop with terminal not_attempted receipts after integrity, authorization, or budget failures.",
		RepeatabilityDesign:     "Five predeclared representative strata; each selected case has exactly three total calls, matching the preregistered repeat-call count. Replicates are not retries.",
		RepeatabilityCaseIDs:    repeatabilityCaseIDs(pilot),
		UnresolvedExternalGates: []string{
			"publication independence and disclosure terms require external collaboration",
			"held-out non-tuning requires external collaboration",
			"account-specific retention/ZDR is not exposed by the documented API",
			"provider-side numeric spend cap is not exposed by the documented API; the runner enforces a local fail-closed cap",
		},
	}
	authorizationBytes, err := canonicalJSON(authorization)
	if err != nil {
		return Authorization{}, err
	}
	if maxCost > AuthorizedNanoUSD {
		return Authorization{}, fmt.Errorf("scheduled worst-case cost %s exceeds cap", formatNanoUSD(maxCost))
	}
	if err := createAuthorizationDirectory(options.OutputDirectory, map[string][]byte{
		"authorization.json":        authorizationBytes,
		"execution-schedule.jsonl":  scheduleBytes,
		"model-list-request-id.txt": requestID,
		"model-list-response.json":  modelList,
		"provider-record.json":      providerBytes,
	}); err != nil {
		return Authorization{}, err
	}
	return authorization, nil
}

func validateModelList(data []byte) ([]string, error) {
	var response struct {
		Models []struct {
			Name        string `json:"name"`
			Description string `json:"description"`
			ReleaseDate string `json:"release_date"`
		} `json:"models"`
	}
	decoder := json.NewDecoder(strings.NewReader(string(data)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&response); err != nil {
		return nil, fmt.Errorf("decode model-list evidence: %w", err)
	}
	if len(response.Models) == 0 {
		return nil, fmt.Errorf("model-list evidence is empty")
	}
	aliases := make([]string, 0, len(response.Models))
	for _, model := range response.Models {
		if model.Name == "" || model.Description == "" || model.ReleaseDate == "" {
			return nil, fmt.Errorf("model-list evidence has incomplete model")
		}
		aliases = append(aliases, model.Name)
	}
	sort.Strings(aliases)
	if !contains(aliases, "jev-latest") || !contains(aliases, "jev-preview") {
		return nil, fmt.Errorf("account model list does not contain documented Jev aliases")
	}
	return aliases, nil
}

func newProviderRecord(aliases []string, modelListDigest, modelListRequestIDDigest string) ProviderRecord {
	return ProviderRecord{
		SchemaVersion:            ProviderSchema,
		Provider:                 ProviderName,
		ModelID:                  ModelID,
		ModelVersion:             ModelVersion,
		ModelListAliases:         aliases,
		ModelListResponseSHA256:  modelListDigest,
		ModelListRequestIDSHA256: modelListRequestIDDigest,
		APIEndpoint:              APIEndpoint,
		APIVersion:               APIVersion,
		HTTPClient:               "Go standard library net/http",
		SDKUsedForExecution:      false,
		ReferenceSDK:             "typesafe-sdk",
		ReferenceSDKVersion:      "0.7.0",
		ReferenceSDKCommit:       "2ce5c65f13646cab6e6f782328194c9d85f3300a",
		Authentication:           "Authorization: Bearer <key>; header never persisted",
		RetryPolicy:              "zero automatic retries; one transport attempt per scheduled call",
		TimeoutSeconds:           int(RequestTimeout / time.Second),
		InputPriceUSDPerMillion:  "0.042000",
		OutputPriceUSDPerMillion: "0.000000",
		MaxInputTokensPerCall:    MaxInputTokens,
		WorstCaseCostUSDPerCall:  formatNanoUSD(WorstCallNanoUSD),
		RateLimits:               "250000 tokens/second and 1200 requests/minute; limits may change without notice",
		UsageFields:              []string{"input_tokens", "output_tokens"},
		ProviderCostField:        "not present in documented response; null in semantic receipt and separately inferred from input tokens",
		RequestIDField:           "x-typesafe-request-id response header",
		ConfidenceSemantics:      "provider-defined [0,1] statistic derived from Choice probabilities; not selected-label probability",
		KnownJaggedEdges: []string{
			"literal reading", "math and numeric precision", "date and time comparison", "indirection",
			"large irrelevant state", "adversarial content", "contradictory instructions and criteria",
			"common-sense structural invariants", "generation",
		},
		ExternalGates: ExternalGates{
			SyntheticPublicDataSubmission: "permitted by the public MCA for customer inputs; fixtures are synthetic/public-domain-style and contain no personal data",
			PublicationIndependence:       "unresolved; public terms assign output to the customer but do not establish the requested collaboration agreement",
			HeldOutNonTuning:              "unresolved external collaboration gate; no held-out or final-study data is submitted in this pilot",
			DataRetention:                 "standard retention is as reasonably necessary; zero-data-retention is documented only for enterprise customers and is unresolved for this account",
			TrainingUse:                   "public MCA and privacy policy state inputs are not used to train or fine-tune models without prior consent",
			ProviderSideSpendCap:          "no documented account API; unresolved externally, with independent local fail-closed authorization",
		},
		Evidence:           append([]EvidenceSource(nil), evidenceSources...),
		FinalStudyEligible: false,
	}
}

func buildExecutionSchedule(pilot pilotData) ([]ExecutionScheduleEntry, error) {
	cases, _, _, err := indexPilot(pilot)
	if err != nil {
		return nil, err
	}
	schedule := make([]ExecutionScheduleEntry, 0, len(pilot.Schedules)+10)
	for _, base := range pilot.Schedules {
		caseRecord, ok := cases[base.CaseID]
		if !ok {
			return nil, fmt.Errorf("schedule references unknown case %q", base.CaseID)
		}
		schedule = append(schedule, ExecutionScheduleEntry{
			SchemaVersion: ScheduleSchema, ScheduleIndex: len(schedule),
			ScheduledCallID: base.ScheduledCallID, RequestID: base.RequestID,
			CaseID: base.CaseID, Category: caseRecord.Category, ReplicateIndex: 1,
		})
	}
	selected := repeatabilityCaseIDs(pilot)
	for _, caseID := range selected {
		base, ok := scheduleForCase(pilot.Schedules, caseID)
		if !ok {
			return nil, fmt.Errorf("repeatability case %q lacks base schedule", caseID)
		}
		caseRecord := cases[caseID]
		for replicate := 2; replicate <= 3; replicate++ {
			schedule = append(schedule, ExecutionScheduleEntry{
				SchemaVersion: ScheduleSchema, ScheduleIndex: len(schedule),
				ScheduledCallID: fmt.Sprintf("%s-replicate-%d", base.ScheduledCallID, replicate),
				RequestID:       base.RequestID, CaseID: caseID, Category: caseRecord.Category,
				ReplicateIndex: replicate,
			})
		}
	}
	return schedule, nil
}

func repeatabilityCaseIDs(pilot pilotData) []string {
	wanted := map[string]bool{
		"fully_complete_verified":         true,
		"missing_observed_test_evidence":  true,
		"stale_baseline":                  true,
		"unresolved_external_effects":     true,
		"contradictory_summary_execution": true,
	}
	var result []string
	for _, record := range pilot.Cases {
		if wanted[record.Category] {
			result = append(result, record.CaseID)
		}
	}
	return result
}

func scheduleForCase(records []studypilot.ScheduleRecord, caseID string) (studypilot.ScheduleRecord, bool) {
	for _, record := range records {
		if record.CaseID == caseID {
			return record, true
		}
	}
	return studypilot.ScheduleRecord{}, false
}

func createAuthorizationDirectory(directory string, files map[string][]byte) error {
	if directory == "" {
		return fmt.Errorf("output directory is required")
	}
	if err := os.Mkdir(directory, 0o700); err != nil {
		return fmt.Errorf("create fresh authorization directory: %w", err)
	}
	for name, data := range files {
		path := filepath.Join(directory, name)
		file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
		if err != nil {
			return fmt.Errorf("create %s: %w", name, err)
		}
		if _, err := file.Write(data); err != nil {
			return fmt.Errorf("write %s: %w", name, errors.Join(err, file.Close()))
		}
		if err := file.Sync(); err != nil {
			return fmt.Errorf("sync %s: %w", name, errors.Join(err, file.Close()))
		}
		if err := file.Close(); err != nil {
			return err
		}
		if err := os.Chmod(path, 0o400); err != nil {
			return err
		}
	}
	return nil
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
