package reducer

import (
	"testing"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
)

func TestStageSkippedSkipsOnlyThatStage(t *testing.T) {
	current := approvedFirstStageState()
	order := current.Orders["o1"]
	order.Stages[1].Status = state.StageRunning
	current.Orders["o1"] = order

	next, effects, err := Reduce(current, mustStateEvent(70, ingest.EventStageSkipped, map[string]any{
		"order_id": "o1", "stage_index": 1, "hold_for_publish": true,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	got := next.Orders["o1"]
	want := []state.StageLifecycleStatus{state.StageCompleted, state.StageSkipped, state.StagePending}
	if s := stageStatuses(got); s[0] != want[0] || s[1] != want[1] || s[2] != want[2] {
		t.Fatalf("stages = %v, want %v", s, want)
	}
	if got.Status != state.OrderActive {
		t.Fatalf("order status = %q, want active while a stage is pending", got.Status)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %+v, want none", effects)
	}
}

func TestStageSkippedLastStageHeldForPublishAwaitsPublish(t *testing.T) {
	current := approvedFirstStageState()
	order := current.Orders["o1"]
	order.Stages[1].Status = state.StageCompleted
	order.Stages[2].Status = state.StageRunning
	current.Orders["o1"] = order

	next, effects, err := Reduce(current, mustStateEvent(71, ingest.EventStageSkipped, map[string]any{
		"order_id": "o1", "stage_index": 2, "hold_for_publish": true,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if got := next.Orders["o1"].Status; got != state.OrderAwaitingPublish {
		t.Fatalf("order status = %q, want %q", got, state.OrderAwaitingPublish)
	}
	assertEffectTypes(t, effects, EffectWriteProjection)
	if err := next.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestStageSkippedLastStageWithoutHoldCompletes(t *testing.T) {
	current := approvedFirstStageState()
	order := current.Orders["o1"]
	order.Stages[1].Status = state.StageCompleted
	order.Stages[2].Status = state.StageRunning
	current.Orders["o1"] = order

	next, effects, err := Reduce(current, mustStateEvent(72, ingest.EventStageSkipped, map[string]any{
		"order_id": "o1", "stage_index": 2,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if got := next.Orders["o1"].Status; got != state.OrderCompleted {
		t.Fatalf("order status = %q, want completed", got)
	}
	assertEffectTypes(t, effects, EffectWriteProjection, EffectAck)
}
