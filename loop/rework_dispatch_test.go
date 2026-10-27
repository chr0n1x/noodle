package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/poteto/noodle/internal/state"
)

func TestExternalRejectDispatchesReworkWithFeedback(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stages []Stage
	}{
		{"review in the middle", []Stage{
			{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "m", Status: StageStatusCompleted},
			{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
			{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusPending},
		}},
		{"review is the last stage", []Stage{
			{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "m", Status: StageStatusCompleted},
			{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			l, rt, _ := newExternalReviewTestLoop(t, []string{"quality", "adversarial-review"}, tc.stages...)
			withReviewSkills(l)
			holdExternal(t, l)
			callsBefore := len(rt.calls)

			ack := l.applyControlCommand(stageComplete("quality", "reject", "missing tests"))
			if ack.Status != "ok" || !strings.Contains(ack.Message, "rework round 1 of 2 queued") {
				t.Fatalf("ack = %+v", ack)
			}
			if co := l.canonical.Orders["42"]; co.Status != state.OrderActive {
				t.Fatalf("canonical status = %s, want active", co.Status)
			}

			if err := l.Cycle(context.Background()); err != nil {
				t.Fatalf("cycle: %v", err)
			}
			found := false
			for _, c := range rt.calls[callsBefore:] {
				if c.Skill == "execute" && strings.Contains(c.Prompt, "missing tests") {
					found = true
				}
			}
			if !found {
				t.Fatalf("rework execute with the feedback was not dispatched")
			}
		})
	}
}
