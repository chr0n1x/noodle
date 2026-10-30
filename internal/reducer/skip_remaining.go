package reducer

import (
	"fmt"
	"strconv"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
)

func init() {
	reduceHandlers[ingest.EventStageRemainingSkipped] = reduceStageRemainingSkipped
	reduceHandlers[ingest.EventStageSkipped] = reduceStageSkipped
}

// reduceStageRemainingSkipped sets every non-terminal stage from stage_index
// on to skipped, then completes the order (or holds it for publish) when all
// its stages are terminal, as reduceStageCompleted does.
func reduceStageRemainingSkipped(current state.State, event ingest.StateEvent) (state.State, []Effect, error) {
	return reduceSkip(current, event, "stage_remaining_skipped", false)
}

// reduceStageSkipped sets only the stage at stage_index to skipped, so later
// stages still run, then completes the order (or holds it for publish) when
// all its stages are terminal, as reduceStageCompleted does.
func reduceStageSkipped(current state.State, event ingest.StateEvent) (state.State, []Effect, error) {
	return reduceSkip(current, event, "stage_skipped", true)
}

func reduceSkip(current state.State, event ingest.StateEvent, name string, onlyOne bool) (state.State, []Effect, error) {
	var payload orderStagePayload
	if err := decodeEventPayload(event, &payload); err != nil {
		return current, nil, fmt.Errorf("reduce %s: %w", name, err)
	}
	order, ok := current.Orders[payload.OrderID]
	if !ok || order.Status.IsTerminal() || payload.StageIndex < 0 {
		return current, nil, nil
	}
	end := len(order.Stages)
	if onlyOne {
		if payload.StageIndex >= end {
			return current, nil, nil
		}
		end = payload.StageIndex + 1
	}

	next := current.Clone()
	order = next.Orders[payload.OrderID]
	for i := payload.StageIndex; i < end; i++ {
		if !order.Stages[i].Status.IsTerminal() {
			order.Stages[i].Status = state.StageSkipped
		}
	}
	order.UpdatedAt = event.Timestamp
	if allStagesTerminal(order) {
		order.Status = endStatus(order, payload.HoldForPublish)
	}
	next.Orders[payload.OrderID] = order
	next.LastEventID = strconv.FormatUint(uint64(event.ID), 10)

	switch order.Status {
	case state.OrderAwaitingPublish:
		effect, err := makeEffect(event, 0, EffectWriteProjection, map[string]any{"order_id": payload.OrderID})
		if err != nil {
			return current, nil, fmt.Errorf("reduce %s projection: %w", name, err)
		}
		return next, []Effect{effect}, nil
	case state.OrderCompleted, state.OrderFailed:
		e0, err := makeEffect(event, 0, EffectWriteProjection, map[string]any{"order_id": payload.OrderID})
		if err != nil {
			return current, nil, fmt.Errorf("reduce %s projection: %w", name, err)
		}
		e1, err := makeEffect(event, 1, EffectAck, map[string]any{"order_id": payload.OrderID})
		if err != nil {
			return current, nil, fmt.Errorf("reduce %s ack: %w", name, err)
		}
		return next, []Effect{e0, e1}, nil
	}
	return next, nil, nil
}
