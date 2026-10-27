package reducer

import (
	"fmt"
	"strconv"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
)

func init() {
	reduceHandlers[ingest.EventStageRemainingSkipped] = reduceStageRemainingSkipped
}

// reduceStageRemainingSkipped sets every non-terminal stage from stage_index
// on to skipped, then completes the order (or holds it for publish) when all
// its stages are terminal, as reduceStageCompleted does.
func reduceStageRemainingSkipped(current state.State, event ingest.StateEvent) (state.State, []Effect, error) {
	var payload orderStagePayload
	if err := decodeEventPayload(event, &payload); err != nil {
		return current, nil, fmt.Errorf("reduce stage_remaining_skipped: %w", err)
	}
	order, ok := current.Orders[payload.OrderID]
	if !ok || order.Status.IsTerminal() || payload.StageIndex < 0 {
		return current, nil, nil
	}

	next := current.Clone()
	order = next.Orders[payload.OrderID]
	for i := payload.StageIndex; i < len(order.Stages); i++ {
		if !order.Stages[i].Status.IsTerminal() {
			order.Stages[i].Status = state.StageSkipped
		}
	}
	order.UpdatedAt = event.Timestamp
	if allStagesTerminal(order) {
		order.Status = state.OrderCompleted
		if payload.HoldForPublish {
			order.Status = state.OrderAwaitingPublish
		}
	}
	next.Orders[payload.OrderID] = order
	next.LastEventID = strconv.FormatUint(uint64(event.ID), 10)

	switch order.Status {
	case state.OrderAwaitingPublish:
		effect, err := makeEffect(event, 0, EffectWriteProjection, map[string]any{"order_id": payload.OrderID})
		if err != nil {
			return current, nil, fmt.Errorf("reduce stage_remaining_skipped projection: %w", err)
		}
		return next, []Effect{effect}, nil
	case state.OrderCompleted:
		e0, err := makeEffect(event, 0, EffectWriteProjection, map[string]any{"order_id": payload.OrderID})
		if err != nil {
			return current, nil, fmt.Errorf("reduce stage_remaining_skipped projection: %w", err)
		}
		e1, err := makeEffect(event, 1, EffectAck, map[string]any{"order_id": payload.OrderID})
		if err != nil {
			return current, nil, fmt.Errorf("reduce stage_remaining_skipped ack: %w", err)
		}
		return next, []Effect{e0, e1}, nil
	}
	return next, nil, nil
}
