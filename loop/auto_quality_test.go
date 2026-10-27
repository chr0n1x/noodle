package loop

import (
	"context"
	"encoding/json"
	"strings"
	"testing"

	"github.com/poteto/noodle/event"
	"github.com/poteto/noodle/internal/taskreg"
	"github.com/poteto/noodle/skill"
)

func registryWithQuality() taskreg.Registry {
	skills := []skill.SkillMeta{}
	for _, name := range []string{"schedule", "execute", "reflect", "review", "quality", "adversarial-review"} {
		skills = append(skills, skill.SkillMeta{Name: name, Path: "/skills/" + name, Frontmatter: skill.Frontmatter{Schedule: "when needed"}})
	}
	return taskreg.NewFromSkills(skills)
}

func newAutoQualityLoop(t *testing.T, on bool, external []string, stages ...Stage) gateTestLoop {
	t.Helper()
	g := newGateTestLoop(t, "auto", external, stages...)
	g.l.registry = registryWithQuality()
	g.l.config.Review.AutoVerifyQuality = on
	return g
}

// runStageWithMessage dispatches order 42's next stage, records a
// stage_message from its session, then lets the session exit cleanly.
func runStageWithMessage(t *testing.T, g gateTestLoop, message string, blocking bool) {
	t.Helper()
	before := len(g.rt.sessions)
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("dispatch cycle: %v", err)
	}
	if len(g.rt.sessions) != before+1 {
		t.Fatalf("sessions = %d, want %d", len(g.rt.sessions), before+1)
	}
	s := g.rt.sessions[before]
	writer, err := event.NewEventWriter(g.l.runtimeDir, s.ID())
	if err != nil {
		t.Fatalf("event writer: %v", err)
	}
	payload, _ := json.Marshal(event.StageMessagePayload{Message: message, Blocking: &blocking})
	if err := writer.Append(context.Background(), event.Event{Type: event.EventStageMessage, Payload: payload}); err != nil {
		t.Fatalf("append stage_message: %v", err)
	}
	s.complete("completed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("completion cycle: %v", err)
	}
}

func lastDispatchSkill(g gateTestLoop) string {
	if len(g.rt.calls) == 0 {
		return ""
	}
	return g.rt.calls[len(g.rt.calls)-1].Skill
}

func taskKeys(stages []Stage) []string {
	keys := make([]string, len(stages))
	for i, s := range stages {
		keys[i] = s.TaskKey
	}
	return keys
}

func TestNonExternalQualityRejectParksToday(t *testing.T) {
	g := newAutoQualityLoop(t, false, []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Provider: "claude", Status: StageStatusPending},
	)
	runStageWithMessage(t, g, "Rejected: missing test", true)

	p, ok := g.l.cooks.pendingReview["42"]
	if !ok || p.stageIndex != 1 || !strings.Contains(p.reason, "blocked by stage message: Rejected: missing test") {
		t.Fatalf("pending = %+v", p)
	}
	if got := taskKeys(orderStagesOf(t, g.l)); !equalStrings(got, []string{"execute", "quality"}) {
		t.Fatalf("stages = %v", got)
	}
}

func orderStagesOf(t *testing.T, l *Loop) []Stage {
	t.Helper()
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	for _, o := range orders.Orders {
		if o.ID == "42" {
			return o.Stages
		}
	}
	t.Fatalf("order 42 missing: %+v", orders.Orders)
	return nil
}

func TestAutoQualityInsertedAfterExecuteAndRunsAsCook(t *testing.T) {
	for _, external := range [][]string{{"review"}, {"quality", "review"}} {
		t.Run(strings.Join(external, ","), func(t *testing.T) {
			g := newAutoQualityLoop(t, true, external,
				Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "claude-sonnet", Status: StageStatusPending},
				Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
			)
			g.l.config.Review.ExtraStages = []string{"adversarial-review"}
			g.runStage(t)

			stages := orderStagesOf(t, g.l)
			if got := taskKeys(stages); !equalStrings(got, []string{"execute", "quality", "adversarial-review", "review"}) {
				t.Fatalf("stages = %v", got)
			}
			q := stages[1]
			if q.Skill != "quality" || q.Provider != "claude" || q.Model != "claude-sonnet" {
				t.Fatalf("quality stage = %+v", q)
			}
			if err := g.l.Cycle(context.Background()); err != nil {
				t.Fatalf("cycle: %v", err)
			}
			if lastDispatchSkill(g) != "quality" {
				t.Fatalf("dispatch = %+v, want a quality cook", g.rt.calls)
			}
			if _, held := g.l.cooks.pendingReview["42"]; held {
				t.Fatal("auto quality stage must run as a cook, not a hold")
			}
		})
	}
}

func TestAutoQualityNotDuplicatedWhenScheduled(t *testing.T) {
	g := newAutoQualityLoop(t, true, []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "quality", Skill: "quality", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
	)
	g.runStage(t)
	if got := taskKeys(orderStagesOf(t, g.l)); !equalStrings(got, []string{"execute", "quality", "review"}) {
		t.Fatalf("stages = %v", got)
	}
}

func TestAutoQualityOffOrUngatedKeepsUpstream(t *testing.T) {
	off := newAutoQualityLoop(t, false, []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
	)
	off.runStage(t)
	if got := taskKeys(orderStagesOf(t, off.l)); !equalStrings(got, []string{"execute", "review"}) {
		t.Fatalf("flag off stages = %v", got)
	}
	if _, held := off.l.cooks.pendingReview["42"]; !held {
		t.Fatal("flag off: review should be held right after execute")
	}

	ungated := newAutoQualityLoop(t, true, nil,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
	)
	ungated.runStage(t)
	assertCompletedUngated(t, ungated, "42-0-execute")
}

func newQualityRejectLoop(t *testing.T) gateTestLoop {
	t.Helper()
	return newAutoQualityLoop(t, true, []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "claude-sonnet", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
	)
}

func TestAutoQualityRejectQueuesExecuteRework(t *testing.T) {
	g := newQualityRejectLoop(t)
	runStageWithMessage(t, g, "Rejected: missing test for edge case", true)

	if _, held := g.l.cooks.pendingReview["42"]; held {
		t.Fatal("a quality reject should queue rework, not park")
	}
	stages := orderStagesOf(t, g.l)
	if got := taskKeys(stages); !equalStrings(got, []string{"execute", "quality", "execute", "quality", "review"}) {
		t.Fatalf("stages = %v", got)
	}
	rework := stages[2]
	if rework.Model != "claude-sonnet" || !strings.Contains(rework.Prompt, "missing test for edge case") || string(rework.Extra[reworkExtraKey]) != "1" {
		t.Fatalf("rework stage = %+v", rework)
	}
	if lastDispatchSkill(g) != "execute" || !strings.Contains(g.rt.calls[len(g.rt.calls)-1].Prompt, "missing test for edge case") {
		if err := g.l.Cycle(context.Background()); err != nil {
			t.Fatalf("cycle: %v", err)
		}
		if lastDispatchSkill(g) != "execute" {
			t.Fatalf("dispatch = %+v, want the rework execute", g.rt.calls)
		}
	}

	// The rework execute is followed by the copied quality stage, so no
	// second quality stage is inserted.
	g.rt.sessions[len(g.rt.sessions)-1].complete("completed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if got := taskKeys(orderStagesOf(t, g.l)); !equalStrings(got, []string{"execute", "quality", "execute", "quality", "review"}) {
		t.Fatalf("after rework stages = %v", got)
	}
}

func TestAutoQualityRejectAtLimitFailsOrder(t *testing.T) {
	g := newQualityRejectLoop(t)
	zero := 0
	g.l.config.Review.MaxRework = &zero
	runStageWithMessage(t, g, "Rejected: still wrong", true)

	if _, held := g.l.cooks.pendingReview["42"]; held {
		t.Fatal("at the limit the order should fail, not park")
	}
	orders, _ := readOrders(g.l.deps.OrdersFile)
	for _, o := range orders.Orders {
		if o.ID == "42" && o.Status == OrderStatusActive {
			t.Fatalf("order still active: %+v", o)
		}
	}
	if ev := g.loopEvents(t); !strings.Contains(ev, "still wrong") || !strings.Contains(ev, "rework limit (0)") {
		t.Fatalf("failure should carry the review and the limit:\n%s", ev)
	}
}

func TestAutoQualityAcceptContinues(t *testing.T) {
	g := newQualityRejectLoop(t)
	runStageWithMessage(t, g, "Accepted", false)
	if p, held := g.l.cooks.pendingReview["42"]; !held || p.stageIndex != 2 {
		t.Fatalf("review should be held next, pending = %+v", p)
	}
}
