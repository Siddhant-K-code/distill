package studyfinal

import (
	"bytes"
	"context"
	"crypto/ed25519"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type fakeTransport struct {
	mu       sync.Mutex
	calls    int
	response TransportResponse
	err      error
	keySeen  string
}

type witnessState struct {
	genesis string
	last    string
	records int
}

type memoryLedgerWitness struct {
	mu     sync.Mutex
	states map[string]witnessState
}

func (w *memoryLedgerWitness) Register(binding LedgerBinding) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.states == nil {
		w.states = map[string]witnessState{}
	}
	if _, exists := w.states[binding.LedgerID]; exists {
		return fmt.Errorf("ledger ID already registered")
	}
	w.states[binding.LedgerID] = witnessState{genesis: binding.LedgerGenesisSHA256, last: binding.LedgerGenesisSHA256}
	return nil
}

func (w *memoryLedgerWitness) Verify(id, genesis, last string, records int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	state, ok := w.states[id]
	if !ok || state.genesis != genesis || state.last != last || state.records != records {
		return fmt.Errorf("ledger witness checkpoint mismatch")
	}
	return nil
}

func (w *memoryLedgerWitness) Advance(id, previous, next string, records int) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	state, ok := w.states[id]
	if !ok || state.last != previous || records != state.records+1 {
		return fmt.Errorf("ledger witness compare-and-append failed")
	}
	state.last, state.records = next, records
	w.states[id] = state
	return nil
}

func (f *fakeTransport) Do(_ context.Context, key string, _ Request) (TransportResponse, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.keySeen = key
	return f.response, f.err
}

func testAuthorization(t *testing.T, d Dataset, schedule []ScheduleEntry, now time.Time) (Authorization, GateAttestation, authorizationEnvironment, string) {
	t.Helper()
	analysis := digestBytes([]byte("frozen-analysis"))
	scheduleHash, _ := DigestDomain("run-schedule", schedule)
	env, signers := testReviewEnvironment()
	attestation := GateAttestation{
		SchemaVersion: SchemaVersion + "/gate-attestation", Status: "passed", ExactHeadCommit: DistillCommit,
		Provider: "TypeSafe AI", AccountIDHash: digestBytes([]byte("test-account")),
		RetentionPolicySHA256:    digestBytes([]byte("test-retention")),
		CollectorPublicKeySHA256: digestBytes(testCollectorPrivateKey().Public().(ed25519.PublicKey)), Model: JevModel,
		ProviderCapNanoUSD: TotalCapNanoUSD, CorpusSHA256: d.CorpusSHA256, SplitSHA256: d.SplitSHA256,
		SourceRegistrySHA256: d.SourceRegistrySHA256,
		ScheduleSHA256:       scheduleHash, AnalysisSHA256: analysis, TokenBound: 1000, CallCount: len(schedule),
		ExpiresAt:                     now.Add(time.Hour).UTC(),
		AgentTraceContaminationSHA256: d.AgentTraceContamination.EvidenceSHA256,
	}
	for _, name := range requiredExternalGates {
		attestation.Gates = append(attestation.Gates, GateResult{Name: name, Status: "passed", EvidenceSHA256: digestBytes([]byte("evidence-" + name))})
	}
	attestation.TrustRootVersion = env.trustVersion
	attestation.TrustRootSHA256, _ = trustedReviewersDigest(env.trusted)
	root, err := os.MkdirTemp(".", ".studyfinal-auth-ledger-")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.RemoveAll(root) })
	binding, err := createExecutionLedger(filepath.Join(root, "execution.jsonl"), d, schedule, analysis, 1000,
		bytes.NewReader(bytes.Repeat([]byte{0x19}, 32)), now, env)
	if err != nil {
		t.Fatal(err)
	}
	attestation.LedgerID, attestation.LedgerPathSHA256, attestation.LedgerGenesisSHA256 =
		binding.LedgerID, binding.LedgerPathSHA256, binding.LedgerGenesisSHA256
	attestation, err = FinalizeGateAttestation(attestation, signers)
	if err != nil {
		t.Fatal(err)
	}
	auth, err := newAuthorization(d, schedule, analysis, 1000, attestation.ExpiresAt, attestation, binding, env, now)
	if err != nil {
		t.Fatal(err)
	}
	return auth, attestation, env, analysis
}

func testReviewEnvironment() (authorizationEnvironment, map[string]ed25519.PrivateKey) {
	signers := map[string]ed25519.PrivateKey{}
	trusted := TrustedReviewers{}
	for i, review := range requiredReviews() {
		seed := bytes.Repeat([]byte{byte(i + 1)}, ed25519.SeedSize)
		key := ed25519.NewKeyFromSeed(seed)
		signers[review] = key
		trusted[review] = key.Public().(ed25519.PublicKey)
	}
	env := authorizationEnvironment{
		trustVersion: "test-only-trust-root-v1", trusted: trusted,
		build:   func() buildIdentity { return buildIdentity{Revision: DistillCommit, Modified: false, Valid: true} },
		witness: &memoryLedgerWitness{},
	}
	env.trustDigest, _ = trustedReviewersDigest(trusted)
	return env, signers
}

func testCollectorPrivateKey() ed25519.PrivateKey {
	return ed25519.NewKeyFromSeed(bytes.Repeat([]byte{0x42}, ed25519.SeedSize))
}

func providerSuccess(t *testing.T, decision Decision, inputTokens int64, requestID string) TransportResponse {
	t.Helper()
	output := int64(0)
	body, err := canonicalJSON(ProviderResponse{
		Model: JevModel, Decision: decision,
		Usage:             Usage{InputTokens: &inputTokens, OutputTokens: &output},
		ProviderRequestID: requestID,
	})
	if err != nil {
		t.Fatal(err)
	}
	return TransportResponse{RawBody: body, HTTPStatus: 200, ProviderRequestID: &requestID}
}

func newTestRunner(t *testing.T, transport Transport, seed string) (Dataset, []ScheduleEntry, Authorization, GateAttestation, authorizationEnvironment, string, *Runner, *PhaseGate) {
	t.Helper()
	d := testCorpus(t)
	schedule, err := BuildSchedule(d, seed)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	auth, attestation, env, analysis := testAuthorization(t, d, schedule, now)
	runner, err := newRunner(transport, d, schedule, auth, attestation, analysis, env, now)
	if err != nil {
		t.Fatal(err)
	}
	gate, err := runner.NewPhaseGate()
	if err != nil {
		t.Fatal(err)
	}
	return d, schedule, auth, attestation, env, analysis, runner, gate
}

func TestRunnerZeroRetriesBudgetAndReceipt(t *testing.T) {
	providerID := "fake-request"
	transport := &fakeTransport{}
	d, schedule, auth, _, _, _, runner, gate := newTestRunner(t, transport, "runner-test")
	transport.response = providerSuccess(t, safeJevDecision(), 100, providerID)
	entry := schedule[0]
	receipt := runner.RunOne(context.Background(), "memory-only-key", gate, entry.CallID)
	if transport.calls != 1 || transport.keySeen != "memory-only-key" {
		t.Fatal("transport retry/key handling failure")
	}
	if receipt.Status != "succeeded" || receipt.InferredCostNanoUSD == nil || *receipt.InferredCostNanoUSD != 4200 {
		t.Fatalf("receipt=%#v", receipt)
	}
	if err := ValidateReceipt(receipt, d, entry, auth); err != nil {
		t.Fatal(err)
	}
	encoded, _ := canonicalJSON(receipt)
	if bytes.Contains(encoded, []byte("memory-only-key")) {
		t.Fatal("API key serialized into receipt")
	}

	transport.err = errors.New("offline failure")
	receipt = runner.RunOne(context.Background(), "memory-only-key", gate, schedule[1].CallID)
	if transport.calls != 2 || receipt.Status != "partial" || receipt.ResponseSHA256 == nil {
		t.Fatal("partial failure was retried or not preserved")
	}
	transport.err = errors.New("transport leaked memory-only-key")
	receipt = runner.RunOne(context.Background(), "memory-only-key", gate, schedule[2].CallID)
	if receipt.ErrorMessageSHA256 == nil || *receipt.ErrorMessageSHA256 == digestBytes([]byte(transport.err.Error())) {
		t.Fatal("API key was hashed through an error message")
	}
}

func TestProviderRawBytesAndParsedDigest(t *testing.T) {
	providerID := "raw-byte-request"
	decision := safeJevDecision()
	input, output := int64(17), int64(0)
	reordered := struct {
		Usage                       Usage    `json:"usage"`
		ProviderRequestID           string   `json:"provider_request_id"`
		Decision                    Decision `json:"decision"`
		ProviderReportedCostNanoUSD *int64   `json:"provider_reported_cost_nano_usd"`
		Model                       string   `json:"model"`
	}{
		Usage: Usage{InputTokens: &input, OutputTokens: &output}, ProviderRequestID: providerID,
		Decision: decision, Model: JevModel,
	}
	envelope := map[string]any{
		"usage": reordered.Usage, "provider_request_id": reordered.ProviderRequestID,
		"decision": reordered.Decision, "provider_reported_cost_nano_usd": reordered.ProviderReportedCostNanoUSD,
		"model": reordered.Model, "provider_metadata": map[string]any{"region": "test", "sequence": 1},
	}
	raw, err := json.MarshalIndent(envelope, "  ", "    ")
	if err != nil {
		t.Fatal(err)
	}
	raw = append(raw, '\n')
	transport := &fakeTransport{response: TransportResponse{RawBody: raw, HTTPStatus: 200, ProviderRequestID: &providerID}}
	d, schedule, auth, _, _, _, runner, gate := newTestRunner(t, transport, "raw-byte-test")
	receipt := runner.RunOne(context.Background(), "key", gate, schedule[0].CallID)
	if receipt.Status != "succeeded" || !bytes.Equal(receipt.RawResponse, raw) ||
		receipt.ResponseSHA256 == nil || *receipt.ResponseSHA256 != digestBytes(raw) ||
		receipt.ParsedResponseSHA256 == nil {
		t.Fatalf("raw/parsed response binding failed: %#v", receipt)
	}
	var parsed ProviderResponse
	if parsed, err = decodeProviderResponse(raw); err != nil {
		t.Fatal(err)
	}
	wantParsed, _ := DigestDomain("provider-response", parsed)
	if *receipt.ParsedResponseSHA256 != wantParsed {
		t.Fatal("parsed response digest changed with whitespace/member order")
	}
	if err := ValidateReceipt(receipt, d, schedule[0], auth); err != nil {
		t.Fatal(err)
	}
	duplicate := bytes.Replace(raw, []byte(`"model": "jev-1.13.0"`), []byte(`"model": "jev-1.13.0", "model": "jev-1.13.0"`), 1)
	if _, err := decodeProviderResponse(duplicate); err == nil {
		t.Fatal("duplicate registered provider field was accepted")
	}
	conflicting := bytes.Replace(raw, []byte(`"model": "jev-1.13.0"`), []byte(`"model": "jev-1.13.0", "model": "jev-latest"`), 1)
	if _, err := decodeProviderResponse(conflicting); err == nil {
		t.Fatal("conflicting registered provider field was accepted")
	}
}

func TestRunOneSurfacesReceiptRecordingFailure(t *testing.T) {
	providerID := "recording-error"
	transport := &fakeTransport{}
	d, schedule, auth, _, _, _, runner, gate := newTestRunner(t, transport, "recording-error-test")
	transport.response = providerSuccess(t, safeJevDecision(), 1, providerID)
	gate.receipts[schedule[0].CallID] = Receipt{}
	receipt := runner.RunOne(context.Background(), "key", gate, schedule[0].CallID)
	if receipt.Status != "partial" || receipt.ErrorCode == nil || *receipt.ErrorCode != "internal_recording" ||
		receipt.LedgerAttemptSHA256 == "" || receipt.LedgerSettlementSHA256 == "" || gate.fatalErr == nil {
		t.Fatalf("recording error was not surfaced and made fatal: %#v", receipt)
	}
	if err := ValidateReceipt(receipt, d, schedule[0], auth); err != nil {
		t.Fatalf("terminal recording-error receipt is invalid: %v", err)
	}
}

func TestAttestationAndAuthorizationBindSourceRegistry(t *testing.T) {
	d := testCorpus(t)
	schedule, err := BuildSchedule(d, "registry-binding-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	auth, attestation, env, analysis := testAuthorization(t, d, schedule, now)
	validAttestation := attestation
	_, signers := testReviewEnvironment()
	attestation.SourceRegistrySHA256 = strings.Repeat("f", 64)
	attestation, err = FinalizeGateAttestation(attestation, signers)
	if err != nil {
		t.Fatal(err)
	}
	if err := validateGateAttestation(attestation, env, d, schedule, analysis, auth.TokenBound, now); err == nil {
		t.Fatal("re-signed source registry attestation substitution accepted")
	}
	auth.SourceRegistrySHA256 = strings.Repeat("e", 64)
	auth.AuthorizationSHA256, _ = authorizationDigest(auth)
	if err := validateAuthorization(auth, d, schedule, analysis, validAttestation, env, now); err == nil {
		t.Fatal("rehashed authorization source registry substitution accepted")
	}
}

func TestProviderDriftAndMalformedProbability(t *testing.T) {
	decision := safeJevDecision()
	transport := &fakeTransport{}
	_, schedule, _, _, _, _, runner, gate := newTestRunner(t, transport, "drift-test")
	transport.response = providerSuccess(t, decision, 1, "drift-request")
	var drift ProviderResponse
	if err := strictDecode(bytes.NewReader(transport.response.RawBody), &drift); err != nil {
		t.Fatal(err)
	}
	drift.Model = "jev-latest"
	transport.response.RawBody, _ = canonicalJSON(drift)
	r := runner.RunOne(context.Background(), "key", gate, schedule[0].CallID)
	if transport.calls != 1 || r.ErrorCode == nil || *r.ErrorCode != "provider_model_drift" {
		t.Fatalf("drift receipt %#v", r)
	}

	decision = safeJevDecision()
	delete(decision.Answers["patch_risk"].Probabilities, "unknown")
	transport.response = providerSuccess(t, decision, 1, "malformed-request")
	transport.err = nil
	transport.calls = 0
	r = runner.RunOne(context.Background(), "key", gate, schedule[1].CallID)
	if transport.calls != 1 || r.ErrorCode == nil || *r.ErrorCode != "malformed_decision" {
		t.Fatalf("malformed receipt %#v", r)
	}
	snapshot, err := runner.ledger.Snapshot()
	if err != nil || snapshot.SpentNanoUSD != 2*InputPriceNanoUSD {
		t.Fatalf("authoritative failure usage was not persisted: %#v err=%v", snapshot, err)
	}
}

func TestUsageMissingNullAndMeasuredZeroAreDistinct(t *testing.T) {
	var missing, nullValue, measured Usage
	if err := json.Unmarshal([]byte(`{}`), &missing); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"input_tokens":null}`), &nullValue); err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal([]byte(`{"input_tokens":0}`), &measured); err != nil {
		t.Fatal(err)
	}
	if missing.InputState() != "missing" || nullValue.InputState() != "null" ||
		measured.InputState() != "measured" || measured.InputTokens == nil || *measured.InputTokens != 0 {
		t.Fatalf("states collapsed: missing=%s null=%s measured=%s", missing.InputState(), nullValue.InputState(), measured.InputState())
	}
	encoded, err := json.Marshal(missing)
	if err != nil || string(encoded) != "{}" {
		t.Fatalf("missing did not round-trip: %s, %v", encoded, err)
	}
	encoded, _ = json.Marshal(nullValue)
	if string(encoded) != `{"input_tokens":null}` {
		t.Fatalf("null did not round-trip: %s", encoded)
	}
}

func TestAuthorizationAndPriorSpendReserve(t *testing.T) {
	unresolved, err := BuildCorpus(Config{LLMTraceFXCommit: OfflineUnresolved, Seed: "unresolved"})
	if err != nil {
		t.Fatal(err)
	}
	unresolvedSchedule, _ := BuildSchedule(unresolved, "unresolved-auth")
	now := time.Now().UTC()
	noGo := GateAttestation{SchemaVersion: SchemaVersion + "/gate-attestation", Status: "denied", DenyReason: "current external record is NO-GO"}
	if _, err := NewAuthorization(unresolved, unresolvedSchedule, digestBytes([]byte("analysis")), DefaultMaxInputToken, now.Add(time.Hour), noGo, LedgerBinding{}, now); err == nil {
		t.Fatal("unresolved source registry authorized")
	}
	d, err := BuildCorpus(Config{LLMTraceFXCommit: LLMTraceFXCommit, Seed: "auth"})
	if err != nil {
		t.Fatal(err)
	}
	schedule, err := BuildSchedule(d, "auth-test")
	if err != nil {
		t.Fatal(err)
	}
	auth, attestation, env, analysisHash := testAuthorization(t, d, schedule, now)
	if auth.PriorSpendNanoUSD != 760242 || auth.RemainingCapNanoUSD != 4999239758 {
		t.Fatal("budget constants drift")
	}
	if err := validateAuthorization(auth, d, schedule, analysisHash, attestation, env, attestation.ExpiresAt.Add(-time.Minute)); err != nil {
		t.Fatal(err)
	}
	if err := validateAuthorization(auth, d, schedule, "other", attestation, env, attestation.ExpiresAt.Add(-time.Minute)); err == nil {
		t.Fatal("analysis drift accepted")
	}
}

func TestRandomLabelCommitmentCustody(t *testing.T) {
	answers := map[string]string{
		"evidence_complete": "yes", "observed_tests_support": "yes", "verifier_support": "yes",
		"cleanup_complete": "yes", "external_effects_resolved": "yes", "patch_risk": "low",
		"recommended_disposition": "accept",
	}

	labels := []GroundTruthRecord{{ConditionID: "c1", Answers: answers, Disposition: "accept", EvidenceSHA256: []string{digestBytes([]byte("evidence"))}}}
	random := bytes.NewReader(bytes.Repeat([]byte{0x5a}, 76))
	custody, vault, err := SealLabels(labels, "independent-custodian", random, time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	publicBytes, _ := canonicalJSON(custody)
	if bytes.Contains(publicBytes, []byte(`"answers"`)) || bytes.Contains(publicBytes, []byte(`"nonce"`)) ||
		bytes.Contains(publicBytes, []byte(`"disposition"`)) {
		t.Fatal("public custody package leaks labels or nonce")
	}
	vaultBytes, _ := json.Marshal(vault)
	if string(vaultBytes) != "{}" {
		t.Fatal("custodian vault serialized secret material")
	}
}

func TestCommitmentRegistryBijection(t *testing.T) {
	d := testCorpus(t)
	labels := make([]GroundTruthRecord, 0, len(d.Conditions))
	for _, condition := range d.Conditions {
		answers := map[string]string{
			"evidence_complete": "yes", "observed_tests_support": "yes", "verifier_support": "yes",
			"cleanup_complete": "yes", "external_effects_resolved": "yes", "patch_risk": "low",
			"recommended_disposition": "accept",
		}
		labels = append(labels, GroundTruthRecord{
			ConditionID: condition.ID, Answers: answers, Disposition: "accept",
			EvidenceSHA256: append([]string(nil), condition.SourceDigests...),
		})
	}
	randomBytes := bytes.Repeat([]byte{0x73}, 32+44*len(labels))
	for i := range labels {
		randomBytes[32+i*44] = byte(i)
		randomBytes[32+i*44+32] = byte(i)
	}
	custody, _, err := SealLabels(labels, "independent-custodian", bytes.NewReader(randomBytes), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateCommitmentsAgainstCorpus(d, custody.Commitments); err != nil {
		t.Fatal(err)
	}
	custody.Commitments = custody.Commitments[1:]
	if err := ValidateCommitmentsAgainstCorpus(d, custody.Commitments); err == nil {
		t.Fatal("missing commitment accepted")
	}
}

func TestAtomicOwnerOnlyPackageAndBijection(t *testing.T) {
	root, err := os.MkdirTemp(".", ".studyfinal-local-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	if err := os.Chmod(root, 0o700); err != nil {
		t.Fatal(err)
	}
	d := testCorpus(t)
	schedule, _ := BuildSchedule(d, "package-test")
	destination := filepath.Join(root, "published")
	if err := PreparePackage(destination, d, schedule); err != nil {
		t.Fatal(err)
	}
	if err := PreparePackage(destination, d, schedule); err == nil {
		t.Fatal("package replaced")
	}
	if _, _, err := ValidatePackage(destination); err != nil {
		t.Fatal(err)
	}
	info, _ := os.Stat(destination)
	if info.Mode().Perm() != 0o700 {
		t.Fatalf("directory mode %o", info.Mode().Perm())
	}
	for _, name := range packageFiles {
		info, _ = os.Stat(filepath.Join(destination, name))
		if info.Mode().Perm() != 0o600 {
			t.Fatalf("%s mode %o", name, info.Mode().Perm())
		}
	}
	templatePath := filepath.Join(destination, "authorization-template.json")
	templateBytes, err := os.ReadFile(templatePath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(templatePath, append(templateBytes, ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidatePackage(destination); err == nil {
		t.Fatal("authorization template tampering accepted")
	}
	if err := os.WriteFile(templatePath, templateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	var template AuthorizationTemplate
	if err := strictDecode(bytes.NewReader(templateBytes), &template); err != nil {
		t.Fatal(err)
	}
	template.RemainingCapNanoUSD++
	tamperedTemplate, _ := json.MarshalIndent(template, "", "  ")
	if err := os.WriteFile(templatePath, append(tamperedTemplate, '\n'), 0o600); err != nil {
		t.Fatal(err)
	}
	var rewritten strings.Builder
	for _, name := range packageFiles[:len(packageFiles)-1] {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&rewritten, "%s  %s\n", digestBytes(data), name)
	}
	if err := os.WriteFile(filepath.Join(destination, "SHA256SUMS"), []byte(rewritten.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidatePackage(destination); err == nil {
		t.Fatal("self-consistent authorization-template tampering accepted")
	}
	if err := os.WriteFile(templatePath, templateBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	rewritten.Reset()
	for _, name := range packageFiles[:len(packageFiles)-1] {
		data, err := os.ReadFile(filepath.Join(destination, name))
		if err != nil {
			t.Fatal(err)
		}
		fmt.Fprintf(&rewritten, "%s  %s\n", digestBytes(data), name)
	}
	if err := os.WriteFile(filepath.Join(destination, "SHA256SUMS"), []byte(rewritten.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidatePackage(destination); err != nil {
		t.Fatal(err)
	}
	registryPath := filepath.Join(destination, "source-registry.json")
	registryBytes, err := os.ReadFile(registryPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(registryPath, append(append([]byte(nil), registryBytes...), ' '), 0o600); err != nil {
		t.Fatal(err)
	}
	rewritten.Reset()
	for _, name := range packageFiles[:len(packageFiles)-1] {
		data, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		fmt.Fprintf(&rewritten, "%s  %s\n", digestBytes(data), name)
	}
	if err := os.WriteFile(filepath.Join(destination, "SHA256SUMS"), []byte(rewritten.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidatePackage(destination); err == nil {
		t.Fatal("rehashed source registry package tampering accepted")
	}
	if err := os.WriteFile(registryPath, registryBytes, 0o600); err != nil {
		t.Fatal(err)
	}
	rewritten.Reset()
	for _, name := range packageFiles[:len(packageFiles)-1] {
		data, readErr := os.ReadFile(filepath.Join(destination, name))
		if readErr != nil {
			t.Fatal(readErr)
		}
		fmt.Fprintf(&rewritten, "%s  %s\n", digestBytes(data), name)
	}
	if err := os.WriteFile(filepath.Join(destination, "SHA256SUMS"), []byte(rewritten.String()), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(destination, "unexpected"), []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, _, err := ValidatePackage(destination); err == nil {
		t.Fatal("unexpected file accepted")
	}

	receipts := make([]Receipt, len(schedule))
	now := time.Now().UTC()
	auth, attestation, env, analysis := testAuthorization(t, d, schedule, now)
	session, err := newAuthorizedSession(auth, d, schedule, analysis, attestation, env, now)
	if err != nil {
		t.Fatal(err)
	}
	for i, entry := range schedule {
		receipts[i] = notAttemptedReceipt(t, d, entry, auth)
	}
	ledger, err := openAuthorizedExecutionLedger(auth, env)
	if err != nil {
		t.Fatal(err)
	}
	registry, err := BuildReceiptRegistry(d, schedule)
	if err != nil {
		t.Fatal(err)
	}
	collector := testCollectorPrivateKey()
	manifest, err := BuildSignedReceiptManifest(d, schedule, registry, session, receipts, ledger, "test-collector", collector)
	if err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptBijection(d, schedule, receipts, session, ledger, manifest, collector.Public().(ed25519.PublicKey)); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptRegistry(registry, d, schedule); err != nil {
		t.Fatal(err)
	}
	if err := ValidateReceiptWithRegistry(receipts[0], registry, d, schedule, session, ledger, manifest, collector.Public().(ed25519.PublicKey)); err != nil {
		t.Fatal(err)
	}
	registry.Bindings[0].CallID = "outside"
	if err := ValidateReceiptRegistry(registry, d, schedule); err == nil {
		t.Fatal("modified closed receipt registry accepted")
	}
	receipts = receipts[1:]
	if err := ValidateReceiptBijection(d, schedule, receipts, session, ledger, manifest, collector.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("missing receipt accepted")
	}
}

func TestPackageRejectsSymlink(t *testing.T) {
	root, err := os.MkdirTemp(".", ".studyfinal-symlink-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(root)
	real := filepath.Join(root, "real")
	if err := os.Mkdir(real, 0o700); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(root, "link")
	if err := os.Symlink(real, link); err != nil {
		t.Skipf("symlink unavailable: %v", err)
	}
	d := testCorpus(t)
	schedule, _ := BuildSchedule(d, "symlink-test")
	if err := PreparePackage(filepath.Join(link, "package"), d, schedule); err == nil {
		t.Fatal("symlink destination accepted")
	}
}

func ptr[T any](v T) *T { return &v }

func conditionByID(t *testing.T, d Dataset, id string) Condition {
	t.Helper()
	for _, condition := range d.Conditions {
		if condition.ID == id {
			return condition
		}
	}
	t.Fatalf("missing condition %s", id)
	return Condition{}
}
