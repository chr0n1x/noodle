package loop

import (
	"context"
	"encoding/json"
	"fmt"
)

// autoQualityExtraKey marks a quality stage the loop inserted for
// auto_verify_quality; it always runs as a cook, even when quality is listed
// in external_stages.
const autoQualityExtraKey = "auto_quality"

func isAutoQuality(s Stage) bool {
	_, ok := s.Extra[autoQualityExtraKey]
	return ok
}

// autoQualityOn reports whether [review] auto_verify_quality applies.
func (l *Loop) autoQualityOn() bool {
	return l.config.Review.AutoVerifyQuality && l.orderGated()
}

// insertAutoQuality adds a quality stage right after the completed execute
// stage, unless a pending quality stage already follows it before the next
// execute, then adds [review] extra_stages after it.
func (l *Loop) insertAutoQuality(cook *cookHandle) error {
	return l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		for i := range orders.Orders {
			order := &orders.Orders[i]
			if order.ID != cook.orderID || cook.stageIndex >= len(order.Stages) {
				continue
			}
			for _, s := range order.Stages[cook.stageIndex+1:] {
				if s.TaskKey == "execute" {
					break
				}
				if s.TaskKey == "quality" && s.Status == StageStatusPending {
					return false, nil
				}
			}
			execute := order.Stages[cook.stageIndex]
			quality := Stage{TaskKey: "quality", Skill: "quality", Provider: execute.Provider, Model: execute.Model, Runtime: execute.Runtime,
				Status: StageStatusPending, Extra: map[string]json.RawMessage{autoQualityExtraKey: json.RawMessage("true")}}
			rest := append([]Stage{quality}, order.Stages[cook.stageIndex+1:]...)
			order.Stages = append(order.Stages[:cook.stageIndex+1], rest...)
			withExtras, unknown := applyExtraStages(OrdersFile{Orders: []Order{*order}}, l.config.Review.ExtraStages, func(name string) bool {
				_, ok := l.registry.ByKey(name)
				return ok
			})
			for _, name := range unknown {
				l.logger.Warn("review extra stage skipped: no skill registered", "stage", name)
			}
			*order = withExtras.Orders[0]
			return true, nil
		}
		return false, nil
	})
}

// rejectQuality sends a rejecting quality cook's findings back to execute as
// rework, failing the order once max_rework rounds are used.
func (l *Loop) rejectQuality(ctx context.Context, cook *cookHandle, review string) error {
	l.forwardToScheduler(cook, "stage_message_blocked", review, nil)
	round, limit, err := l.startRework(cook, review)
	if err != nil || round > 0 {
		return err
	}
	return l.failStage(ctx, cook, fmt.Sprintf("quality rejected; rework limit (%d) reached: %s", limit, review))
}
