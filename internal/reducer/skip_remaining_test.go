package reducer

import (
	"testing"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
)

func approvedFirstStageState() state.State {
	ts := fixtureTime()
	return state.State{Orders: map[string]state.OrderNode{"o1": {
		OrderID: "o1", Status: state.OrderActive, CreatedAt: ts, UpdatedAt: ts,
		Stages: []state.StageNode{
			{StageIndex: 0, Status: state.StageCompleted},
			{StageIndex: 1, Status: state.StagePending},
			{StageIndex: 2, Status: state.StagePending},
		},
	}}}
}

func stageStatuses(order state.OrderNode) []state.StageLifecycleStatus {
	out := make([]state.StageLifecycleStatus, len(order.Stages))
	for i, s := range order.Stages {
		out[i] = s.Status
	}
	return out
}

func TestStageRemainingSkippedHeldForPublishAwaitsPublish(t *testing.T) {
	next, effects, err := Reduce(approvedFirstStageState(), mustStateEvent(60, ingest.EventStageRemainingSkipped, map[string]any{
		"order_id": "o1", "stage_index": 1, "hold_for_publish": true,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	order := next.Orders["o1"]
	want := []state.StageLifecycleStatus{state.StageCompleted, state.StageSkipped, state.StageSkipped}
	if got := stageStatuses(order); len(got) != 3 || got[0] != want[0] || got[1] != want[1] || got[2] != want[2] {
		t.Fatalf("stages = %v, want %v", got, want)
	}
	if order.Status != state.OrderAwaitingPublish {
		t.Fatalf("order status = %q, want %q", order.Status, state.OrderAwaitingPublish)
	}
	assertEffectTypes(t, effects, EffectWriteProjection)
	if err := next.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestStageRemainingSkippedWithoutHoldCompletes(t *testing.T) {
	next, effects, err := Reduce(approvedFirstStageState(), mustStateEvent(61, ingest.EventStageRemainingSkipped, map[string]any{
		"order_id": "o1", "stage_index": 1,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	if got := next.Orders["o1"].Status; got != state.OrderCompleted {
		t.Fatalf("order status = %q, want completed", got)
	}
	assertEffectTypes(t, effects, EffectWriteProjection, EffectAck)
}

func TestStageRemainingSkippedLeavesEarlierAndTerminalStages(t *testing.T) {
	current := approvedFirstStageState()
	order := current.Orders["o1"]
	order.Stages[0].Status = state.StageRunning
	order.Stages[2].Status = state.StageFailed
	current.Orders["o1"] = order

	next, effects, err := Reduce(current, mustStateEvent(62, ingest.EventStageRemainingSkipped, map[string]any{
		"order_id": "o1", "stage_index": 1, "hold_for_publish": true,
	}, fixtureTime()))
	if err != nil {
		t.Fatalf("reduce: %v", err)
	}
	got := next.Orders["o1"]
	want := []state.StageLifecycleStatus{state.StageRunning, state.StageSkipped, state.StageFailed}
	if s := stageStatuses(got); s[0] != want[0] || s[1] != want[1] || s[2] != want[2] {
		t.Fatalf("stages = %v, want %v", s, want)
	}
	if got.Status != state.OrderActive {
		t.Fatalf("order status = %q, want active while a stage runs", got.Status)
	}
	if len(effects) != 0 {
		t.Fatalf("effects = %+v, want none", effects)
	}
}
