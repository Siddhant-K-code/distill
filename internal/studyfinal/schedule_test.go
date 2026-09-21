package studyfinal

import (
	"bytes"
	"crypto/ed25519"
	"sort"
	"testing"
	"time"
)

func TestScheduleExactCountsAndStrata(t *testing.T) {
	d := testCorpus(t)
	schedule, err := BuildSchedule(d, "schedule-test")
	if err != nil {
		t.Fatal(err)
	}
	if len(schedule) != 420 {
		t.Fatalf("got %d calls", len(schedule))
	}
	replicateOne, extras := 0, 0
	strata := map[string]int{}
	seenRepeat := map[string]bool{}
	for _, entry := range schedule {
		if entry.Replicate == 1 {
			replicateOne++
		} else {
			extras++
		}
		if entry.Repeatability {
			key := entry.ConditionID + "\x00" + entry.Arm
			if !seenRepeat[key] {
				seenRepeat[key] = true
				strata[entry.RepositoryRole+"\x00"+entry.Arm]++
			}
		}
	}
	if replicateOne != 300 || extras != 120 || len(seenRepeat) != 60 {
		t.Fatalf("wrong call allocation: r1=%d extras=%d repeated=%d", replicateOne, extras, len(seenRepeat))
	}
	if len(strata) != 6 {
		t.Fatalf("expected 6 strata: %#v", strata)
	}
	want := map[string]int{
		"threshold_development\x00" + ArmRaw: 15, "threshold_development\x00" + ArmDistillLock: 15,
		"safety_calibration\x00" + ArmRaw: 8, "safety_calibration\x00" + ArmDistillLock: 8,
		"confirmatory_secondary\x00" + ArmRaw: 7, "confirmatory_secondary\x00" + ArmDistillLock: 7,
	}
	for stratum, count := range strata {
		if count != want[stratum] {
			t.Fatalf("%s has %d repeats, want %d", stratum, count, want[stratum])
		}
	}
	again, err := BuildSchedule(d, "schedule-test")
	if err != nil {
		t.Fatal(err)
	}
	a, _ := DigestDomain("run-schedule", schedule)
	b, _ := DigestDomain("run-schedule", again)
	if a != b {
		t.Fatal("schedule digest is not deterministic")
	}
}

func TestPhaseOrderingAndHeldOutOnce(t *testing.T) {
	d := testCorpus(t)
	schedule, err := BuildSchedule(d, "phase-test")
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now().UTC()
	auth, attestation, env, analysis := testAuthorization(t, d, schedule, now)
	ledger, err := openAuthorizedExecutionLedger(auth, env)
	if err != nil {
		t.Fatal(err)
	}
	runner, err := newRunner(&fakeTransport{}, d, schedule, auth, attestation, analysis, env, now)
	if err != nil {
		t.Fatal(err)
	}
	g, err := runner.NewPhaseGate()
	if err != nil {
		t.Fatal(err)
	}
	labelAnswers := map[string]string{
		"evidence_complete": "yes", "observed_tests_support": "yes", "verifier_support": "yes",
		"cleanup_complete": "yes", "external_effects_resolved": "yes", "patch_risk": "low",
		"recommended_disposition": "accept",
	}
	groundTruth := make([]GroundTruthRecord, 0, len(d.Conditions))
	for _, condition := range d.Conditions {
		groundTruth = append(groundTruth, GroundTruthRecord{
			ConditionID: condition.ID, Answers: labelAnswers, Disposition: "accept",
			EvidenceSHA256: condition.SourceDigests,
		})
	}
	custodyRandom := bytes.Repeat([]byte{0x31}, 32+44*len(groundTruth))
	for i := range groundTruth {
		custodyRandom[32+i*44] = byte(i)
		custodyRandom[32+i*44+32] = byte(i)
	}
	publicCustody, vault, err := SealLabels(groundTruth, "independent-custodian",
		bytes.NewReader(custodyRandom), time.Unix(1, 0))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := vault.Release(g, publicCustody, nil, nil); err == nil {
		t.Fatal("labels released before receipt manifest completion")
	}
	held := ScheduleEntry{Split: "confirmatory_secondary"}
	if err := g.AuthorizeEntry(held); err == nil {
		t.Fatal("held-out early access allowed")
	}
	if err := g.BeginCalibration(); err == nil {
		t.Fatal("calibration before selection allowed")
	}
	receipts := make([]Receipt, 0, len(schedule))
	for _, entry := range schedule {
		if entry.Split == "threshold-development" {
			receipt := notAttemptedReceipt(t, d, entry, auth)
			if err := g.recordReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			receipts = append(receipts, receipt)
		}
	}
	score := .9
	development := []ScoredDecision{}
	calibration := []ScoredDecision{}
	for _, base := range d.Bases {
		switch base.Split {
		case "threshold-development":
			development = append(development, ScoredDecision{ID: base.ID, Score: &score})
		case "safety-calibration":
			calibration = append(calibration, ScoredDecision{ID: base.ID, Score: &score})
		}
	}
	if _, err := g.SelectAutomatically(map[string][]ScoredDecision{ArmRaw: development, ArmDistillLock: development}); err != nil {
		t.Fatal(err)
	}
	if err := g.BeginConfirmatorySecondary(); err == nil {
		t.Fatal("secondary evaluation before calibration allowed")
	}
	if err := g.BeginCalibration(); err != nil {
		t.Fatal(err)
	}
	for _, entry := range schedule {
		if entry.Split == "safety-calibration" {
			receipt := notAttemptedReceipt(t, d, entry, auth)
			if err := g.recordReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			receipts = append(receipts, receipt)
		}
	}
	if _, err := g.QualifyAutomatically(map[string][]ScoredDecision{ArmRaw: calibration, ArmDistillLock: calibration}); err != nil {
		t.Fatal(err)
	}
	if err := g.BeginHeldOut(); err == nil {
		t.Fatal("downgraded AgentTrace entered held-out phase")
	}
	if err := g.BeginConfirmatorySecondary(); err != nil {
		t.Fatal(err)
	}
	if err := g.BeginConfirmatorySecondary(); err == nil {
		t.Fatal("secondary evaluation opened twice")
	}
	if err := g.CompleteConfirmatorySecondary(nil, nil); err == nil {
		t.Fatal("secondary evaluation completed without receipts")
	}
	for _, entry := range schedule {
		if entry.Split == "confirmatory_secondary" {
			receipt := notAttemptedReceipt(t, d, entry, auth)
			if err := g.recordReceipt(receipt); err != nil {
				t.Fatal(err)
			}
			receipts = append(receipts, receipt)
		}
	}
	sort.Slice(receipts, func(i, j int) bool { return receipts[i].ScheduleIndex < receipts[j].ScheduleIndex })
	registry, _ := BuildReceiptRegistry(d, schedule)
	collector := testCollectorPrivateKey()
	manifest, err := BuildSignedReceiptManifest(d, schedule, registry, runner.session, receipts, ledger, "independent-collector", collector)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := BuildSignedReceiptManifest(d, schedule, registry, runner.session, receipts[1:], ledger, "independent-collector", collector); err == nil {
		t.Fatal("incomplete receipt manifest signed")
	}
	var forgedManifest ReceiptManifest
	if err := strictDecode(bytes.NewReader(manifest), &forgedManifest); err != nil {
		t.Fatal(err)
	}
	forgedManifest.Receipts[0].ErrorCode = ptr("forged")
	forgedManifest.Receipts[0].ReceiptSHA256, _ = receiptDigest(forgedManifest.Receipts[0])
	forgedManifest.StatementSHA256, _ = receiptManifestStatement(forgedManifest)
	forgedManifest.ManifestSHA256, _ = receiptManifestDigest(forgedManifest)
	forgedBytes, _ := canonicalJSON(forgedManifest)
	if _, err := ValidateReceiptManifest(forgedBytes, d, schedule, registry, runner.session, ledger, collector.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("rehashed receipt manifest passed independent signature")
	}
	if err := strictDecode(bytes.NewReader(manifest), &forgedManifest); err != nil {
		t.Fatal(err)
	}
	forgedManifest.CollectorID = "substituted-collector"
	forgedManifest.StatementSHA256, _ = receiptManifestStatement(forgedManifest)
	forgedManifest.ManifestSHA256, _ = receiptManifestDigest(forgedManifest)
	forgedBytes, _ = canonicalJSON(forgedManifest)
	if _, err := ValidateReceiptManifest(forgedBytes, d, schedule, registry, runner.session, ledger, collector.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("rehashed collector substitution passed signature validation")
	}
	if err := g.CompleteConfirmatorySecondary(manifest, collector.Public().(ed25519.PublicKey)); err != nil {
		t.Fatal(err)
	}
	labels, err := vault.Release(g, publicCustody, manifest, collector.Public().(ed25519.PublicKey))
	if err != nil || len(labels) != len(d.Conditions) {
		t.Fatalf("authorized label release failed: labels=%d err=%v", len(labels), err)
	}
	if _, err := vault.Release(g, publicCustody, manifest, collector.Public().(ed25519.PublicKey)); err == nil {
		t.Fatal("labels released twice")
	}
	if err := g.AuthorizeEntry(held); err == nil {
		t.Fatal("held-out accessible after completion")
	}
}

func notAttemptedReceipt(t *testing.T, d Dataset, entry ScheduleEntry, auth Authorization) Receipt {
	t.Helper()
	condition := conditionByID(t, d, entry.ConditionID)
	request, err := buildRequest(entry, condition)
	if err != nil {
		t.Fatal(err)
	}
	requestBytes, _ := canonicalJSON(request)
	code := "not_attempted"
	message := digestBytes([]byte("not attempted"))
	rawError, _ := canonicalJSON(ErrorArtifact{SchemaVersion: SchemaVersion + "/error", Code: code, MessageSHA256: message})
	errorArtifactHash := digestBytes(rawError)
	at := time.Unix(1, 0).UTC()
	receipt := Receipt{
		SchemaVersion: SchemaVersion + "/receipt", CallID: entry.CallID, ScheduleIndex: entry.Index,
		ConditionID: entry.ConditionID, RepositoryRole: entry.RepositoryRole, Split: entry.Split,
		Arm: entry.Arm, Replicate: entry.Replicate, Status: "not_attempted", Model: JevModel,
		AuthorizationSHA256: auth.AuthorizationSHA256, RequestSHA256: digestBytes(requestBytes),
		ErrorCode: &code, ErrorMessageSHA256: &message, RawError: rawError,
		ErrorArtifactSHA256: &errorArtifactHash, StartedAt: at, FinishedAt: at,
	}
	receipt.ReceiptSHA256, _ = receiptDigest(receipt)
	return receipt
}
