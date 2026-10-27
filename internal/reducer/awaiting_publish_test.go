package reducer

import (
	"testing"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
)

func lastStageRunningState() state.State {
	ts := fixtureTime()
	return state.State{Orders: map[string]state.OrderNode{"o1": {
		OrderID: "o1", Status: state.OrderActive, CreatedAt: ts, UpdatedAt: ts,
		Stages: []state.StageNode{
			{StageIndex: 0, Status: state.StageCompleted},
			{StageIndex: 1, Status: state.StageRunning},
		},
	}}}
}

func TestStageCompletedHeldForPublishAwaitsPublish(t *testing.T) {
	next, effects, err := Reduce(lastStageRunningState(), mustStateEvent(50, ingest.EventStageCompleted, map[string]any{
		"order_id": "o1", "stage_index": 1, "mergeable": false, "hold_for_publish": true,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if got := next.Orders["o1"].Status; got != state.OrderAwaitingPublish {
		t.Fatalf("order status = %q, want %q", got, state.OrderAwaitingPublish)
	}
	if next.Orders["o1"].Status.IsTerminal() {
		t.Fatal("awaiting-publish must not be terminal")
	}
	assertEffectTypes(t, effects, EffectWriteProjection)
	if err := next.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestStageCompletedWithoutHoldStillCompletes(t *testing.T) {
	next, _, err := Reduce(lastStageRunningState(), mustStateEvent(51, ingest.EventStageCompleted, map[string]any{
		"order_id": "o1", "stage_index": 1, "mergeable": false,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if got := next.Orders["o1"].Status; got != state.OrderCompleted {
		t.Fatalf("order status = %q, want completed", got)
	}
}

func TestOrderCompletedFinishesAwaitingPublishOrder(t *testing.T) {
	current := lastStageRunningState()
	order := current.Orders["o1"]
	order.Status = state.OrderAwaitingPublish
	order.Stages[1].Status = state.StageCompleted
	current.Orders["o1"] = order

	next, _, err := Reduce(current, mustStateEvent(52, ingest.EventOrderCompleted, map[string]any{"order_id": "o1"}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if got := next.Orders["o1"].Status; got != state.OrderCompleted {
		t.Fatalf("order status = %q, want completed", got)
	}
}
