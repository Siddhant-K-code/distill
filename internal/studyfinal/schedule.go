package studyfinal

import (
	"fmt"
	"sort"
	"strings"
)

var frozenThresholdGrid = []float64{0.50, 0.60, 0.70, 0.80, 0.90, 0.95}

func ThresholdGrid() []float64 {
	return append([]float64(nil), frozenThresholdGrid...)
}

func repeatTarget(role string) int {
	switch role {
	case "threshold_development":
		return 15
	case "safety_calibration":
		return 8
	case "held_out", "confirmatory_secondary":
		return 7
	default:
		return 0
	}
}

func BuildSchedule(d Dataset, seed string) ([]ScheduleEntry, error) {
	return buildSchedule(d, seed, productionAuthorizationEnvironment())
}

func buildSchedule(d Dataset, seed string, env authorizationEnvironment) ([]ScheduleEntry, error) {
	if err := validateCorpus(d, env); err != nil {
		return nil, err
	}
	if seed == "" {
		seed = DefaultScheduleSeed
	}
	questionDigest, _ := DigestDomain("question-schema", AtomicQuestions())
	decisionDigest, _ := DigestDomain("decision-system", struct {
		Model, Policy string
	}{JevModel, "final-policy-v1"})
	thresholdDigest, _ := DigestDomain("threshold-set", ThresholdGrid())
	contexts := make(map[string]ContextArtifact, 300)
	type pair struct {
		condition Condition
		arm       string
		score     string
	}
	groups := map[string][]pair{}
	for _, c := range d.Conditions {
		for _, arm := range []string{ArmRaw, ArmDistillLock} {
			ctx, err := CompileContext(c, arm)
			if err != nil {
				return nil, err
			}
			key := c.ID + "\x00" + arm
			contexts[key] = ctx
			group := c.RepositoryRole + "\x00" + arm
			groups[group] = append(groups[group], pair{c, arm, digestBytes([]byte(seed + "\x00" + group + "\x00" + c.ID))})
		}
	}
	repeat := map[string]bool{}
	if len(groups) != 6 {
		return nil, fmt.Errorf("expected six repository-role/arm strata")
	}
	for _, members := range groups {
		sort.Slice(members, func(i, j int) bool {
			if members[i].score == members[j].score {
				return members[i].condition.ID < members[j].condition.ID
			}
			return members[i].score < members[j].score
		})
		role := members[0].condition.RepositoryRole
		target := repeatTarget(role)
		if target == 0 || len(members) < target {
			return nil, fmt.Errorf("repeatability stratum too small")
		}
		for _, p := range members[:target] {
			repeat[p.condition.ID+"\x00"+p.arm] = true
		}
	}
	conditions := append([]Condition(nil), d.Conditions...)
	sort.Slice(conditions, func(i, j int) bool {
		if conditions[i].Split == conditions[j].Split {
			return conditions[i].ID < conditions[j].ID
		}
		return splitOrder(conditions[i].Split) < splitOrder(conditions[j].Split)
	})
	out := make([]ScheduleEntry, 0, 420)
	appendEntry := func(c Condition, arm string, replicate int, isRepeat bool) {
		ctx := contexts[c.ID+"\x00"+arm]
		index := len(out) + 1
		callID := fmt.Sprintf("final-call-%03d-%s-r%d", index, arm, replicate)
		out = append(out, ScheduleEntry{
			Index: index, CallID: callID, ConditionID: c.ID, RepositoryRole: c.RepositoryRole, Split: c.Split,
			Arm: arm, Replicate: replicate, Repeatability: isRepeat, ContextSHA256: ctx.SHA256,
			CorpusSHA256: d.CorpusSHA256, QuestionSHA256: questionDigest, DecisionSHA256: decisionDigest,
			ThresholdSHA256: thresholdDigest,
		})
	}
	finalSplit := "held_out_repository"
	if d.AgentTraceContamination.Status == AgentTraceDowngraded {
		finalSplit = "confirmatory_secondary"
	}
	for _, split := range []string{"threshold-development", "safety-calibration", finalSplit} {
		for _, c := range conditions {
			if c.Split != split {
				continue
			}
			for _, arm := range []string{ArmRaw, ArmDistillLock} {
				appendEntry(c, arm, 1, repeat[c.ID+"\x00"+arm])
			}
		}
		for _, c := range conditions {
			if c.Split != split {
				continue
			}
			for _, arm := range []string{ArmRaw, ArmDistillLock} {
				if repeat[c.ID+"\x00"+arm] {
					appendEntry(c, arm, 2, true)
					appendEntry(c, arm, 3, true)
				}
			}
		}
	}
	if err := validateSchedule(d, out, env); err != nil {
		return nil, err
	}
	return out, nil
}

func splitOrder(split string) int {
	switch split {
	case "threshold-development":
		return 0
	case "safety-calibration":
		return 1
	case "held_out_repository":
		return 2
	case "confirmatory_secondary":
		return 2
	default:
		return 99
	}
}

func ValidateSchedule(d Dataset, schedule []ScheduleEntry) error {
	return validateSchedule(d, schedule, productionAuthorizationEnvironment())
}

func validateSchedule(d Dataset, schedule []ScheduleEntry, env authorizationEnvironment) error {
	if err := validateCorpus(d, env); err != nil {
		return err
	}
	if len(schedule) != 420 {
		return fmt.Errorf("expected 420 calls, got %d", len(schedule))
	}
	conditions := map[string]Condition{}
	for _, c := range d.Conditions {
		conditions[c.ID] = c
	}
	counts := map[string]int{}
	replicates := map[string]map[int]bool{}
	strata := map[string]int{}
	questionDigest, _ := DigestDomain("question-schema", AtomicQuestions())
	decisionDigest, _ := DigestDomain("decision-system", struct {
		Model, Policy string
	}{JevModel, "final-policy-v1"})
	thresholdDigest, _ := DigestDomain("threshold-set", ThresholdGrid())
	lastSplitOrder := -1
	for i, s := range schedule {
		c, ok := conditions[s.ConditionID]
		if !ok || s.Index != i+1 || s.CallID == "" || s.CorpusSHA256 != d.CorpusSHA256 {
			return fmt.Errorf("invalid schedule entry %d", i+1)
		}
		if s.RepositoryRole != c.RepositoryRole || s.Split != c.Split || (s.Arm != ArmRaw && s.Arm != ArmDistillLock) || s.Replicate < 1 || s.Replicate > 3 {
			return fmt.Errorf("schedule identity mismatch at %d", i+1)
		}
		order := splitOrder(s.Split)
		if order < lastSplitOrder {
			return fmt.Errorf("schedule violates phase order at %d", i+1)
		}
		lastSplitOrder = order
		context, err := CompileContext(c, s.Arm)
		if err != nil || context.SHA256 != s.ContextSHA256 || s.QuestionSHA256 != questionDigest ||
			s.DecisionSHA256 != decisionDigest || s.ThresholdSHA256 != thresholdDigest {
			return fmt.Errorf("schedule digest mismatch at %d", i+1)
		}
		key := s.ConditionID + "\x00" + s.Arm
		if replicates[key] == nil {
			replicates[key] = map[int]bool{}
		}
		if replicates[key][s.Replicate] {
			return fmt.Errorf("duplicate replicate for %s", key)
		}
		if s.Replicate > 1 && !replicates[key][s.Replicate-1] {
			return fmt.Errorf("replicate order violation for %s", key)
		}
		replicates[key][s.Replicate] = true
		counts[key]++
	}
	repeated := 0
	for key, count := range counts {
		if count == 3 {
			repeated++
			var arm string
			for _, s := range schedule {
				if s.ConditionID+"\x00"+s.Arm == key {
					arm = s.Arm
					strata[s.RepositoryRole+"\x00"+arm]++
					break
				}
			}
		} else if count != 1 {
			return fmt.Errorf("pair %q has %d calls", key, count)
		}
	}
	if len(counts) != 300 || repeated != 60 {
		return fmt.Errorf("expected 300 pairs and 60 repeated, got %d and %d", len(counts), repeated)
	}
	for stratum, count := range strata {
		role := strings.SplitN(stratum, "\x00", 2)[0]
		if count != repeatTarget(role) {
			return fmt.Errorf("repeatability stratum %s has %d pairs", stratum, count)
		}
	}
	if len(strata) != 6 {
		return fmt.Errorf("expected six repeatability strata")
	}
	return nil
}
