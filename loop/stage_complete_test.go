package loop

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/poteto/noodle/internal/taskreg"
	"github.com/poteto/noodle/skill"
)

func withReviewSkills(l *Loop) {
	metas := []skill.SkillMeta{}
	for _, name := range []string{"schedule", "execute", "review", "quality", "adversarial-review", "reflect"} {
		metas = append(metas, skill.SkillMeta{Name: name, Path: "/skills/" + name, Frontmatter: skill.Frontmatter{Schedule: "test"}})
	}
	l.registry = taskreg.NewFromSkills(metas)
}

func assertHeldReviewAdvancedTo(t *testing.T, l *Loop, rt *mockRuntime, wantSkill string) {
	t.Helper()
	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("held review should leave the list")
	}
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 1 || rt.calls[0].Skill != wantSkill {
		t.Fatalf("next stage dispatch = %+v, want %s", rt.calls, wantSkill)
	}
}

func newHeldReviewForPublish(t *testing.T) (*Loop, *mockRuntime, *fakeWorktree) {
	t.Helper()
	l, rt, wt := newExternalReviewTestLoop(t, []string{"review"},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusPending},
	)
	wt.hasUnmergedCommits = map[string]bool{"42-0-review": true}
	holdExternal(t, l)
	return l, rt, wt
}

func stageComplete(stage, value, review string) ControlCommand {
	return ControlCommand{ID: "c1", Action: "stage-complete", OrderID: "42", TaskKey: stage, Prompt: review, Value: value}
}

// acceptHeld accepts order 42's held stage by its own task key.
func acceptHeld(l *Loop) error {
	pending, ok := l.cooks.pendingReview["42"]
	if !ok {
		return os.ErrNotExist
	}
	return l.dispatchControlCommand(stageComplete(pending.stage.TaskKey, "accept", "looks good"))
}

func assertStageCompleteRefused(t *testing.T, l *Loop, cmd ControlCommand, want ...string) {
	t.Helper()
	before := captureGuardState(t, l)
	ack := l.applyControlCommand(cmd)
	if ack.Status != "error" {
		t.Fatalf("ack = %+v, want error", ack)
	}
	for _, w := range want {
		if !strings.Contains(ack.Message, w) {
			t.Fatalf("ack message = %q, want %q", ack.Message, w)
		}
	}
	if after := captureGuardState(t, l); !reflect.DeepEqual(before, after) {
		t.Fatalf("state changed:\nbefore %+v\nafter  %+v", before, after)
	}
	if _, err := ReadExternalReviewVerdict(l.runtimeDir, "42"); err == nil {
		t.Fatal("a refused stage-complete recorded a verdict")
	}
}

func TestStageCompleteAcceptAdvancesWithoutPublishing(t *testing.T) {
	l, wt, pr := qualityHeldBeforeAdversarial(t)
	l.deps.DraftPR = pr

	ack := l.applyControlCommand(stageComplete("quality", "accept", "looks good"))
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	if len(pr.requests) != 0 || len(wt.merged) != 0 {
		t.Fatalf("accept published: PR requests %d, merged %v", len(pr.requests), wt.merged)
	}
	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("accepted stage should leave pending review")
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if got := orders.Orders[0].Stages[0].Status; got != StageStatusCompleted {
		t.Fatalf("quality status = %q, want completed", got)
	}
	if d := lastDecision(t, l); d["verdict"] != "accepted" || d["task_key"] != "quality" {
		t.Fatalf("decision = %v", d)
	}
	if v, err := ReadExternalReviewVerdict(l.runtimeDir, "42"); err != nil || v.Verdict != "accepted" {
		t.Fatalf("verdict = %+v, err %v", v, err)
	}
}

func TestStageCompleteAcceptKeepsWork(t *testing.T) {
	l, wt := newApproveTestLoop(t)
	l.deps.DraftPR = &fakeDraftPR{}
	if err := l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	holdExternal(t, l)

	if err := l.dispatchControlCommand(stageComplete("review", "accept", "looks good")); err != nil {
		t.Fatalf("stage-complete: %v", err)
	}
	if len(wt.cleaned) != 0 || len(wt.merged) != 0 {
		t.Fatalf("accept must keep work: cleaned=%v merged=%v", wt.cleaned, wt.merged)
	}
	if branch, ok := l.orderWorkBranch("42"); !ok || branch != "42-1-review" {
		t.Fatalf("kept branch = %q (%v), want 42-1-review", branch, ok)
	}
}

func TestStageCompleteWrongStageIsRefused(t *testing.T) {
	l, _, _ := qualityHeldBeforeAdversarial(t)
	assertStageCompleteRefused(t, l, stageComplete("adversarial-review", "accept", "ok"),
		"stage-complete refused: order 42 is held at stage quality; use noodle order 42 complete quality")
}

func TestStageCompleteWithoutHoldIsRefused(t *testing.T) {
	l, _, _ := guardTestLoop(t, 0, reviewStagesAfterExecute()...)
	ack := l.applyControlCommand(stageComplete("execute", "accept", "ok"))
	if ack.Status != "error" || !strings.Contains(ack.Message, "not held for external review") {
		t.Fatalf("ack = %+v", ack)
	}

	empty, _, _ := newExternalReviewTestLoop(t, []string{"review"})
	ack = empty.applyControlCommand(stageComplete("review", "accept", "ok"))
	if ack.Status != "error" || !strings.Contains(ack.Message, "not held for external review") {
		t.Fatalf("ack = %+v", ack)
	}
}

func TestStageCompleteHeldStageWithoutTaskKeyIsRefused(t *testing.T) {
	l, _, _ := guardTestLoop(t, -1, Stage{Skill: "quality", Status: StageStatusPending})
	assertStageCompleteRefused(t, l, stageComplete("quality", "accept", "ok"),
		"order 42 is held at a stage with no task key; it cannot be completed by name")
}

func TestStageCompleteRejectQueuesRework(t *testing.T) {
	l, _, pr := guardTestLoop(t, -1,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "m", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
		Stage{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusPending},
	)
	ack := l.applyControlCommand(stageComplete("quality", "reject", "missing tests"))
	if ack.Status != "ok" || !strings.Contains(ack.Message, "rework round 1 of 2") {
		t.Fatalf("ack = %+v", ack)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if keys := stageKeys(orders.Orders[0]); !equalStrings(keys, []string{"execute", "quality", "execute", "quality", "adversarial-review"}) {
		t.Fatalf("stages = %v", keys)
	}
	if len(pr.requests) != 0 {
		t.Fatalf("reject published: %d PR requests", len(pr.requests))
	}
	if d := lastDecision(t, l); d["verdict"] != "rejected" {
		t.Fatalf("decision = %v", d)
	}
}

func TestStageCompleteRejectRespectsMaxRework(t *testing.T) {
	l, _, _ := guardTestLoop(t, -1,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "m", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
	)
	zero := 0
	l.config.Review.MaxRework = &zero
	ack := l.applyControlCommand(stageComplete("quality", "reject", "still wrong"))
	if ack.Status != "ok" || !strings.Contains(ack.Message, "stage blocked") {
		t.Fatalf("ack = %+v", ack)
	}
	if _, ok := l.cooks.pendingReview["42"]; !ok {
		t.Fatal("at the limit the stage should stay parked")
	}
	orders, _ := readOrders(l.deps.OrdersFile)
	if len(orders.Orders[0].Stages) != 2 {
		t.Fatalf("no stages should be added at the limit: %v", stageKeys(orders.Orders[0]))
	}
}

func TestStageCompleteRepeatedNameAfterReworkCompletesHeldCopy(t *testing.T) {
	l, _, pr := guardTestLoop(t, -1,
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusCompleted},
		Stage{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusCompleted},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusCompleted, Extra: reworkExtra(1)},
		Stage{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusPending},
		Stage{TaskKey: "reflect", Skill: "reflect", Status: StageStatusPending},
	)
	if got := l.cooks.pendingReview["42"].stageIndex; got != 3 {
		t.Fatalf("held stage index = %d, want the rework copy at 3", got)
	}
	if ack := l.applyControlCommand(stageComplete("adversarial-review", "accept", "ok now")); ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if got := orders.Orders[0].Stages[3].Status; got != StageStatusCompleted {
		t.Fatalf("rework copy status = %q, want completed", got)
	}
	if got := orders.Orders[0].Stages[4].Status; got != StageStatusPending {
		t.Fatalf("reflect status = %q, want pending", got)
	}
	if len(pr.requests) != 0 {
		t.Fatalf("accept published: %d PR requests", len(pr.requests))
	}
}

func TestRemovedReviewActionsAreUnknown(t *testing.T) {
	for _, action := range []string{"review-complete", "quality-review-complete", "adversarial-review-complete", "merge", "draft-pr"} {
		t.Run(action, func(t *testing.T) {
			l, wt, pr := qualityHeldBeforeAdversarial(t)
			ack := l.applyControlCommand(ControlCommand{ID: "c1", Action: action, OrderID: "42", Prompt: "ok", Value: "accept"})
			if ack.Status != "error" {
				t.Fatalf("ack = %+v, want error", ack)
			}
			if len(wt.merged) != 0 || len(pr.requests) != 0 {
				t.Fatalf("%s published", action)
			}
		})
	}
}

func TestReviewPromptNamesOrderCompleteCommand(t *testing.T) {
	for _, key := range []string{"quality", "adversarial-review"} {
		t.Run(key, func(t *testing.T) {
			l, _, _ := guardTestLoop(t, -1, Stage{TaskKey: key, Skill: key, Status: StageStatusPending})
			data, err := os.ReadFile(filepath.Join(l.runtimeDir, "reviews", "42-0.prompt.md"))
			if err != nil {
				t.Fatalf("read prompt: %v", err)
			}
			want := "noodle order 42 complete " + key + " --file "
			if !strings.Contains(string(data), want) || !strings.Contains(string(data), "--reject") {
				t.Fatalf("prompt missing %q:\n%s", want, data)
			}
		})
	}
}
