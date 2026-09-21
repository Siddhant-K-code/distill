package studyfinal

import (
	"bufio"
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"
)

type LedgerBinding struct {
	LedgerID            string `json:"ledger_id"`
	CanonicalPath       string `json:"canonical_path"`
	LedgerPathSHA256    string `json:"ledger_path_sha256"`
	LedgerGenesisSHA256 string `json:"ledger_genesis_sha256"`
}

type LedgerGenesis struct {
	SchemaVersion     string    `json:"schema_version"`
	LedgerID          string    `json:"ledger_id"`
	LedgerPathSHA256  string    `json:"ledger_path_sha256"`
	CorpusSHA256      string    `json:"corpus_sha256"`
	SplitSHA256       string    `json:"split_sha256"`
	ScheduleSHA256    string    `json:"schedule_sha256"`
	AnalysisSHA256    string    `json:"analysis_sha256"`
	TokenBound        int64     `json:"token_bound"`
	PriorSpendNanoUSD int64     `json:"prior_spend_nano_usd"`
	TotalCapNanoUSD   int64     `json:"total_cap_nano_usd"`
	CreatedAt         time.Time `json:"created_at"`
	GenesisSHA256     string    `json:"genesis_sha256"`
}

type LedgerRecord struct {
	SchemaVersion       string    `json:"schema_version"`
	Sequence            int       `json:"sequence"`
	AuthorizationSHA256 string    `json:"authorization_sha256"`
	CallID              string    `json:"call_id"`
	Event               string    `json:"event"`
	ReservedNanoUSD     int64     `json:"reserved_nano_usd"`
	ChargedNanoUSD      int64     `json:"charged_nano_usd"`
	CostBasis           string    `json:"cost_basis"`
	At                  time.Time `json:"at"`
	PreviousSHA256      string    `json:"previous_sha256"`
	RecordSHA256        string    `json:"record_sha256"`
}

type LedgerSnapshot struct {
	SpentNanoUSD       int64
	OutstandingNanoUSD int64
	Attempted          map[string]bool
	Outstanding        map[string]int64
	LastSHA256         string
	Records            int
	Entries            []LedgerRecord
}

type ledgerWitness interface {
	Register(LedgerBinding) error
	Verify(ledgerID, genesisSHA256, lastSHA256 string, records int) error
	Advance(ledgerID, previousSHA256, nextSHA256 string, records int) error
}

type ExecutionLedger struct {
	path    string
	auth    Authorization
	binding LedgerBinding
	witness ledgerWitness
}

func ledgerBindingFromAuthorization(auth Authorization) LedgerBinding {
	return LedgerBinding{
		LedgerID: auth.LedgerID, CanonicalPath: auth.LedgerCanonicalPath,
		LedgerPathSHA256: auth.LedgerPathSHA256, LedgerGenesisSHA256: auth.LedgerGenesisSHA256,
	}
}

func canonicalLedgerPath(path string) (string, error) {
	if !safeDestination(path) {
		return "", fmt.Errorf("unsafe execution ledger path")
	}
	parent, err := filepath.Abs(filepath.Dir(path))
	if err != nil {
		return "", err
	}
	parent, err = filepath.EvalSymlinks(parent)
	if err != nil {
		return "", err
	}
	info, err := os.Stat(parent)
	if err != nil || !info.IsDir() || info.Mode().Perm()&0o077 != 0 {
		return "", fmt.Errorf("execution ledger parent must be an owner-only directory")
	}
	if err := requireCurrentOwner(info); err != nil {
		return "", err
	}
	return filepath.Join(parent, filepath.Base(path)), nil
}

func ledgerGenesisDigest(genesis LedgerGenesis) (string, error) {
	genesis.GenesisSHA256 = ""
	return DigestDomain("ledger-genesis", genesis)
}

func createExecutionLedger(path string, d Dataset, schedule []ScheduleEntry, analysisHash string, tokenBound int64, randomSource io.Reader, now time.Time, env authorizationEnvironment) (LedgerBinding, error) {
	if env.witness == nil {
		return LedgerBinding{}, fmt.Errorf("no production append-only ledger witness is configured")
	}
	if err := validateSchedule(d, schedule, env); err != nil {
		return LedgerBinding{}, err
	}
	canonicalPath, err := canonicalLedgerPath(path)
	if err != nil {
		return LedgerBinding{}, err
	}
	if randomSource == nil {
		randomSource = rand.Reader
	}
	idBytes := make([]byte, 32)
	if _, err := io.ReadFull(randomSource, idBytes); err != nil {
		return LedgerBinding{}, err
	}
	scheduleSHA256, _ := DigestDomain("run-schedule", schedule)
	genesis := LedgerGenesis{
		SchemaVersion: SchemaVersion + "/execution-ledger-genesis", LedgerID: hex.EncodeToString(idBytes),
		LedgerPathSHA256: digestBytes([]byte(canonicalPath)), CorpusSHA256: d.CorpusSHA256,
		SplitSHA256: d.SplitSHA256, ScheduleSHA256: scheduleSHA256, AnalysisSHA256: analysisHash,
		TokenBound: tokenBound, PriorSpendNanoUSD: PriorSpendNanoUSD, TotalCapNanoUSD: TotalCapNanoUSD, CreatedAt: now.UTC(),
	}
	genesis.GenesisSHA256, _ = ledgerGenesisDigest(genesis)
	binding := LedgerBinding{
		LedgerID: genesis.LedgerID, CanonicalPath: canonicalPath,
		LedgerPathSHA256: genesis.LedgerPathSHA256, LedgerGenesisSHA256: genesis.GenesisSHA256,
	}
	file, err := os.OpenFile(canonicalPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return LedgerBinding{}, err
	}
	if err := env.witness.Register(binding); err != nil {
		_ = file.Close()
		_ = os.Remove(canonicalPath)
		return LedgerBinding{}, err
	}
	data, err := canonicalJSON(genesis)
	if err == nil {
		_, err = file.Write(append(data, '\n'))
	}
	if err == nil {
		err = file.Sync()
	}
	closeErr := file.Close()
	if err != nil {
		_ = os.Remove(canonicalPath)
		return LedgerBinding{}, err
	}
	if closeErr != nil {
		_ = os.Remove(canonicalPath)
		return LedgerBinding{}, closeErr
	}
	parent, err := os.Open(filepath.Dir(canonicalPath))
	if err != nil {
		return LedgerBinding{}, err
	}
	defer parent.Close()
	if err := parent.Sync(); err != nil {
		return LedgerBinding{}, err
	}
	return binding, nil
}

func CreateExecutionLedger(path string, d Dataset, schedule []ScheduleEntry, analysisHash string, tokenBound int64, now time.Time) (LedgerBinding, error) {
	return createExecutionLedger(path, d, schedule, analysisHash, tokenBound, nil, now, productionAuthorizationEnvironment())
}

func LedgerBindingFromAttestation(path string, attestation GateAttestation) (LedgerBinding, error) {
	canonicalPath, err := canonicalLedgerPath(path)
	if err != nil {
		return LedgerBinding{}, err
	}
	if digestBytes([]byte(canonicalPath)) != attestation.LedgerPathSHA256 {
		return LedgerBinding{}, fmt.Errorf("ledger path does not match attestation")
	}
	return LedgerBinding{
		LedgerID: attestation.LedgerID, CanonicalPath: canonicalPath,
		LedgerPathSHA256: attestation.LedgerPathSHA256, LedgerGenesisSHA256: attestation.LedgerGenesisSHA256,
	}, nil
}

func validateFreshLedgerBinding(binding LedgerBinding, d Dataset, schedule []ScheduleEntry, analysisHash string, tokenBound int64, env authorizationEnvironment) error {
	if env.witness == nil {
		return fmt.Errorf("no append-only ledger witness is configured")
	}
	canonicalPath, err := canonicalLedgerPath(binding.CanonicalPath)
	if err != nil {
		return err
	}
	if canonicalPath != binding.CanonicalPath || digestBytes([]byte(canonicalPath)) != binding.LedgerPathSHA256 {
		return fmt.Errorf("ledger canonical path binding mismatch")
	}
	file, err := os.Open(canonicalPath)
	if err != nil {
		return fmt.Errorf("bound ledger missing: %w", err)
	}
	defer file.Close()
	scanner := bufio.NewScanner(file)
	if !scanner.Scan() {
		return fmt.Errorf("ledger genesis missing")
	}
	var genesis LedgerGenesis
	if err := strictDecode(bytes.NewReader(scanner.Bytes()), &genesis); err != nil {
		return err
	}
	canonical, _ := canonicalJSON(genesis)
	digest, _ := ledgerGenesisDigest(genesis)
	scheduleSHA256, _ := DigestDomain("run-schedule", schedule)
	if !bytes.Equal(canonical, scanner.Bytes()) || scanner.Scan() || genesis.GenesisSHA256 != digest ||
		genesis.LedgerID != binding.LedgerID || genesis.GenesisSHA256 != binding.LedgerGenesisSHA256 ||
		genesis.LedgerPathSHA256 != binding.LedgerPathSHA256 || genesis.CorpusSHA256 != d.CorpusSHA256 ||
		genesis.SplitSHA256 != d.SplitSHA256 || genesis.ScheduleSHA256 != scheduleSHA256 ||
		genesis.AnalysisSHA256 != analysisHash || genesis.TokenBound != tokenBound ||
		genesis.PriorSpendNanoUSD != PriorSpendNanoUSD || genesis.TotalCapNanoUSD != TotalCapNanoUSD {
		return fmt.Errorf("ledger genesis is not fresh or does not match authorization inputs")
	}
	return env.witness.Verify(binding.LedgerID, binding.LedgerGenesisSHA256, binding.LedgerGenesisSHA256, 0)
}

func openAuthorizedExecutionLedger(auth Authorization, env authorizationEnvironment) (*ExecutionLedger, error) {
	if env.witness == nil {
		return nil, fmt.Errorf("no append-only ledger witness is configured")
	}
	canonicalPath, err := canonicalLedgerPath(auth.LedgerCanonicalPath)
	if err != nil {
		return nil, err
	}
	if canonicalPath != auth.LedgerCanonicalPath || digestBytes([]byte(canonicalPath)) != auth.LedgerPathSHA256 {
		return nil, fmt.Errorf("ledger path differs from authorization")
	}
	info, err := os.Lstat(canonicalPath)
	if err != nil {
		return nil, fmt.Errorf("bound execution ledger missing: %w", err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() || info.Mode().Perm()&0o077 != 0 {
		return nil, fmt.Errorf("bound execution ledger is unsafe")
	}
	file, err := os.OpenFile(canonicalPath, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return nil, err
	}
	if err := lockLedgerFile(file); err != nil {
		_ = file.Close()
		return nil, err
	}
	defer func() { _ = unlockLedgerFile(file); _ = file.Close() }()
	if err := requireCurrentOwner(info); err != nil {
		return nil, err
	}
	ledger := &ExecutionLedger{
		path: canonicalPath, auth: auth,
		binding: LedgerBinding{auth.LedgerID, auth.LedgerCanonicalPath, auth.LedgerPathSHA256, auth.LedgerGenesisSHA256},
		witness: env.witness,
	}
	if _, err := readLedger(file, auth, ledger.binding, env.witness); err != nil {
		return nil, err
	}
	return ledger, nil
}

func ledgerRecordDigest(record LedgerRecord) (string, error) {
	record.RecordSHA256 = ""
	return DigestDomain("ledger-record", record)
}

func readLedger(file *os.File, auth Authorization, binding LedgerBinding, witness ledgerWitness) (LedgerSnapshot, error) {
	if _, err := file.Seek(0, 0); err != nil {
		return LedgerSnapshot{}, err
	}
	scanner := bufio.NewScanner(file)
	scanner.Buffer(make([]byte, 1024), 1<<20)
	if !scanner.Scan() {
		return LedgerSnapshot{}, fmt.Errorf("execution ledger genesis missing")
	}
	var genesis LedgerGenesis
	if err := strictDecode(bytes.NewReader(scanner.Bytes()), &genesis); err != nil {
		return LedgerSnapshot{}, fmt.Errorf("invalid ledger genesis: %w", err)
	}
	genesisDigest, _ := ledgerGenesisDigest(genesis)
	canonicalGenesis, _ := canonicalJSON(genesis)
	if !bytes.Equal(canonicalGenesis, scanner.Bytes()) || genesis.GenesisSHA256 != genesisDigest ||
		genesis.GenesisSHA256 != binding.LedgerGenesisSHA256 || genesis.LedgerID != binding.LedgerID ||
		genesis.LedgerPathSHA256 != binding.LedgerPathSHA256 || genesis.CorpusSHA256 != auth.CorpusSHA256 ||
		genesis.SplitSHA256 != auth.SplitSHA256 || genesis.ScheduleSHA256 != auth.ScheduleSHA256 ||
		genesis.AnalysisSHA256 != auth.AnalysisSHA256 || genesis.TokenBound != auth.TokenBound ||
		genesis.PriorSpendNanoUSD != auth.PriorSpendNanoUSD || genesis.TotalCapNanoUSD != auth.TotalCapNanoUSD {
		return LedgerSnapshot{}, fmt.Errorf("execution ledger genesis mismatch")
	}
	snapshot := LedgerSnapshot{Attempted: map[string]bool{}, Outstanding: map[string]int64{}, LastSHA256: genesis.GenesisSHA256}
	previous := genesis.GenesisSHA256
	for scanner.Scan() {
		var record LedgerRecord
		if err := strictDecode(bytes.NewReader(scanner.Bytes()), &record); err != nil {
			return LedgerSnapshot{}, fmt.Errorf("invalid ledger record: %w", err)
		}
		if record.SchemaVersion != SchemaVersion+"/execution-ledger" || record.Sequence != snapshot.Records+1 ||
			record.AuthorizationSHA256 != auth.AuthorizationSHA256 || record.PreviousSHA256 != previous ||
			record.CallID == "" || record.At.IsZero() {
			return LedgerSnapshot{}, fmt.Errorf("execution ledger chain mismatch")
		}
		digest, err := ledgerRecordDigest(record)
		canonical, canonicalErr := canonicalJSON(record)
		if err != nil || canonicalErr != nil || digest != record.RecordSHA256 || !bytes.Equal(canonical, scanner.Bytes()) {
			return LedgerSnapshot{}, fmt.Errorf("execution ledger digest mismatch")
		}
		switch record.Event {
		case "attempt":
			if snapshot.Attempted[record.CallID] || record.ReservedNanoUSD != auth.WorstCaseReserveNanoUSD ||
				record.ChargedNanoUSD != 0 || record.CostBasis != "worst_case_reserve" {
				return LedgerSnapshot{}, fmt.Errorf("invalid or duplicate ledger attempt")
			}
			snapshot.Attempted[record.CallID] = true
			snapshot.Outstanding[record.CallID] = record.ReservedNanoUSD
			snapshot.OutstandingNanoUSD += record.ReservedNanoUSD
		case "settlement":
			reserved, ok := snapshot.Outstanding[record.CallID]
			if !ok || record.ReservedNanoUSD != reserved || record.ChargedNanoUSD < 0 || record.ChargedNanoUSD > reserved ||
				(record.CostBasis != "authoritative_usage" && record.CostBasis != "ambiguous_full_reserve") {
				return LedgerSnapshot{}, fmt.Errorf("invalid ledger settlement")
			}
			delete(snapshot.Outstanding, record.CallID)
			snapshot.OutstandingNanoUSD -= reserved
			snapshot.SpentNanoUSD += record.ChargedNanoUSD
		default:
			return LedgerSnapshot{}, fmt.Errorf("unknown ledger event")
		}
		previous, snapshot.LastSHA256 = record.RecordSHA256, record.RecordSHA256
		snapshot.Records++
		snapshot.Entries = append(snapshot.Entries, record)
	}
	if err := scanner.Err(); err != nil {
		return LedgerSnapshot{}, err
	}
	if auth.PriorSpendNanoUSD+snapshot.SpentNanoUSD+snapshot.OutstandingNanoUSD > auth.TotalCapNanoUSD {
		return LedgerSnapshot{}, fmt.Errorf("execution ledger exceeds cumulative cap")
	}
	if err := witness.Verify(binding.LedgerID, binding.LedgerGenesisSHA256, snapshot.LastSHA256, snapshot.Records); err != nil {
		return LedgerSnapshot{}, err
	}
	return snapshot, nil
}

func (l *ExecutionLedger) withFile(fn func(*os.File, LedgerSnapshot) error) error {
	file, err := os.OpenFile(l.path, os.O_RDWR|os.O_APPEND, 0)
	if err != nil {
		return err
	}
	if err := lockLedgerFile(file); err != nil {
		_ = file.Close()
		return err
	}
	defer func() { _ = unlockLedgerFile(file); _ = file.Close() }()
	snapshot, err := readLedger(file, l.auth, l.binding, l.witness)
	if err != nil {
		return err
	}
	return fn(file, snapshot)
}

func appendLedger(file *os.File, snapshot LedgerSnapshot, binding LedgerBinding, witness ledgerWitness, record LedgerRecord) (string, error) {
	record.SchemaVersion = SchemaVersion + "/execution-ledger"
	record.Sequence = snapshot.Records + 1
	record.PreviousSHA256 = snapshot.LastSHA256
	record.RecordSHA256, _ = ledgerRecordDigest(record)
	if err := witness.Advance(binding.LedgerID, snapshot.LastSHA256, record.RecordSHA256, record.Sequence); err != nil {
		return "", err
	}
	data, err := canonicalJSON(record)
	if err != nil {
		return "", err
	}
	n, err := file.Write(append(data, '\n'))
	if err != nil {
		return "", err
	}
	if n != len(data)+1 {
		return "", io.ErrShortWrite
	}
	if err := file.Sync(); err != nil {
		return "", err
	}
	return record.RecordSHA256, nil
}

func (l *ExecutionLedger) begin(callID string, at time.Time) (string, error) {
	var digest string
	err := l.withFile(func(file *os.File, snapshot LedgerSnapshot) error {
		if snapshot.Attempted[callID] {
			return fmt.Errorf("scheduled call already attempted")
		}
		reserve := l.auth.WorstCaseReserveNanoUSD
		if reserve <= 0 || l.auth.PriorSpendNanoUSD+snapshot.SpentNanoUSD+snapshot.OutstandingNanoUSD+reserve > l.auth.TotalCapNanoUSD {
			return fmt.Errorf("cumulative budget cap exceeded")
		}
		var err error
		digest, err = appendLedger(file, snapshot, l.binding, l.witness, LedgerRecord{
			AuthorizationSHA256: l.auth.AuthorizationSHA256, CallID: callID, Event: "attempt",
			ReservedNanoUSD: reserve, CostBasis: "worst_case_reserve", At: at.UTC(),
		})
		return err
	})
	return digest, err
}

func (l *ExecutionLedger) settle(callID string, charged int64, authoritative bool, at time.Time) (string, error) {
	var digest string
	err := l.withFile(func(file *os.File, snapshot LedgerSnapshot) error {
		reserved, ok := snapshot.Outstanding[callID]
		if !ok {
			return fmt.Errorf("call has no outstanding reservation")
		}
		basis := "ambiguous_full_reserve"
		if authoritative {
			basis = "authoritative_usage"
		} else {
			charged = reserved
		}
		if charged < 0 || charged > reserved {
			return fmt.Errorf("invalid settlement cost")
		}
		var err error
		digest, err = appendLedger(file, snapshot, l.binding, l.witness, LedgerRecord{
			AuthorizationSHA256: l.auth.AuthorizationSHA256, CallID: callID, Event: "settlement",
			ReservedNanoUSD: reserved, ChargedNanoUSD: charged, CostBasis: basis, At: at.UTC(),
		})
		return err
	})
	return digest, err
}

func (l *ExecutionLedger) Snapshot() (LedgerSnapshot, error) {
	var snapshot LedgerSnapshot
	err := l.withFile(func(_ *os.File, current LedgerSnapshot) error { snapshot = current; return nil })
	return snapshot, err
}

func ValidateReceiptLedgerBinding(receipts []Receipt, ledger *ExecutionLedger) error {
	if ledger == nil {
		return fmt.Errorf("execution ledger required")
	}
	snapshot, err := ledger.Snapshot()
	if err != nil {
		return err
	}
	attempts, settlements := map[string]LedgerRecord{}, map[string]LedgerRecord{}
	for _, record := range snapshot.Entries {
		if record.Event == "attempt" {
			attempts[record.CallID] = record
		} else {
			settlements[record.CallID] = record
		}
	}
	for _, receipt := range receipts {
		attempt, attempted := attempts[receipt.CallID]
		settlement, settled := settlements[receipt.CallID]
		if receipt.Status == "not_attempted" {
			if attempted || settled {
				return fmt.Errorf("not-attempted receipt has ledger events")
			}
			continue
		}
		if !attempted || !settled || receipt.LedgerAttemptSHA256 != attempt.RecordSHA256 ||
			receipt.LedgerSettlementSHA256 != settlement.RecordSHA256 ||
			receipt.InferredCostNanoUSD == nil || *receipt.InferredCostNanoUSD != settlement.ChargedNanoUSD ||
			receipt.CostBasis != settlement.CostBasis {
			return fmt.Errorf("receipt/ledger mismatch for %s", receipt.CallID)
		}
	}
	return nil
}
