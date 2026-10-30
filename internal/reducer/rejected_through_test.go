package reducer

import (
	"encoding/json"
	"testing"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
)

func lastStageRunningWithOutcomes(outcomes string) state.State {
	current := approvedFirstStageState()
	order := current.Orders["o1"]
	order.Stages[0].Extra = map[string]json.RawMessage{state.OutcomesExtraKey: json.RawMessage(outcomes)}
	order.Stages[1].Status = state.StageCompleted
	order.Stages[2].Status = state.StageRunning
	current.Orders["o1"] = order
	return current
}

func TestLastStageCompletedFailsOrderWithRejectLetThrough(t *testing.T) {
	for _, hold := range []bool{false, true} {
		current := lastStageRunningWithOutcomes(`[{"state":"reject","action":"promote","feedback":"no tests"}]`)
		next, effects, err := Reduce(current, mustStateEvent(80, ingest.EventStageCompleted, map[string]any{
			"order_id": "o1", "stage_index": 2, "hold_for_publish": hold,
		}, fixtureTime()))
		if err != nil {
			t.Fatalf("reduce: %v", err)
		}
		if got := next.Orders["o1"].Status; got != state.OrderFailed {
			t.Fatalf("hold=%v: order status = %q, want failed", hold, got)
		}
		assertEffectTypes(t, effects, EffectWriteProjection, EffectAck)
	}
}

func TestLastStageCompletedIgnoresSupersededReject(t *testing.T) {
	for _, outcomes := range []string{
		`[{"state":"reject","action":"retry"},{"state":"accept","action":"promote"}]`,
		`[{"state":"reject","action":"retry-previous"}]`,
		`[{"state":"reject","action":"park"}]`,
	} {
		current := lastStageRunningWithOutcomes(outcomes)
		next, _, err := Reduce(current, mustStateEvent(81, ingest.EventStageCompleted, map[string]any{
			"order_id": "o1", "stage_index": 2,
		}, fixtureTime()))
		if err != nil {
			t.Fatalf("reduce: %v", err)
		}
		if got := next.Orders["o1"].Status; got != state.OrderCompleted {
			t.Fatalf("%s: order status = %q, want completed", outcomes, got)
		}
	}
}

func TestSkipEndsOrderFailedWithRejectLetThrough(t *testing.T) {
	current := lastStageRunningWithOutcomes(`[{"state":"reject","action":"exit"}]`)
	next, _, err := Reduce(current, mustStateEvent(82, ingest.EventStageSkipped, map[string]any{
		"order_id": "o1", "stage_index": 2,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if got := next.Orders["o1"].Status; got != state.OrderFailed {
		t.Fatalf("order status = %q, want failed", got)
	}
}
