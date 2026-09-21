package studyfinal

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
)

func ValidateDecision(d Decision) error {
	if d.System != "deterministic-baseline" && d.System != "jev" {
		return fmt.Errorf("unknown decision system")
	}
	if d.System == "jev" && d.Model != JevModel {
		return fmt.Errorf("provider model drift: got %q", d.Model)
	}
	if len(d.Answers) != len(frozenQuestions) {
		return fmt.Errorf("expected exactly seven answers")
	}
	for _, q := range frozenQuestions {
		a, ok := d.Answers[q.ID]
		if !ok {
			return fmt.Errorf("missing answer %s", q.ID)
		}
		if err := validateAnswer(q, a, d.System == "jev"); err != nil {
			return fmt.Errorf("%s: %w", q.ID, err)
		}
	}
	if d.Disposition != "accept" && d.Disposition != "review" && d.Disposition != "reject" {
		return fmt.Errorf("invalid disposition")
	}
	expectedDisposition, expectedReasons, expectedScore := ApplyPolicy(d.Answers, d.PolicyFacts)
	if d.Disposition != expectedDisposition || !slices.Equal(d.ReasonCodes, expectedReasons) ||
		(d.AcceptanceScore == nil) != (expectedScore == nil) ||
		(d.AcceptanceScore != nil && math.Abs(*d.AcceptanceScore-*expectedScore) > 1e-12) {
		return fmt.Errorf("decision does not match deterministic policy")
	}
	return nil
}

func validateAnswer(q Question, a Answer, probabilistic bool) error {
	allowed := make(map[string]bool, len(q.Allowed))
	for _, label := range q.Allowed {
		allowed[label] = true
	}
	if !allowed[a.SelectedLabel] {
		return fmt.Errorf("invalid selected label %q", a.SelectedLabel)
	}
	if !probabilistic {
		if a.Probabilities != nil || a.RawConfidence != nil {
			return fmt.Errorf("deterministic answer must not fabricate probabilities")
		}
		return nil
	}
	if len(a.Probabilities) != len(q.Allowed) {
		return fmt.Errorf("incomplete probability vector")
	}
	sum := 0.0
	for label, p := range a.Probabilities {
		if !allowed[label] {
			return fmt.Errorf("unknown probability label %q", label)
		}
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return fmt.Errorf("invalid probability")
		}
		sum += p
	}
	if math.Abs(sum-1) > 1e-9 {
		return fmt.Errorf("probabilities sum to %.17g", sum)
	}
	selected := a.Probabilities[a.SelectedLabel]
	if math.Abs(selected-a.SelectedLabelProbability) > 1e-12 {
		return fmt.Errorf("selected-label probability mismatch")
	}
	for label, p := range a.Probabilities {
		if p > selected+1e-12 || (math.Abs(p-selected) <= 1e-12 && label < a.SelectedLabel) {
			return fmt.Errorf("selected label does not use maximum probability with lexical tie-break")
		}
	}
	if a.RawConfidence == nil || math.IsNaN(*a.RawConfidence) || math.IsInf(*a.RawConfidence, 0) || *a.RawConfidence < 0 || *a.RawConfidence > 1 {
		return fmt.Errorf("invalid raw confidence")
	}
	return nil
}

func ApplyPolicy(answers map[string]Answer, facts PolicyFacts) (string, []string, *float64) {
	var reject, review []string
	if facts.SchemaContradiction {
		reject = append(reject, "schema_contradiction")
	}
	if facts.CryptoContradiction {
		reject = append(reject, "crypto_contradiction")
	}
	if facts.PrivacyContradiction {
		reject = append(reject, "privacy_contradiction")
	}
	if facts.AffirmativeUnsupportedClaim {
		reject = append(reject, "affirmative_unsupported_claim")
	}
	if facts.ClaimedPoolingIdentityMismatch {
		reject = append(reject, "identity_mismatch_claimed_comparable")
	}
	if facts.TeardownClaimed && !facts.IndependentProviderEvidence {
		reject = append(reject, "teardown_without_provider_evidence")
	}
	if facts.BufferedTimingClaimedAsTTFT {
		reject = append(reject, "buffered_timing_called_ttft")
	}
	if facts.ChecksumValidClosedRegistryBad {
		reject = append(reject, "closed_registry_invalid_reseal")
	}
	if facts.HonestIncomplete {
		review = append(review, "honest_incomplete")
	}
	if facts.HonestUnsupported {
		review = append(review, "honest_unsupported")
	}
	if facts.HonestRefusal {
		review = append(review, "honest_refusal")
	}
	if facts.ExplicitNoncomparability {
		review = append(review, "explicit_noncomparability")
	}
	if facts.MissingRequired {
		review = append(review, "missing_required")
	}
	if facts.StaleEvidence {
		review = append(review, "stale_evidence")
	}
	if facts.LocalRedactionReview {
		review = append(review, "locally_redacted_operational_path")
	}
	if len(reject) > 0 {
		sort.Strings(reject)
		return "reject", reject, acceptanceScore(answers)
	}
	if facts.HonestIncomplete || facts.HonestUnsupported || facts.HonestRefusal || facts.ExplicitNoncomparability ||
		facts.StaleEvidence || facts.LocalRedactionReview {
		sort.Strings(review)
		return "review", review, acceptanceScore(answers)
	}

	unsafe := map[string]map[string]bool{
		"evidence_complete":         {"no": true, "unknown": true},
		"observed_tests_support":    {"no": true, "unknown": true},
		"verifier_support":          {"no": true, "unknown": true},
		"cleanup_complete":          {"no": true, "unknown": true},
		"external_effects_resolved": {"no": true, "unknown": true},
		"patch_risk":                {"high": true, "unknown": true},
		"recommended_disposition":   {"reject": true},
	}
	for _, q := range frozenQuestions {
		a, ok := answers[q.ID]
		if !ok {
			review = append(review, "missing_answer_"+q.ID)
			continue
		}
		if unsafe[q.ID][a.SelectedLabel] {
			reject = append(reject, "unsafe_"+q.ID+"_"+a.SelectedLabel)
		}
		if a.SelectedLabel == "review" || a.SelectedLabel == "medium" {
			review = append(review, "review_"+q.ID+"_"+a.SelectedLabel)
		}
	}
	applicable := map[string]bool{
		"observed_tests_support":    !facts.ObservedTestsNotApplicable,
		"verifier_support":          !facts.VerifierNotApplicable,
		"cleanup_complete":          !facts.CleanupNotApplicable,
		"external_effects_resolved": !facts.ExternalEffectsNotApplicable,
	}
	for field, required := range applicable {
		if required && answers[field].SelectedLabel == "not_applicable" {
			review = append(review, "unsupported_not_applicable_"+field)
		}
	}
	sort.Strings(reject)
	sort.Strings(review)
	score := acceptanceScore(answers)
	if len(reject) > 0 {
		return "reject", reject, score
	}
	if len(review) > 0 {
		return "review", review, score
	}
	return "accept", []string{"all_registered_evidence_supports_accept"}, score
}

func acceptanceScore(answers map[string]Answer) *float64 {
	minimum := 1.0
	hasProbability := false
	safeLabel := map[string]string{
		"evidence_complete": "yes", "observed_tests_support": "yes", "verifier_support": "yes",
		"cleanup_complete": "yes", "external_effects_resolved": "yes", "patch_risk": "low",
		"recommended_disposition": "accept",
	}
	for field, label := range safeLabel {
		a, ok := answers[field]
		if !ok || a.Probabilities == nil {
			continue
		}
		hasProbability = true
		if a.Probabilities[label] < minimum {
			minimum = a.Probabilities[label]
		}
	}
	if !hasProbability {
		return nil
	}
	return &minimum
}

func DeterministicBaseline(labels map[string]string, facts PolicyFacts) (Decision, error) {
	answers := make(map[string]Answer, len(frozenQuestions))
	for _, q := range frozenQuestions {
		label, ok := labels[q.ID]
		if !ok {
			return Decision{}, fmt.Errorf("missing baseline observation %s", q.ID)
		}
		answers[q.ID] = Answer{SelectedLabel: label}
	}
	disposition, reasons, score := ApplyPolicy(answers, facts)
	d := Decision{System: "deterministic-baseline", Model: "final-policy-v1", Answers: answers, PolicyFacts: facts, Disposition: disposition, ReasonCodes: reasons, AcceptanceScore: score}
	return d, ValidateDecision(d)
}

// IndependentFacts derives policy facts from preserved evidence bytes and digests.
// It deliberately does not inspect Condition.Family or invoke the generator.
func IndependentFacts(condition Condition) PolicyFacts {
	var facts PolicyFacts
	if len(condition.SourceContents) != len(condition.SourceDigests) || len(condition.SourcePaths) != len(condition.SourceContents) {
		facts.SchemaContradiction = true
		return facts
	}
	if len(condition.SourceContents) < 2 {
		facts.HonestIncomplete = true
	}
	for i, content := range condition.SourceContents {
		if digestBytes([]byte(content)) != condition.SourceDigests[i] {
			facts.SchemaContradiction = true
			continue
		}
		for _, line := range strings.Split(content, "\n") {
			switch line {
			case "semantic=schema_contradiction", "observed_tests=fail", "contradicts=execution.txt":
				facts.SchemaContradiction = true
			case "semantic=crypto_contradiction":
				facts.CryptoContradiction = true
			case "semantic=privacy_contradiction":
				facts.PrivacyContradiction = true
			case "semantic=affirmative_unsupported":
				facts.AffirmativeUnsupportedClaim = true
			case "semantic=honest_incomplete":
				facts.HonestIncomplete = true
			case "semantic=honest_unsupported":
				facts.HonestUnsupported = true
			case "semantic=honest_refusal":
				facts.HonestRefusal = true
			case "semantic=explicit_noncomparability":
				facts.ExplicitNoncomparability = true
			case "semantic=claimed_pooling_identity_mismatch":
				facts.ClaimedPoolingIdentityMismatch = true
			case "semantic=teardown_unverified":
				facts.TeardownClaimed = true
			case "semantic=teardown_provider_verified":
				facts.TeardownClaimed, facts.IndependentProviderEvidence = true, true
			case "semantic=buffered_timing_claimed_ttft":
				facts.BufferedTimingClaimedAsTTFT = true
			case "semantic=closed_registry_invalid_reseal":
				facts.ChecksumValidClosedRegistryBad = true
			case "baseline=superseded_after_evidence":
				facts.StaleEvidence = true
			case "semantic=local_operational_redaction":
				facts.LocalRedactionReview = true
			}
		}
	}
	return facts
}

func expectedFactsForFamily(family string) PolicyFacts {
	var facts PolicyFacts
	switch family {
	case "contradiction", "expected_attested", "prompt_work", "output_evaluator", "schema_conflict":
		facts.SchemaContradiction = true
	case "checksum_mismatch", "tampered":
		facts.CryptoContradiction = true
	case "privacy_checksum", "secret_leak", "partial_redaction":
		facts.PrivacyContradiction = true
	case "affirmative_unsupported", "claimed_only", "html_only":
		facts.AffirmativeUnsupportedClaim = true
	case "missing_evidence", "missing_file", "missing", "missing_timing", "missing_outcome", "unfinished", "partial", "stream_incomplete":
		facts.HonestIncomplete = true
	case "unsupported_timing":
		facts.HonestUnsupported = true
	case "safe_refusal", "budget_preflight":
		facts.HonestRefusal = true
	case "explicit_noncomparability", "model_mismatch", "runtime_mismatch", "workload_mismatch", "evaluator_mismatch":
		facts.ExplicitNoncomparability = true
	case "claimed_pooling":
		facts.ClaimedPoolingIdentityMismatch = true
	case "local_shutdown", "provider_unverified":
		facts.TeardownClaimed = true
	case "provider_verified":
		facts.TeardownClaimed, facts.IndependentProviderEvidence = true, true
	case "buffered_ttft":
		facts.BufferedTimingClaimedAsTTFT = true
	case "closed_registry_invalid", "resealed":
		facts.ChecksumValidClosedRegistryBad = true
	case "stale_baseline":
		facts.StaleEvidence = true
	case "local_path_redacted":
		facts.LocalRedactionReview = true
	}
	return facts
}
