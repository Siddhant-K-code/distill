package studylocal

import (
	"fmt"
	"sort"
)

func BuildSchedule(corpus Corpus, protocol Protocol, contexts []Context) ([]ScheduleEntry, error) {
	if err := ValidateCorpus(corpus); err != nil {
		return nil, err
	}
	if err := ValidateProtocol(protocol); err != nil {
		return nil, err
	}
	if err := ValidateContexts(corpus, contexts); err != nil {
		return nil, err
	}
	contextByKey := make(map[string]Context, len(contexts))
	for _, context := range contexts {
		contextByKey[context.ConditionID+"\x00"+context.Arm] = context
	}
	conditionByID := make(map[string]Condition, len(corpus.Conditions))
	for _, condition := range corpus.Conditions {
		conditionByID[condition.ID] = condition
	}
	modelContractHash, _ := DigestDomain("model-contract", struct {
		ID, Revision, Format, ArtifactSHA256 string
		Generation                           GenerationConfig
	}{TargetModelID, TargetModelRevision, TargetModelFormat, TargetModelArtifactSHA256, protocol.Generation})

	orderedConditions := append([]Condition(nil), corpus.Conditions...)
	sort.Slice(orderedConditions, func(i, j int) bool {
		left := DigestBytes([]byte(ConditionOrderSeed + "\x00" + orderedConditions[i].ID))
		right := DigestBytes([]byte(ConditionOrderSeed + "\x00" + orderedConditions[j].ID))
		if left == right {
			return orderedConditions[i].ID < orderedConditions[j].ID
		}
		return left < right
	})
	repeated := map[string]bool{}
	for _, id := range repeatConditionIDs {
		if _, ok := conditionByID[id]; !ok {
			return nil, fmt.Errorf("repeat subset references unknown condition %q", id)
		}
		repeated[id] = true
	}
	var schedule []ScheduleEntry
	appendBlock := func(condition Condition, replicate int) {
		arms := []string{ArmRaw, ArmDistillLock}
		if DigestBytes([]byte(ArmOrderSeed + "\x00" + condition.ID + "\x00" + fmt.Sprint(replicate)))[0]%2 == 1 {
			arms[0], arms[1] = arms[1], arms[0]
		}
		for _, arm := range arms {
			index := len(schedule) + 1
			schedule = append(schedule, ScheduleEntry{
				Index: index, ObservationID: fmt.Sprintf("local-observation-%03d", index),
				ConditionID: condition.ID, BaseID: condition.BaseID, Dataset: condition.Dataset,
				Arm: arm, Replicate: replicate, RepeatSubset: repeated[condition.ID],
				ContextSHA256: contextByKey[condition.ID+"\x00"+arm].ContentSHA256,
				CorpusSHA256:  corpus.CorpusSHA256, ProtocolSHA256: protocol.ProtocolSHA256,
				ModelContractHash: modelContractHash,
			})
		}
	}
	for _, condition := range orderedConditions {
		appendBlock(condition, 1)
	}
	for replicate := 2; replicate <= 3; replicate++ {
		for _, condition := range orderedConditions {
			if repeated[condition.ID] {
				appendBlock(condition, replicate)
			}
		}
	}
	if err := ValidateSchedule(corpus, protocol, contexts, schedule); err != nil {
		return nil, err
	}
	return schedule, nil
}

func ValidateSchedule(corpus Corpus, protocol Protocol, contexts []Context, schedule []ScheduleEntry) error {
	if len(schedule) != ObservationsPerModel {
		return fmt.Errorf("expected %d observations, got %d", ObservationsPerModel, len(schedule))
	}
	conditions := map[string]Condition{}
	for _, condition := range corpus.Conditions {
		conditions[condition.ID] = condition
	}
	contextByKey := map[string]Context{}
	for _, context := range contexts {
		contextByKey[context.ConditionID+"\x00"+context.Arm] = context
	}
	repeated := map[string]bool{}
	for _, id := range repeatConditionIDs {
		repeated[id] = true
	}
	counts := map[string]int{}
	seenObservations := map[string]bool{}
	primary, repeat := 0, 0
	for i, entry := range schedule {
		condition, ok := conditions[entry.ConditionID]
		key := entry.ConditionID + "\x00" + entry.Arm
		if !ok || entry.Index != i+1 || entry.ObservationID != fmt.Sprintf("local-observation-%03d", i+1) ||
			seenObservations[entry.ObservationID] || entry.BaseID != condition.BaseID ||
			entry.Dataset != condition.Dataset || (entry.Arm != ArmRaw && entry.Arm != ArmDistillLock) ||
			entry.ContextSHA256 != contextByKey[key].ContentSHA256 ||
			entry.CorpusSHA256 != corpus.CorpusSHA256 || entry.ProtocolSHA256 != protocol.ProtocolSHA256 ||
			entry.RepeatSubset != repeated[entry.ConditionID] {
			return fmt.Errorf("invalid schedule entry %d", i+1)
		}
		seenObservations[entry.ObservationID] = true
		counts[key]++
		if entry.Replicate == 1 {
			primary++
		} else if entry.Replicate == 2 || entry.Replicate == 3 {
			repeat++
		} else {
			return fmt.Errorf("invalid replicate at %d", i+1)
		}
	}
	if primary != PrimaryObservationCount || repeat != RepeatObservationCount {
		return fmt.Errorf("schedule allocation mismatch: primary=%d repeat=%d", primary, repeat)
	}
	for conditionID := range conditions {
		for _, arm := range []string{ArmRaw, ArmDistillLock} {
			want := 1
			if repeated[conditionID] {
				want = 3
			}
			if counts[conditionID+"\x00"+arm] != want {
				return fmt.Errorf("replicate count mismatch for %s/%s", conditionID, arm)
			}
		}
	}
	return nil
}
