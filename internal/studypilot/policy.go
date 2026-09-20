package studypilot

import (
	"fmt"
	"sort"
)

var policyRuleTable = []struct {
	Code   string
	Result string
	Match  func(Evidence) bool
}{
	{"observed_tests_failed", "reject", func(e Evidence) bool { return e.ObservedTestsSupport == "no" }},
	{"verifier_failed", "reject", func(e Evidence) bool { return e.VerifierSupport == "no" }},
	{"external_effects_unresolved", "reject", func(e Evidence) bool { return e.ExternalEffectsResolved == "no" }},
	{"contradictory_evidence", "reject", func(e Evidence) bool { return e.Contradiction }},
	{"recommended_reject", "reject", func(e Evidence) bool { return e.RecommendedDisposition == "reject" }},
	{"high_patch_risk", "reject", func(e Evidence) bool { return e.PatchRisk == "high" }},
	{"evidence_incomplete", "review", func(e Evidence) bool { return e.EvidenceComplete == "no" }},
	{"evidence_completeness_unknown", "review", func(e Evidence) bool { return e.EvidenceComplete == "unknown" }},
	{"observed_tests_unknown", "review", func(e Evidence) bool { return e.ObservedTestsSupport == "unknown" }},
	{"verifier_unknown", "review", func(e Evidence) bool { return e.VerifierSupport == "unknown" }},
	{"cleanup_incomplete", "review", func(e Evidence) bool { return e.CleanupComplete == "no" }},
	{"cleanup_unknown", "review", func(e Evidence) bool { return e.CleanupComplete == "unknown" }},
	{"external_effects_unknown", "review", func(e Evidence) bool { return e.ExternalEffectsResolved == "unknown" }},
	{"stale_evidence", "review", func(e Evidence) bool { return e.Stale }},
	{"non_low_patch_risk", "review", func(e Evidence) bool { return e.PatchRisk == "medium" || e.PatchRisk == "unknown" }},
	{"recommended_review", "review", func(e Evidence) bool { return e.RecommendedDisposition == "review" }},
}

func EvaluatePolicy(status string, evidence Evidence) (PolicyResult, error) {
	switch status {
	case "failed":
		return PolicyResult{Result: "review", ReasonCodes: []string{"decision_failed"}}, nil
	case "not_attempted":
		return PolicyResult{Result: "review", ReasonCodes: []string{"not_attempted"}}, nil
	case "valid":
	default:
		return PolicyResult{}, fmt.Errorf("unknown decision status %q", status)
	}
	if err := validateEvidence(evidence); err != nil {
		return PolicyResult{}, err
	}

	strongest := "accept"
	reasons := make([]string, 0)
	for _, rule := range policyRuleTable {
		if !rule.Match(evidence) {
			continue
		}
		reasons = append(reasons, rule.Code)
		if rule.Result == "reject" {
			strongest = "reject"
		} else if strongest == "accept" {
			strongest = "review"
		}
	}
	if strongest == "accept" {
		if evidence.EvidenceComplete != "yes" ||
			!safeSupport(evidence.ObservedTestsSupport, evidence.Applicability.ObservedTests) ||
			!safeSupport(evidence.VerifierSupport, evidence.Applicability.Verifier) ||
			!safeSupport(evidence.CleanupComplete, evidence.Applicability.Cleanup) ||
			!safeSupport(evidence.ExternalEffectsResolved, evidence.Applicability.ExternalEffects) ||
			evidence.PatchRisk != "low" ||
			evidence.RecommendedDisposition != "accept" ||
			evidence.Contradiction || evidence.Stale {
			return PolicyResult{}, fmt.Errorf("policy invariant: incomplete safe evidence reached accept")
		}
		reasons = []string{"all_required_evidence_safe"}
	}
	sort.Strings(reasons)
	return PolicyResult{Result: strongest, ReasonCodes: reasons}, nil
}

func safeSupport(value string, applicable bool) bool {
	if applicable {
		return value == "yes"
	}
	return value == "not_applicable"
}

func validateEvidence(e Evidence) error {
	allowed := map[string]map[string]bool{
		"evidence_complete":         {"yes": true, "no": true, "unknown": true},
		"observed_tests_support":    {"yes": true, "no": true, "unknown": true, "not_applicable": true},
		"verifier_support":          {"yes": true, "no": true, "unknown": true, "not_applicable": true},
		"cleanup_complete":          {"yes": true, "no": true, "unknown": true, "not_applicable": true},
		"external_effects_resolved": {"yes": true, "no": true, "unknown": true, "not_applicable": true},
		"patch_risk":                {"low": true, "medium": true, "high": true, "unknown": true},
		"recommended_disposition":   {"accept": true, "review": true, "reject": true},
	}
	values := map[string]string{
		"evidence_complete":         e.EvidenceComplete,
		"observed_tests_support":    e.ObservedTestsSupport,
		"verifier_support":          e.VerifierSupport,
		"cleanup_complete":          e.CleanupComplete,
		"external_effects_resolved": e.ExternalEffectsResolved,
		"patch_risk":                e.PatchRisk,
		"recommended_disposition":   e.RecommendedDisposition,
	}
	for field, value := range values {
		if !allowed[field][value] {
			return fmt.Errorf("%s has invalid label %q", field, value)
		}
	}
	applicability := map[string]bool{
		"observed_tests_support":    e.Applicability.ObservedTests,
		"verifier_support":          e.Applicability.Verifier,
		"cleanup_complete":          e.Applicability.Cleanup,
		"external_effects_resolved": e.Applicability.ExternalEffects,
	}
	for field, applicable := range applicability {
		if applicable && values[field] == "not_applicable" {
			return fmt.Errorf("%s is applicable but labelled not_applicable", field)
		}
		if !applicable && values[field] != "not_applicable" {
			return fmt.Errorf("%s is inapplicable but labelled %q", field, values[field])
		}
	}
	return nil
}
