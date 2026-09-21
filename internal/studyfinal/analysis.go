package studyfinal

import (
	"fmt"
	"math"
	"sort"
)

type CalibrationObservation struct {
	Confidence float64
	Correct    bool
}

type ECEBin struct {
	Lower          float64 `json:"lower"`
	Upper          float64 `json:"upper"`
	Count          int     `json:"count"`
	MeanConfidence float64 `json:"mean_confidence"`
	Accuracy       float64 `json:"accuracy"`
}

func ECE(observations []CalibrationObservation, bins int) (float64, []ECEBin, error) {
	if bins <= 0 {
		return 0, nil, fmt.Errorf("bins must be positive")
	}
	out := make([]ECEBin, bins)
	for i := range out {
		out[i].Lower = float64(i) / float64(bins)
		out[i].Upper = float64(i+1) / float64(bins)
	}
	correct := make([]int, bins)
	for _, o := range observations {
		if math.IsNaN(o.Confidence) || math.IsInf(o.Confidence, 0) || o.Confidence < 0 || o.Confidence > 1 {
			return 0, nil, fmt.Errorf("confidence outside [0,1]")
		}
		index := int(math.Floor(o.Confidence * float64(bins)))
		if index == bins {
			index = bins - 1
		}
		out[index].Count++
		out[index].MeanConfidence += o.Confidence
		if o.Correct {
			correct[index]++
		}
	}
	ece := 0.0
	for i := range out {
		if out[i].Count == 0 {
			continue
		}
		out[i].MeanConfidence /= float64(out[i].Count)
		out[i].Accuracy = float64(correct[i]) / float64(out[i].Count)
		ece += float64(out[i].Count) / float64(len(observations)) * math.Abs(out[i].Accuracy-out[i].MeanConfidence)
	}
	return ece, out, nil
}

func Brier(probabilities map[string]float64, observed string) (float64, error) {
	if len(probabilities) == 0 {
		return 0, fmt.Errorf("empty probabilities")
	}
	sum, score := 0.0, 0.0
	if _, ok := probabilities[observed]; !ok {
		return 0, fmt.Errorf("observed label absent")
	}
	for label, p := range probabilities {
		if math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
			return 0, fmt.Errorf("invalid probability")
		}
		sum += p
		target := 0.0
		if label == observed {
			target = 1
		}
		score += (p - target) * (p - target)
	}
	if math.Abs(sum-1) > 1e-9 {
		return 0, fmt.Errorf("probabilities do not sum to one")
	}
	return score, nil
}

func LogLoss(probabilities map[string]float64, observed string) (float64, error) {
	p, ok := probabilities[observed]
	if !ok || math.IsNaN(p) || math.IsInf(p, 0) || p < 0 || p > 1 {
		return 0, fmt.Errorf("invalid observed-label probability")
	}
	return -math.Log(math.Max(p, 1e-15)), nil
}

type ScoredDecision struct {
	ID     string
	Score  *float64
	Unsafe bool
}

type RiskPoint struct {
	Threshold float64 `json:"threshold"`
	Accepted  int     `json:"accepted"`
	Total     int     `json:"total"`
	Unsafe    int     `json:"unsafe"`
	Coverage  float64 `json:"coverage"`
	Risk      float64 `json:"risk"`
}

func SelectiveRisk(decisions []ScoredDecision, grid []float64) ([]RiskPoint, error) {
	points := make([]RiskPoint, 0, len(grid))
	for _, threshold := range grid {
		if threshold < 0 || threshold > 1 || math.IsNaN(threshold) {
			return nil, fmt.Errorf("invalid threshold")
		}
		p := RiskPoint{Threshold: threshold, Total: len(decisions)}
		for _, d := range decisions {
			if d.Score == nil {
				continue
			}
			if *d.Score < 0 || *d.Score > 1 || math.IsNaN(*d.Score) {
				return nil, fmt.Errorf("invalid score")
			}
			if *d.Score >= threshold {
				p.Accepted++
				if d.Unsafe {
					p.Unsafe++
				}
			}
		}
		if p.Total > 0 {
			p.Coverage = float64(p.Accepted) / float64(p.Total)
		}
		if p.Accepted > 0 {
			p.Risk = float64(p.Unsafe) / float64(p.Accepted)
		}
		points = append(points, p)
	}
	return points, nil
}

func ExactConsistency(decisions []Decision) (float64, error) {
	if len(decisions) == 0 {
		return 0, fmt.Errorf("no decisions")
	}
	first, err := DigestDomain("analysis-decision", decisions[0])
	if err != nil {
		return 0, err
	}
	matches := 0
	for _, decision := range decisions {
		d, err := DigestDomain("analysis-decision", decision)
		if err != nil {
			return 0, err
		}
		if d == first {
			matches++
		}
	}
	return float64(matches) / float64(len(decisions)), nil
}

// OneSidedUnsafeUpper returns the one-sided Clopper-Pearson 95% upper bound.
func OneSidedUnsafeUpper(unsafe, accepted int) (float64, error) {
	if accepted <= 0 || unsafe < 0 || unsafe > accepted {
		return 0, fmt.Errorf("invalid binomial counts")
	}
	if unsafe == accepted {
		return 1, nil
	}
	const alpha = 0.05
	lo, hi := 0.0, 1.0
	for i := 0; i < 100; i++ {
		mid := (lo + hi) / 2
		cdf := binomialCDF(unsafe, accepted, mid)
		if cdf > alpha {
			lo = mid
		} else {
			hi = mid
		}
	}
	return (lo + hi) / 2, nil
}

func binomialCDF(k, n int, p float64) float64 {
	if p <= 0 {
		return 1
	}
	if p >= 1 {
		if k == n {
			return 1
		}
		return 0
	}
	term := math.Pow(1-p, float64(n))
	sum := term
	for i := 0; i < k; i++ {
		term *= float64(n-i) / float64(i+1) * p / (1 - p)
		sum += term
	}
	return sum
}

type ThresholdSelection struct {
	Threshold float64 `json:"threshold"`
	Accepted  int     `json:"accepted"`
	Unsafe    int     `json:"unsafe"`
	Upper95   float64 `json:"upper_95"`
	Coverage  float64 `json:"coverage"`
}

func SelectThreshold(d Dataset, baseAllowlist []string, decisions []ScoredDecision, grid []float64, ceiling float64, minimumAccepted int) (*ThresholdSelection, error) {
	if ceiling != 0.15 || minimumAccepted != 25 {
		return nil, fmt.Errorf("final study freezes ceiling 0.15 and minimum accepted 25")
	}
	if err := requireIndependentDecisions(d, baseAllowlist, decisions); err != nil {
		return nil, err
	}
	points, err := SelectiveRisk(decisions, grid)
	if err != nil {
		return nil, err
	}
	candidates := make([]ThresholdSelection, 0)
	for _, p := range points {
		if p.Accepted < minimumAccepted {
			continue
		}
		upper, err := OneSidedUnsafeUpper(p.Unsafe, p.Accepted)
		if err != nil {
			return nil, err
		}
		if upper <= ceiling {
			candidates = append(candidates, ThresholdSelection{p.Threshold, p.Accepted, p.Unsafe, upper, p.Coverage})
		}
	}
	if len(candidates) == 0 {
		return nil, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].Coverage == candidates[j].Coverage {
			return candidates[i].Threshold > candidates[j].Threshold
		}
		return candidates[i].Coverage > candidates[j].Coverage
	})
	return &candidates[0], nil
}

func QualifyThreshold(d Dataset, baseAllowlist []string, decisions []ScoredDecision, selected ThresholdSelection, ceiling float64, minimumAccepted int) (bool, ThresholdSelection, error) {
	if err := requireIndependentDecisions(d, baseAllowlist, decisions); err != nil {
		return false, ThresholdSelection{}, err
	}

	points, err := SelectiveRisk(decisions, []float64{selected.Threshold})
	if err != nil {
		return false, ThresholdSelection{}, err
	}

	p := points[0]
	result := ThresholdSelection{Threshold: p.Threshold, Accepted: p.Accepted, Unsafe: p.Unsafe, Coverage: p.Coverage}
	if p.Accepted == 0 {
		return false, result, nil
	}
	result.Upper95, err = OneSidedUnsafeUpper(p.Unsafe, p.Accepted)
	if err != nil {
		return false, result, err
	}
	return p.Accepted >= minimumAccepted && result.Upper95 <= ceiling, result, nil
}

// EvaluateHeldOut applies a previously frozen threshold without searching or retuning.
func EvaluateHeldOut(d Dataset, baseAllowlist []string, decisions []ScoredDecision, selected ThresholdSelection) (bool, ThresholdSelection, error) {
	return QualifyThreshold(d, baseAllowlist, decisions, selected, 0.15, 25)
}

func requireIndependentDecisions(d Dataset, baseAllowlist []string, decisions []ScoredDecision) error {
	known, allowed := map[string]bool{}, map[string]bool{}
	for _, base := range d.Bases {
		known[base.ID] = true
	}
	for _, id := range baseAllowlist {
		if !known[id] || allowed[id] {
			return fmt.Errorf("invalid base allowlist")
		}
		allowed[id] = true
	}
	seen := map[string]bool{}
	for _, decision := range decisions {
		if decision.ID == "" || seen[decision.ID] || !allowed[decision.ID] {
			return fmt.Errorf("threshold decisions must use unique allowlisted base IDs")
		}
		seen[decision.ID] = true
	}
	return nil
}
