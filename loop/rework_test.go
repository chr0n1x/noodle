package loop

import (
	"context"
	"strings"
	"testing"
)

func rejectReview(t *testing.T, l *Loop, text string) {
	t.Helper()
	if err := l.dispatchControlCommand(stageComplete("review", "reject", text)); err != nil {
		t.Fatalf("reject: %v", err)
	}
}

func newReworkTestLoop(t *testing.T) (*Loop, *mockRuntime, *fakeWorktree) {
	t.Helper()
	l, rt, wt := newExternalReviewTestLoop(t, []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "claude-sonnet", Status: StageStatusCompleted},
		Stage{TaskKey: "review", Skill: "review", Prompt: "check it", Status: StageStatusPending},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusPending},
	)
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if _, ok := l.cooks.pendingReview["42"]; !ok {
		t.Fatal("review stage was not held")
	}
	return l, rt, wt
}

func TestRejectQueuesReworkThenReview(t *testing.T) {
	l, rt, _ := newReworkTestLoop(t)

	rejectReview(t, l, "missing tests")

	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("rejected review should leave the list when rework is queued")
	}
	if !strings.Contains(l.cmds.ackMessage, "rework round 1 of 2") {
		t.Fatalf("ack = %q", l.cmds.ackMessage)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	stages := orders.Orders[0].Stages
	if len(stages) != 5 {
		t.Fatalf("stages = %+v, want 5", stages)
	}
	rework, again := stages[2], stages[3]
	if rework.TaskKey != "execute" || rework.Provider != "claude" || rework.Model != "claude-sonnet" || !strings.Contains(rework.Prompt, "missing tests") {
		t.Fatalf("rework stage = %+v", rework)
	}
	if again.TaskKey != "review" || again.Prompt != "check it" || again.Status != StageStatusPending {
		t.Fatalf("repeated review stage = %+v", again)
	}

	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 1 || rt.calls[0].Skill != "execute" || !strings.Contains(rt.calls[0].Prompt, "missing tests") {
		t.Fatalf("dispatch = %+v, want the rework execute with the review text", rt.calls)
	}
	if d := lastDecision(t, l); d["verdict"] != "rejected" || !strings.Contains(d["outcome"].(string), "rework round 1 of 2") {
		t.Fatalf("decision = %v", d)
	}
}

func TestRejectAtReworkLimitReparks(t *testing.T) {
	l, _, _ := newReworkTestLoop(t)
	zero := 0
	l.config.Review.MaxRework = &zero

	rejectReview(t, l, "still wrong")

	if _, ok := l.cooks.pendingReview["42"]; !ok {
		t.Fatal("at the limit the review should stay parked for a human")
	}
	if !strings.Contains(l.cmds.ackMessage, "stage blocked") {
		t.Fatalf("ack = %q", l.cmds.ackMessage)
	}
	orders, _ := readOrders(l.deps.OrdersFile)
	if len(orders.Orders[0].Stages) != 3 {
		t.Fatalf("no stages should be added at the limit: %+v", orders.Orders[0].Stages)
	}
}

func TestReworkRoundsAreCounted(t *testing.T) {
	l, _, _ := newReworkTestLoop(t)
	one := 1
	l.config.Review.MaxRework = &one
	if err := l.mutateOrdersState(func(o *OrdersFile) (bool, error) {
		o.Orders[0].Stages[0].Extra = reworkExtra(1)
		return true, nil
	}); err != nil {
		t.Fatal(err)
	}

	rejectReview(t, l, "again")

	if _, ok := l.cooks.pendingReview["42"]; !ok {
		t.Fatal("one round already used; the review should stay parked")
	}
	if !strings.Contains(l.cmds.ackMessage, "rework limit (1) reached") {
		t.Fatalf("ack = %q", l.cmds.ackMessage)
	}
}
