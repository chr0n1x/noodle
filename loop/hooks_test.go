package loop

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/poteto/noodle/adapter"
	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/state"
	"github.com/poteto/noodle/internal/taskreg"
	"github.com/poteto/noodle/mise"
	"github.com/poteto/noodle/skill"
)

func allKnown(string) bool { return true }

func hookPoints(stages []Stage) []string {
	out := make([]string, len(stages))
	for i, s := range stages {
		out[i] = hookPoint(s)
	}
	return out
}

func unstartedOrder(keys ...string) Order {
	stages := make([]Stage, len(keys))
	for i, k := range keys {
		stages[i] = Stage{TaskKey: k, Skill: k, Provider: "claude", Model: "m-" + k, Status: StageStatusPending}
	}
	return Order{ID: "o1", Status: OrderStatusActive, Stages: stages}
}

func TestApplyHooksWrapsOrderAndEachWorkStage(t *testing.T) {
	after := []string{"reflect"}
	hooks := config.HooksConfig{Before: []string{"prep"}, After: &after, BeforeEach: []string{"lint"}, AfterEach: []string{"note"}}
	out, unknown := applyHooks(OrdersFile{Orders: []Order{unstartedOrder("execute", "adversarial-review")}}, hooks, allKnown)
	if len(unknown) != 0 {
		t.Fatalf("unknown = %v", unknown)
	}
	got := out.Orders[0].Stages
	want := []string{"prep", "lint", "execute", "note", "lint", "adversarial-review", "note", "reflect"}
	if !slices.Equal(taskKeys(got), want) {
		t.Fatalf("stages = %v, want %v", taskKeys(got), want)
	}
	wantPoints := []string{"before", "before_each", "", "after_each", "before_each", "", "after_each", "after"}
	if !slices.Equal(hookPoints(got), wantPoints) {
		t.Fatalf("hook points = %v, want %v", hookPoints(got), wantPoints)
	}
	if got[3].Model != "m-execute" || got[7].Model != "m-adversarial-review" {
		t.Fatalf("hooks should reuse the model of the stage they wrap: %q, %q", got[3].Model, got[7].Model)
	}
}

func TestApplyHooksDefaultAddsReflectOnce(t *testing.T) {
	out, _ := applyHooks(OrdersFile{Orders: []Order{unstartedOrder("execute")}}, config.HooksConfig{}, allKnown)
	if got := taskKeys(out.Orders[0].Stages); !slices.Equal(got, []string{"execute", "reflect"}) {
		t.Fatalf("stages = %v, want [execute reflect]", got)
	}
	again, _ := applyHooks(out, config.HooksConfig{}, allKnown)
	if got := taskKeys(again.Orders[0].Stages); !slices.Equal(got, []string{"execute", "reflect"}) {
		t.Fatalf("second pass stages = %v, want hooks added once", got)
	}
}

func TestApplyHooksDedupesEachListInOrder(t *testing.T) {
	after := []string{"reflect", "note", "reflect", " note "}
	hooks := config.HooksConfig{
		Before:     []string{"prep", "lint", "prep"},
		After:      &after,
		BeforeEach: []string{"fmt", "fmt"},
		AfterEach:  []string{"check", "log", "check"},
	}
	out, _ := applyHooks(OrdersFile{Orders: []Order{unstartedOrder("execute")}}, hooks, allKnown)
	want := []string{"prep", "lint", "fmt", "execute", "check", "log", "reflect", "note"}
	if got := taskKeys(out.Orders[0].Stages); !slices.Equal(got, want) {
		t.Fatalf("stages = %v, want %v", got, want)
	}
}

func TestApplyHooksSkipsOrderLevelHookTheOrderHas(t *testing.T) {
	out, _ := applyHooks(OrdersFile{Orders: []Order{unstartedOrder("execute", "reflect")}}, config.HooksConfig{}, allKnown)
	if got := taskKeys(out.Orders[0].Stages); !slices.Equal(got, []string{"execute", "reflect"}) {
		t.Fatalf("stages = %v, want the existing reflect only", got)
	}
}

func TestApplyHooksLeavesStartedAndScheduleOrders(t *testing.T) {
	started := unstartedOrder("execute", "quality")
	started.Stages[0].Status = StageStatusActive
	schedule := Order{ID: scheduleOrderID, Status: OrderStatusActive, Stages: []Stage{{TaskKey: "schedule", Skill: "schedule", Status: StageStatusPending}}}
	out, _ := applyHooks(OrdersFile{Orders: []Order{started, schedule}}, config.HooksConfig{}, allKnown)
	if got := taskKeys(out.Orders[0].Stages); !slices.Equal(got, []string{"execute", "quality"}) {
		t.Fatalf("started order stages = %v", got)
	}
	if got := taskKeys(out.Orders[1].Stages); !slices.Equal(got, []string{"schedule"}) {
		t.Fatalf("schedule order stages = %v", got)
	}
}

func TestApplyHooksReturnsUnknownNames(t *testing.T) {
	hooks := config.HooksConfig{BeforeEach: []string{"missing"}}
	out, unknown := applyHooks(OrdersFile{Orders: []Order{unstartedOrder("execute")}}, hooks, func(name string) bool { return name == "reflect" })
	if !slices.Equal(unknown, []string{"missing"}) {
		t.Fatalf("unknown = %v", unknown)
	}
	if got := taskKeys(out.Orders[0].Stages); !slices.Equal(got, []string{"execute", "reflect"}) {
		t.Fatalf("stages = %v", got)
	}
}

func hookStage(task, point string) Stage {
	return Stage{TaskKey: task, Skill: task, Status: StageStatusPending, Extra: hookExtra(point)}
}

func TestHookWithChangesNeverParks(t *testing.T) {
	stages := []Stage{hookStage("lint", "before_each"), {TaskKey: "execute", Skill: "execute", Status: StageStatusPending}}
	stages[0].Status = StageStatusActive
	f := newOutcomeFixture(t, stages, nil)
	f.wt.hasUnmergedCommits = map[string]bool{"order-1-0-lint": true}
	f.finish(0, StageResultCompleted)
	if _, parked := f.l.cooks.pendingReview["order-1"]; parked {
		t.Fatal("a hook should never park")
	}
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StagePending)
}

func TestFailedHookIsSkipped(t *testing.T) {
	stages := []Stage{{TaskKey: "execute", Skill: "execute", Status: StageStatusCompleted}, hookStage("note", "after_each")}
	stages[1].Status = StageStatusActive
	f := newOutcomeFixture(t, stages, nil)
	f.finish(1, StageResultFailed)
	f.wantStatuses(state.OrderCompleted, state.StageCompleted, state.StageSkipped)
}

func TestExitKeepsAfterHooks(t *testing.T) {
	stages := []Stage{
		{TaskKey: "execute", Skill: "execute", Status: StageStatusActive},
		{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusPending},
		hookStage("reflect", "after"),
	}
	f := newOutcomeFixture(t, stages, map[string]config.StageOutcomeConfig{"execute": {Reject: "exit"}})
	f.finish(0, StageResultFailed)
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StageSkipped, state.StagePending)
	f.finish(2, StageResultCompleted)
	f.wantStatuses(state.OrderFailed, state.StageCompleted, state.StageSkipped, state.StageCompleted)
}

func TestAmendKeepsAfterHook(t *testing.T) {
	existing := Order{ID: "o1", Status: OrderStatusActive, Stages: []Stage{
		{TaskKey: "execute", Skill: "execute", Status: StageStatusActive},
		{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
		hookStage("reflect", "after"),
	}}
	incoming := Order{ID: "o1", Status: OrderStatusActive, Stages: []Stage{
		{TaskKey: "execute", Skill: "execute", Status: StageStatusPending},
		{TaskKey: "document", Skill: "document", Status: StageStatusPending},
	}}
	merged, ok := mergeAmendedActiveOrder(existing, incoming, nil)
	if !ok {
		t.Fatal("expected an amend")
	}
	last := merged.Stages[len(merged.Stages)-1]
	if last.TaskKey != "reflect" || last.Status != StageStatusPending {
		t.Fatalf("last stage = %s %s, want the reflect hook still pending", last.TaskKey, last.Status)
	}
}

func promoteOneOrder(t *testing.T, configure func(*config.Config)) Order {
	t.Helper()
	logger, _ := newTestLogger()
	tc := newTestLoop(t, logger, func(o *testLoopOpts) {
		brief := mise.Brief{Backlog: []adapter.BacklogItem{{ID: "1", Title: "test", Status: "open"}}}
		o.brief = &brief
	})
	configure(&tc.loop.config)
	tc.loop.registry = taskreg.NewFromSkills([]skill.SkillMeta{
		{Name: "schedule", Path: "/skills/schedule", Frontmatter: skill.Frontmatter{Schedule: "When queue is empty"}},
		{Name: "execute", Path: "/skills/execute", Frontmatter: skill.Frontmatter{Schedule: "Always"}},
		{Name: "reflect", Path: "/skills/reflect", Frontmatter: skill.Frontmatter{Schedule: "After quality"}},
	})
	ordersNextPath := filepath.Join(tc.runtimeDir, "orders-next.json")
	if err := os.WriteFile(ordersNextPath, []byte(`{"orders":[{"id":"o1","title":"t","stages":[{"do":"execute","with":"claude","model":"m"}]}]}`), 0o644); err != nil {
		t.Fatalf("write orders-next: %v", err)
	}
	tc.loop.deps.OrdersNextFile = ordersNextPath
	if err := tc.loop.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	orders, err := tc.loop.currentOrders()
	if err != nil {
		t.Fatalf("orders: %v", err)
	}
	for _, o := range orders.Orders {
		if o.ID == "o1" {
			return o
		}
	}
	t.Fatal("o1 not promoted")
	return Order{}
}

func TestPromotionAddsDefaultReflectHook(t *testing.T) {
	order := promoteOneOrder(t, func(*config.Config) {})
	if k := taskKeys(order.Stages); !slices.Equal(k, []string{"execute", "reflect"}) {
		t.Fatalf("stages = %v, want [execute reflect]", k)
	}
	if got := hookPoint(order.Stages[1]); got != hookAfter {
		t.Fatalf("reflect hook point = %q, want after", got)
	}
}

func TestPromotionWithAfterHooksOff(t *testing.T) {
	off := []string{}
	order := promoteOneOrder(t, func(c *config.Config) { c.Hooks.After = &off })
	if k := taskKeys(order.Stages); !slices.Equal(k, []string{"execute"}) {
		t.Fatalf("stages = %v, want [execute]", k)
	}
}
