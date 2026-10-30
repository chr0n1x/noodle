package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/worktree"
)

const reworkExtraKey = "rework_round"

func reworkExtra(round int) map[string]json.RawMessage {
	return map[string]json.RawMessage{reworkExtraKey: json.RawMessage(fmt.Sprint(round))}
}

// changesRequestedExtraKey counts the human request-changes re-runs of a
// stage, which reuse the stage instead of inserting one.
const changesRequestedExtraKey = "changes_requested"

func changesRequested(s Stage) int {
	var n int
	_ = json.Unmarshal(s.Extra[changesRequestedExtraKey], &n)
	return n
}

// reworkRounds counts rework inserted for rejected external reviews plus
// request-changes re-runs; both share the [review] max_rework budget.
func reworkRounds(order Order) int {
	rounds := 0
	for _, s := range order.Stages {
		if _, ok := s.Extra[reworkExtraKey]; ok {
			rounds++
		}
		rounds += changesRequested(s)
	}
	return rounds
}

// startRework handles a rejected external review by inserting an execute stage
// with the review as its prompt, then a copy of the review stage, and
// advancing to the new execute stage. Returns the round started, or 0 when the
// [review] max_rework limit is reached or no execute stage precedes the review.
func (l *Loop) startRework(cook *cookHandle, review string) (round, limit int, err error) {
	return l.reworkFrom(cook, review, func(s Stage) bool { return s.TaskKey == "execute" })
}

// reworkFrom is startRework for the nearest earlier stage matching target: a
// copy of that stage runs with the review as its prompt, then a copy of the
// rejecting stage.
func (l *Loop) reworkFrom(cook *cookHandle, review string, target func(Stage) bool) (round, limit int, err error) {
	limit = l.config.Review.MaxReworkRounds()
	err = l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		for i := range orders.Orders {
			order := &orders.Orders[i]
			if order.ID != cook.orderID || cook.stageIndex >= len(order.Stages) {
				continue
			}
			if reworkRounds(*order) >= limit {
				return false, nil
			}
			round = reworkRounds(*order) + 1
			held := order.Stages[cook.stageIndex]
			var execute Stage
			found := false
			for j := cook.stageIndex - 1; j >= 0 && !found; j-- {
				if prev := order.Stages[j]; target(prev) {
					execute = Stage{
						TaskKey: prev.TaskKey, Skill: nonEmpty(prev.Skill, prev.TaskKey), Status: StageStatusPending, Extra: reworkExtra(round),
						Provider: prev.Provider, Model: prev.Model, Runtime: prev.Runtime,
					}
					found = true
				}
			}
			if !found {
				round = 0
				return false, nil
			}
			execute.Prompt = fmt.Sprintf("Rework round %d of %d: the %s review rejected the previous work. Address this feedback, verify again, and commit:\n\n%s",
				round, limit, held.TaskKey, strings.TrimSpace(review))
			again := Stage{TaskKey: held.TaskKey, Skill: held.Skill, Prompt: held.Prompt, Provider: held.Provider, Model: held.Model, Runtime: held.Runtime, Status: StageStatusPending}
			if isAutoQuality(held) {
				again.Extra = map[string]json.RawMessage{autoQualityExtraKey: held.Extra[autoQualityExtraKey]}
			}
			rest := append([]Stage{execute, again}, order.Stages[cook.stageIndex+1:]...)
			order.Stages = append(order.Stages[:cook.stageIndex+1], rest...)
			return true, nil
		}
		return false, nil
	})
	if err != nil || round == 0 {
		return 0, limit, err
	}

	if _, kept := l.orderWorkBranch(cook.orderID); !kept && strings.TrimSpace(cook.worktreeName) != "" {
		_ = l.deps.Worktree.Cleanup(cook.worktreeName, worktree.CleanupOpts{Force: true})
	}
	msg := fmt.Sprintf("review rejected; rework round %d of %d queued", round, limit)
	source := "external "
	if !l.config.Review.IsExternal(cook.stage.TaskKey, cook.stage.Skill) || isAutoQuality(cook.stage) {
		source = cook.stage.TaskKey + " "
	}
	if err := l.emitEventChecked(ingest.EventStageReviewApproved, map[string]any{
		"order_id":    cook.orderID,
		"stage_index": cook.stageIndex,
		"reason":      source + msg,
	}); err != nil {
		return 0, limit, err
	}
	if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, false)); err != nil {
		return 0, limit, err
	}
	msg = fmt.Sprintf("review rejected; rework round %d of %d queued", round, limit)
	if err := l.advanceAndPersist(context.Background(), cook, &msg); err != nil {
		return 0, limit, err
	}
	return round, limit, l.syncPendingReviewProjection()
}
