package studyfinal

import (
	"crypto/ed25519"
	"encoding/hex"
	"fmt"
	"regexp"
	"runtime/debug"
	"sort"
	"time"
)

var requiredExternalGates = []string{
	"immutable_exact_model",
	"no_disallowed_post_submission_tuning_or_approved_temporal_interpretation",
	"publication_independence",
	"retention_requirement_as_preregistered",
	"provider_side_numeric_spend_cap",
	"current_pricing",
	"current_terms_and_privacy",
	"exact_model_list",
}

type GateResult struct {
	Name           string `json:"name"`
	Status         string `json:"status"`
	EvidenceSHA256 string `json:"evidence_sha256"`
}

type ReviewApproval struct {
	Review       string `json:"review"`
	ReviewerID   string `json:"reviewer_id"`
	PublicKeyHex string `json:"public_key_hex"`
	SignatureHex string `json:"signature_hex"`
}

type GateAttestation struct {
	SchemaVersion                 string           `json:"schema_version"`
	Status                        string           `json:"status"`
	DenyReason                    string           `json:"deny_reason,omitempty"`
	ExactHeadCommit               string           `json:"exact_head_commit"`
	Provider                      string           `json:"provider"`
	AccountIDHash                 string           `json:"account_id_hash"`
	RetentionPolicySHA256         string           `json:"retention_policy_sha256"`
	CollectorPublicKeySHA256      string           `json:"collector_public_key_sha256"`
	Model                         string           `json:"model"`
	ProviderCapNanoUSD            int64            `json:"provider_cap_nano_usd"`
	LedgerID                      string           `json:"ledger_id"`
	LedgerPathSHA256              string           `json:"ledger_path_sha256"`
	LedgerGenesisSHA256           string           `json:"ledger_genesis_sha256"`
	AgentTraceContaminationSHA256 string           `json:"agenttrace_contamination_sha256"`
	TrustRootVersion              string           `json:"trust_root_version"`
	TrustRootSHA256               string           `json:"trust_root_sha256"`
	CorpusSHA256                  string           `json:"corpus_sha256"`
	SourceRegistrySHA256          string           `json:"source_registry_sha256"`
	SplitSHA256                   string           `json:"split_sha256"`
	ScheduleSHA256                string           `json:"schedule_sha256"`
	AnalysisSHA256                string           `json:"analysis_sha256"`
	TokenBound                    int64            `json:"token_bound"`
	CallCount                     int              `json:"call_count"`
	ExpiresAt                     time.Time        `json:"expires_at"`
	Gates                         []GateResult     `json:"gates"`
	Approvals                     []ReviewApproval `json:"approvals"`
	StatementSHA256               string           `json:"statement_sha256"`
	AttestationSHA256             string           `json:"attestation_sha256"`
}

type TrustedReviewers map[string]ed25519.PublicKey

type buildIdentity struct {
	Revision string
	Modified bool
	Valid    bool
}

type authorizationEnvironment struct {
	trustVersion        string
	trustDigest         string
	trusted             TrustedReviewers
	build               func() buildIdentity
	witness             ledgerWitness
	contaminationDigest string
}

const compiledTrustRootVersion = "no-production-trust-root-v1"
const compiledTrustRootSHA256 = "44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a"
const reviewedAgentTraceContaminationSHA256 = "fa2bb7022b30d043d1cabba085253190dba94dd0b85eed98a9cc4fa5e23d0c19"

func productionAuthorizationEnvironment() authorizationEnvironment {
	return authorizationEnvironment{
		trustVersion:        compiledTrustRootVersion,
		trustDigest:         compiledTrustRootSHA256,
		trusted:             TrustedReviewers{},
		build:               runtimeBuildIdentity,
		contaminationDigest: reviewedAgentTraceContaminationSHA256,
	}
}

func runtimeBuildIdentity() buildIdentity {
	info, ok := debug.ReadBuildInfo()
	if !ok {
		return buildIdentity{}
	}
	identity := buildIdentity{}
	modifiedPresent := false
	for _, setting := range info.Settings {
		switch setting.Key {
		case "vcs.revision":
			identity.Revision = setting.Value
		case "vcs.modified":
			modifiedPresent = true
			identity.Modified = setting.Value != "false"
		}
	}
	identity.Valid = modifiedPresent && regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(identity.Revision)
	return identity
}

func gateStatementDigest(attestation GateAttestation) (string, error) {
	attestation.Approvals = nil
	attestation.StatementSHA256 = ""
	attestation.AttestationSHA256 = ""
	return DigestDomain("gate-statement", attestation)
}

func gateAttestationDigest(attestation GateAttestation) (string, error) {
	attestation.AttestationSHA256 = ""
	return DigestDomain("gate-attestation", attestation)
}

func FinalizeGateAttestation(attestation GateAttestation, signers map[string]ed25519.PrivateKey) (GateAttestation, error) {
	sort.Slice(attestation.Gates, func(i, j int) bool { return attestation.Gates[i].Name < attestation.Gates[j].Name })
	attestation.Approvals = nil
	statement, err := gateStatementDigest(attestation)
	if err != nil {
		return GateAttestation{}, err
	}
	attestation.StatementSHA256 = statement
	for _, review := range requiredReviews() {
		privateKey, ok := signers[review]
		if !ok || len(privateKey) != ed25519.PrivateKeySize {
			return GateAttestation{}, fmt.Errorf("missing signer for %s", review)
		}
		publicKey := privateKey.Public().(ed25519.PublicKey)
		reviewerID := review + "-reviewer"
		signature := ed25519.Sign(privateKey, []byte(statement+"\x00"+review+"\x00"+reviewerID))
		attestation.Approvals = append(attestation.Approvals, ReviewApproval{
			Review: review, ReviewerID: reviewerID, PublicKeyHex: hex.EncodeToString(publicKey),
			SignatureHex: hex.EncodeToString(signature),
		})
	}
	attestation.AttestationSHA256, _ = gateAttestationDigest(attestation)
	return attestation, nil
}

func requiredReviews() []string {
	return []string{"methodology", "operations", "privacy", "reproducibility", "security", "statistics"}
}

func trustedReviewersDigest(trusted TrustedReviewers) (string, error) {
	encoded := map[string]string{}
	for review, key := range trusted {
		encoded[review] = hex.EncodeToString(key)
	}
	return DigestDomain("reviewer-trust-root", encoded)
}

func validateGateAttestation(attestation GateAttestation, env authorizationEnvironment, d Dataset, schedule []ScheduleEntry, analysisHash string, tokenBound int64, now time.Time) error {
	if attestation.SchemaVersion != SchemaVersion+"/gate-attestation" || attestation.Status != "passed" || attestation.DenyReason != "" {
		return fmt.Errorf("external gate attestation is no-go")
	}
	if !regexp.MustCompile(`^[0-9a-f]{40}$`).MatchString(attestation.ExactHeadCommit) ||
		attestation.Provider != "TypeSafe AI" || attestation.Model != JevModel ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(attestation.AccountIDHash) ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(attestation.RetentionPolicySHA256) ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(attestation.CollectorPublicKeySHA256) ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(attestation.LedgerID) ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(attestation.LedgerPathSHA256) ||
		!regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(attestation.LedgerGenesisSHA256) {
		return fmt.Errorf("gate identity is incomplete")
	}
	trustDigest, _ := trustedReviewersDigest(env.trusted)
	if len(env.trusted) != len(requiredReviews()) || env.trustVersion == "" ||
		trustDigest != env.trustDigest || attestation.TrustRootVersion != env.trustVersion ||
		attestation.TrustRootSHA256 != env.trustDigest {
		return fmt.Errorf("compiled reviewer trust root is unavailable or mismatched")
	}
	seenKeys := map[string]bool{}
	for _, review := range requiredReviews() {
		key, ok := env.trusted[review]
		encoded := hex.EncodeToString(key)
		if !ok || len(key) != ed25519.PublicKeySize || seenKeys[encoded] {
			return fmt.Errorf("compiled reviewer keys are missing, substituted, or non-distinct")
		}
		seenKeys[encoded] = true
	}
	identity := env.build()
	if !identity.Valid || identity.Modified || identity.Revision != attestation.ExactHeadCommit {
		return fmt.Errorf("runtime build identity is absent, dirty, or differs from reviewed head")
	}
	scheduleHash, err := DigestDomain("run-schedule", schedule)
	if err != nil {
		return err
	}
	if attestation.CorpusSHA256 != d.CorpusSHA256 || attestation.SplitSHA256 != d.SplitSHA256 ||
		attestation.SourceRegistrySHA256 != d.SourceRegistrySHA256 ||
		attestation.ScheduleSHA256 != scheduleHash || attestation.AnalysisSHA256 != analysisHash ||
		attestation.TokenBound != tokenBound || attestation.CallCount != len(schedule) ||
		attestation.ProviderCapNanoUSD != TotalCapNanoUSD || !now.Before(attestation.ExpiresAt) {
		return fmt.Errorf("gate attestation binding mismatch")
	}
	if attestation.AgentTraceContaminationSHA256 != d.AgentTraceContamination.EvidenceSHA256 {
		return fmt.Errorf("gate attestation contamination binding mismatch")
	}
	expectedGates := map[string]bool{}
	for _, name := range requiredExternalGates {
		expectedGates[name] = true
	}
	if len(attestation.Gates) != len(expectedGates) {
		return fmt.Errorf("required gate count mismatch")
	}
	for _, gate := range attestation.Gates {
		if !expectedGates[gate.Name] || gate.Status != "passed" || !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(gate.EvidenceSHA256) {
			return fmt.Errorf("gate %q did not pass", gate.Name)
		}
		delete(expectedGates, gate.Name)
	}
	statement, err := gateStatementDigest(attestation)
	if err != nil || statement != attestation.StatementSHA256 {
		return fmt.Errorf("gate statement digest mismatch")
	}
	if len(attestation.Approvals) != len(requiredReviews()) {
		return fmt.Errorf("review approval count mismatch")
	}
	seen := map[string]bool{}
	for _, approval := range attestation.Approvals {
		if seen[approval.Review] {
			return fmt.Errorf("duplicate review approval")
		}
		key, ok := env.trusted[approval.Review]
		if !ok || approval.ReviewerID == "" || approval.PublicKeyHex != hex.EncodeToString(key) {
			return fmt.Errorf("untrusted %s reviewer", approval.Review)
		}
		signature, err := hex.DecodeString(approval.SignatureHex)
		if err != nil || !ed25519.Verify(key, []byte(statement+"\x00"+approval.Review+"\x00"+approval.ReviewerID), signature) {
			return fmt.Errorf("invalid %s approval signature", approval.Review)
		}
		seen[approval.Review] = true
	}
	digest, err := gateAttestationDigest(attestation)
	if err != nil || digest != attestation.AttestationSHA256 {
		return fmt.Errorf("gate attestation digest mismatch")
	}
	return nil
}

func ValidateGateAttestation(attestation GateAttestation, d Dataset, schedule []ScheduleEntry, analysisHash string, tokenBound int64, now time.Time) error {
	return validateGateAttestation(attestation, productionAuthorizationEnvironment(), d, schedule, analysisHash, tokenBound, now)
}
