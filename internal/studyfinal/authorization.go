package studyfinal

import (
	"fmt"
	"regexp"
	"time"
)

type Authorization struct {
	SchemaVersion                 string    `json:"schema_version"`
	Model                         string    `json:"model"`
	ModelSHA256                   string    `json:"model_sha256"`
	PricingVersion                string    `json:"pricing_version"`
	PricingSHA256                 string    `json:"pricing_sha256"`
	PricingNanoUSDPerToken        int64     `json:"pricing_nano_usd_per_token"`
	OutputPriceNanoUSD            int64     `json:"output_price_nano_usd_per_token"`
	TotalCapUSD                   string    `json:"total_cap_usd"`
	PriorSpendUSD                 string    `json:"prior_inferred_spend_usd"`
	RemainingCapUSD               string    `json:"remaining_cap_usd"`
	CorpusSHA256                  string    `json:"corpus_sha256"`
	SourceRegistrySHA256          string    `json:"source_registry_sha256"`
	SplitSHA256                   string    `json:"split_sha256"`
	ScheduleSHA256                string    `json:"schedule_sha256"`
	AnalysisSHA256                string    `json:"analysis_sha256"`
	GateAttestationSHA256         string    `json:"gate_attestation_sha256"`
	ExactHeadCommit               string    `json:"exact_head_commit"`
	Provider                      string    `json:"provider"`
	AccountIDHash                 string    `json:"account_id_hash"`
	RetentionPolicySHA256         string    `json:"retention_policy_sha256"`
	CollectorPublicKeySHA256      string    `json:"collector_public_key_sha256"`
	ReviewApprovalsSHA256         string    `json:"review_approvals_sha256"`
	TrustedReviewersSHA256        string    `json:"trusted_reviewers_sha256"`
	ProviderCapNanoUSD            int64     `json:"provider_cap_nano_usd"`
	LedgerID                      string    `json:"ledger_id"`
	LedgerCanonicalPath           string    `json:"ledger_canonical_path"`
	LedgerPathSHA256              string    `json:"ledger_path_sha256"`
	LedgerGenesisSHA256           string    `json:"ledger_genesis_sha256"`
	AgentTraceContaminationSHA256 string    `json:"agenttrace_contamination_sha256"`
	CallCount                     int       `json:"call_count"`
	TokenBound                    int64     `json:"token_bound"`
	TotalCapNanoUSD               int64     `json:"total_cap_nano_usd"`
	PriorSpendNanoUSD             int64     `json:"prior_inferred_spend_nano_usd"`
	RemainingCapNanoUSD           int64     `json:"remaining_cap_nano_usd"`
	WorstCaseReserveNanoUSD       int64     `json:"worst_case_reserve_nano_usd"`
	ExpiresAt                     time.Time `json:"expires_at"`
	AuthorizationSHA256           string    `json:"authorization_sha256"`
}

type AuthorizedSession struct {
	authorization Authorization
	attestation   GateAttestation
	analysisHash  string
	environment   authorizationEnvironment
	valid         bool
}

func NewAuthorizedSession(authorization Authorization, d Dataset, schedule []ScheduleEntry, analysisHash string, attestation GateAttestation, now time.Time) (*AuthorizedSession, error) {
	return newAuthorizedSession(authorization, d, schedule, analysisHash, attestation, productionAuthorizationEnvironment(), now)
}

func newAuthorizedSession(authorization Authorization, d Dataset, schedule []ScheduleEntry, analysisHash string, attestation GateAttestation, env authorizationEnvironment, now time.Time) (*AuthorizedSession, error) {
	if err := validateAuthorization(authorization, d, schedule, analysisHash, attestation, env, now); err != nil {
		return nil, err
	}
	return &AuthorizedSession{
		authorization: authorization, attestation: attestation,
		analysisHash: analysisHash, environment: env, valid: true,
	}, nil
}

func (s *AuthorizedSession) validate(d Dataset, schedule []ScheduleEntry, now time.Time) error {
	if s == nil || !s.valid {
		return fmt.Errorf("validated authorization session required")
	}
	return validateAuthorization(s.authorization, d, schedule, s.analysisHash, s.attestation, s.environment, now)
}

func NewAuthorization(d Dataset, schedule []ScheduleEntry, analysisHash string, tokenBound int64, expires time.Time, attestation GateAttestation, ledger LedgerBinding, now time.Time) (Authorization, error) {
	return newAuthorization(d, schedule, analysisHash, tokenBound, expires, attestation, ledger, productionAuthorizationEnvironment(), now)
}

func newAuthorization(d Dataset, schedule []ScheduleEntry, analysisHash string, tokenBound int64, expires time.Time, attestation GateAttestation, ledger LedgerBinding, env authorizationEnvironment, now time.Time) (Authorization, error) {
	if tokenBound <= 0 || analysisHash == "" || expires.IsZero() {
		return Authorization{}, fmt.Errorf("token bound, analysis hash, and expiry are required")
	}
	if !d.Executable {
		return Authorization{}, fmt.Errorf("unresolved source registry cannot be authorized")
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(analysisHash) {
		return Authorization{}, fmt.Errorf("analysis hash must be lowercase SHA-256")
	}
	if err := validateGateAttestation(attestation, env, d, schedule, analysisHash, tokenBound, now); err != nil {
		return Authorization{}, err
	}
	if attestation.LedgerID != ledger.LedgerID || attestation.LedgerPathSHA256 != ledger.LedgerPathSHA256 ||
		attestation.LedgerGenesisSHA256 != ledger.LedgerGenesisSHA256 {
		return Authorization{}, fmt.Errorf("gate attestation ledger binding mismatch")
	}
	if err := validateFreshLedgerBinding(ledger, d, schedule, analysisHash, tokenBound, env); err != nil {
		return Authorization{}, err
	}
	if !expires.Equal(attestation.ExpiresAt) {
		return Authorization{}, fmt.Errorf("authorization expiry must equal gate attestation expiry")
	}
	if err := validateSchedule(d, schedule, env); err != nil {
		return Authorization{}, err
	}
	scheduleHash, err := DigestDomain("run-schedule", schedule)
	if err != nil {
		return Authorization{}, err
	}
	if tokenBound > int64(^uint64(0)>>1)/InputPriceNanoUSD {
		return Authorization{}, fmt.Errorf("token reserve overflows")
	}
	reserve := tokenBound * InputPriceNanoUSD
	if int64(len(schedule)) > RemainingCapNanoUSD/reserve {
		return Authorization{}, fmt.Errorf("worst-case schedule cost exceeds remaining cap")
	}
	a := Authorization{
		SchemaVersion: SchemaVersion + "/authorization", Model: JevModel,
		ModelSHA256: digestBytes([]byte(JevModel)), PricingVersion: PricingVersion,
		PricingSHA256: digestBytes([]byte(PricingVersion)), PricingNanoUSDPerToken: InputPriceNanoUSD,
		OutputPriceNanoUSD: 0, TotalCapUSD: TotalCapUSD, PriorSpendUSD: PriorSpendUSD, RemainingCapUSD: RemainingCapUSD,
		CorpusSHA256: d.CorpusSHA256, SourceRegistrySHA256: d.SourceRegistrySHA256,
		SplitSHA256: d.SplitSHA256, ScheduleSHA256: scheduleHash,
		AnalysisSHA256: analysisHash, GateAttestationSHA256: attestation.AttestationSHA256,
		ExactHeadCommit: attestation.ExactHeadCommit, Provider: attestation.Provider,
		AccountIDHash: attestation.AccountIDHash, RetentionPolicySHA256: attestation.RetentionPolicySHA256,
		CollectorPublicKeySHA256: attestation.CollectorPublicKeySHA256,
		ProviderCapNanoUSD:       attestation.ProviderCapNanoUSD,
		LedgerID:                 ledger.LedgerID, LedgerCanonicalPath: ledger.CanonicalPath,
		LedgerPathSHA256: ledger.LedgerPathSHA256, LedgerGenesisSHA256: ledger.LedgerGenesisSHA256,
		AgentTraceContaminationSHA256: attestation.AgentTraceContaminationSHA256,
		CallCount:                     len(schedule), TokenBound: tokenBound,
		TotalCapNanoUSD: TotalCapNanoUSD, PriorSpendNanoUSD: PriorSpendNanoUSD,
		RemainingCapNanoUSD: RemainingCapNanoUSD, WorstCaseReserveNanoUSD: reserve, ExpiresAt: expires.UTC(),
	}
	a.ReviewApprovalsSHA256, _ = DigestDomain("review-approvals", attestation.Approvals)
	a.TrustedReviewersSHA256, _ = trustedReviewersDigest(env.trusted)
	a.AuthorizationSHA256, _ = authorizationDigest(a)
	return a, nil
}

func authorizationDigest(a Authorization) (string, error) {
	a.AuthorizationSHA256 = ""
	return DigestDomain("authorization", a)
}

func ValidateAuthorization(a Authorization, d Dataset, schedule []ScheduleEntry, analysisHash string, attestation GateAttestation, now time.Time) error {
	return validateAuthorization(a, d, schedule, analysisHash, attestation, productionAuthorizationEnvironment(), now)
}

func validateAuthorization(a Authorization, d Dataset, schedule []ScheduleEntry, analysisHash string, attestation GateAttestation, env authorizationEnvironment, now time.Time) error {
	if err := validateCorpus(d, env); err != nil {
		return err
	}
	if err := validateSchedule(d, schedule, env); err != nil {
		return err
	}
	if !regexp.MustCompile(`^[0-9a-f]{64}$`).MatchString(analysisHash) {
		return fmt.Errorf("analysis hash must be lowercase SHA-256")
	}
	if a.SchemaVersion != SchemaVersion+"/authorization" ||
		a.Model != JevModel || a.ModelSHA256 != digestBytes([]byte(JevModel)) ||
		a.PricingVersion != PricingVersion || a.PricingSHA256 != digestBytes([]byte(PricingVersion)) ||
		a.PricingNanoUSDPerToken != InputPriceNanoUSD || a.OutputPriceNanoUSD != 0 ||
		a.TotalCapUSD != TotalCapUSD || a.PriorSpendUSD != PriorSpendUSD || a.RemainingCapUSD != RemainingCapUSD ||
		a.TotalCapNanoUSD != TotalCapNanoUSD || a.PriorSpendNanoUSD != PriorSpendNanoUSD ||
		a.RemainingCapNanoUSD != RemainingCapNanoUSD || a.RemainingCapNanoUSD != a.TotalCapNanoUSD-a.PriorSpendNanoUSD {
		return fmt.Errorf("authorization pricing or cap mismatch")
	}
	if !now.Before(a.ExpiresAt) {
		return fmt.Errorf("authorization expired")
	}
	if err := validateGateAttestation(attestation, env, d, schedule, analysisHash, a.TokenBound, now); err != nil {
		return err
	}
	approvalsDigest, _ := DigestDomain("review-approvals", attestation.Approvals)
	trustedDigest, _ := trustedReviewersDigest(env.trusted)
	if a.GateAttestationSHA256 != attestation.AttestationSHA256 ||
		a.ExactHeadCommit != attestation.ExactHeadCommit || a.Provider != attestation.Provider ||
		a.AccountIDHash != attestation.AccountIDHash || a.RetentionPolicySHA256 != attestation.RetentionPolicySHA256 ||
		a.CollectorPublicKeySHA256 != attestation.CollectorPublicKeySHA256 ||
		a.ReviewApprovalsSHA256 != approvalsDigest || a.TrustedReviewersSHA256 != trustedDigest ||
		a.ProviderCapNanoUSD != attestation.ProviderCapNanoUSD ||
		a.LedgerID != attestation.LedgerID || a.LedgerPathSHA256 != attestation.LedgerPathSHA256 ||
		a.LedgerGenesisSHA256 != attestation.LedgerGenesisSHA256 ||
		a.AgentTraceContaminationSHA256 != attestation.AgentTraceContaminationSHA256 ||
		a.SourceRegistrySHA256 != attestation.SourceRegistrySHA256 ||
		digestBytes([]byte(a.LedgerCanonicalPath)) != a.LedgerPathSHA256 ||
		!a.ExpiresAt.Equal(attestation.ExpiresAt) {
		return fmt.Errorf("authorization gate binding mismatch")
	}
	if a.CorpusSHA256 != d.CorpusSHA256 || a.SourceRegistrySHA256 != d.SourceRegistrySHA256 ||
		a.SplitSHA256 != d.SplitSHA256 || a.AnalysisSHA256 != analysisHash ||
		a.CallCount != len(schedule) || a.TokenBound <= 0 || a.WorstCaseReserveNanoUSD != a.TokenBound*InputPriceNanoUSD {
		return fmt.Errorf("authorization binding mismatch")
	}
	scheduleHash, err := DigestDomain("run-schedule", schedule)
	if err != nil || scheduleHash != a.ScheduleSHA256 {
		return fmt.Errorf("authorization schedule mismatch")
	}
	digest, err := authorizationDigest(a)
	if err != nil || digest != a.AuthorizationSHA256 {
		return fmt.Errorf("authorization digest mismatch")
	}
	if int64(a.CallCount) > a.RemainingCapNanoUSD/a.WorstCaseReserveNanoUSD {
		return fmt.Errorf("authorization exceeds remaining cap")
	}
	return nil
}
