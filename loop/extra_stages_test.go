package loop

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/poteto/noodle/adapter"
	"github.com/poteto/noodle/internal/taskreg"
	"github.com/poteto/noodle/mise"
	"github.com/poteto/noodle/skill"
)

func knowAll(string) bool { return true }

func stageKeys(order Order) []string {
	keys := make([]string, len(order.Stages))
	for i, s := range order.Stages {
		keys[i] = s.TaskKey
	}
	return keys
}

func TestApplyExtraStagesInsertsAfterPendingQuality(t *testing.T) {
	orders := OrdersFile{Orders: []Order{
		{ID: "a", Stages: []Stage{
			{TaskKey: "execute", Status: StageStatusPending},
			{TaskKey: "quality", Skill: "quality", Provider: "claude", Model: "opus", Status: StageStatusPending},
			{TaskKey: "reflect", Status: StageStatusPending},
		}},
		{ID: "no-quality", Stages: []Stage{{TaskKey: "execute", Status: StageStatusPending}}},
		{ID: "has-it", Stages: []Stage{
			{TaskKey: "quality", Status: StageStatusPending},
			{TaskKey: "adversarial-review", Status: StageStatusPending},
		}},
		{ID: "quality-done", Stages: []Stage{{TaskKey: "quality", Status: StageStatusCompleted}}},
	}}

	got, _ := applyExtraStages(orders, []string{"adversarial-review", " "}, knowAll)

	want := map[string][]string{
		"a":            {"execute", "quality", "adversarial-review", "reflect"},
		"no-quality":   {"execute", "adversarial-review"},
		"has-it":       {"quality", "adversarial-review"},
		"quality-done": {"quality"},
	}
	for _, order := range got.Orders {
		if k := stageKeys(order); !equalStrings(k, want[order.ID]) {
			t.Errorf("order %s stages = %v, want %v", order.ID, k, want[order.ID])
		}
	}
	added := got.Orders[0].Stages[2]
	if added.Skill != "adversarial-review" || added.Provider != "claude" || added.Model != "opus" || added.Status != StageStatusPending {
		t.Errorf("added stage = %+v", added)
	}
	if len(orders.Orders[0].Stages) != 3 {
		t.Errorf("input order was mutated: %v", stageKeys(orders.Orders[0]))
	}
}

func TestApplyExtraStagesAppendsToUnstartedOrderWithoutQuality(t *testing.T) {
	orders := OrdersFile{Orders: []Order{
		{ID: "review-only", Stages: []Stage{
			{TaskKey: "adversarial-review", Skill: "adversarial-review", Provider: "claude", Model: "sonnet", Status: StageStatusPending},
		}},
		{ID: "started", Stages: []Stage{
			{TaskKey: "execute", Status: StageStatusCompleted},
			{TaskKey: "adversarial-review", Status: StageStatusPending},
		}},
		{ID: scheduleOrderID, Stages: []Stage{{TaskKey: "schedule", Status: StageStatusPending}}},
		{ID: scheduleBootstrapOrderID, Stages: []Stage{{TaskKey: "oops", Status: StageStatusPending}}},
		{ID: "empty"},
	}}

	got, _ := applyExtraStages(orders, []string{"adversarial-review", "reflect"}, knowAll)

	want := map[string][]string{
		"review-only":            {"adversarial-review", "reflect"},
		"started":                {"execute", "adversarial-review"},
		scheduleOrderID:          {"schedule"},
		scheduleBootstrapOrderID: {"oops"},
		"empty":                  {},
	}
	for _, order := range got.Orders {
		if k := stageKeys(order); !equalStrings(k, want[order.ID]) {
			t.Errorf("order %s stages = %v, want %v", order.ID, k, want[order.ID])
		}
	}
	added := got.Orders[0].Stages[1]
	if added.Skill != "reflect" || added.Provider != "claude" || added.Model != "sonnet" || added.Status != StageStatusPending {
		t.Errorf("added stage = %+v", added)
	}
}

func TestApplyExtraStagesNoneConfigured(t *testing.T) {
	orders := OrdersFile{Orders: []Order{{ID: "a", Stages: []Stage{{TaskKey: "quality", Status: StageStatusPending}}}}}
	if got, _ := applyExtraStages(orders, nil, knowAll); len(got.Orders[0].Stages) != 1 {
		t.Errorf("stages = %v", stageKeys(got.Orders[0]))
	}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestApplyExtraStagesSkipsUnknownSkills(t *testing.T) {
	orders := OrdersFile{Orders: []Order{{ID: "a", Stages: []Stage{{TaskKey: "quality", Status: StageStatusPending}}}}}
	known := func(name string) bool { return name == "known" }
	got, unknown := applyExtraStages(orders, []string{"missing", "known"}, known)
	if k := stageKeys(got.Orders[0]); !equalStrings(k, []string{"quality", "known"}) {
		t.Errorf("stages = %v", k)
	}
	if !equalStrings(unknown, []string{"missing"}) {
		t.Errorf("unknown = %v", unknown)
	}
}

func TestPromotionAppliesExtraStages(t *testing.T) {
	logger, _ := newTestLogger()
	tc := newTestLoop(t, logger, func(o *testLoopOpts) {
		brief := mise.Brief{Backlog: []adapter.BacklogItem{{ID: "1", Title: "test", Status: "open"}}}
		o.brief = &brief
	})
	tc.loop.config.Review.ExtraStages = []string{"adversarial-review"}
	tc.loop.registry = taskreg.NewFromSkills([]skill.SkillMeta{
		{Name: "schedule", Path: "/skills/schedule", Frontmatter: skill.Frontmatter{Schedule: "When queue is empty"}},
		{Name: "execute", Path: "/skills/execute", Frontmatter: skill.Frontmatter{Schedule: "When ready"}},
		{Name: "quality", Path: "/skills/quality", Frontmatter: skill.Frontmatter{Schedule: "After execute"}},
		{Name: "adversarial-review", Path: "/skills/adversarial-review", Frontmatter: skill.Frontmatter{Schedule: "After quality"}},
	})
	ordersNextPath := filepath.Join(tc.runtimeDir, "orders-next.json")
	compactJSON := []byte(`{"orders":[{"id":"o1","title":"t","stages":[{"do":"execute","with":"claude","model":"m"},{"do":"quality","with":"claude","model":"m"}]}]}`)
	if err := os.WriteFile(ordersNextPath, compactJSON, 0o644); err != nil {
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
	if len(orders.Orders) != 1 {
		t.Fatalf("orders = %d", len(orders.Orders))
	}
	if k := stageKeys(orders.Orders[0]); !equalStrings(k, []string{"execute", "quality", "adversarial-review"}) {
		t.Errorf("stages = %v", k)
	}
}

func TestPromotionAppendsExtraStagesToOrderWithoutQuality(t *testing.T) {
	logger, _ := newTestLogger()
	tc := newTestLoop(t, logger, func(o *testLoopOpts) {
		brief := mise.Brief{Backlog: []adapter.BacklogItem{{ID: "1", Title: "test", Status: "open"}}}
		o.brief = &brief
	})
	tc.loop.config.Review.ExtraStages = []string{"adversarial-review", "reflect"}
	tc.loop.registry = taskreg.NewFromSkills([]skill.SkillMeta{
		{Name: "schedule", Path: "/skills/schedule", Frontmatter: skill.Frontmatter{Schedule: "When queue is empty"}},
		{Name: "adversarial-review", Path: "/skills/adversarial-review", Frontmatter: skill.Frontmatter{Schedule: "After quality"}},
		{Name: "reflect", Path: "/skills/reflect", Frontmatter: skill.Frontmatter{Schedule: "After quality"}},
	})
	ordersNextPath := filepath.Join(tc.runtimeDir, "orders-next.json")
	compactJSON := []byte(`{"orders":[{"id":"o1","title":"t","stages":[{"do":"adversarial-review","with":"claude","model":"m"}]}]}`)
	if err := os.WriteFile(ordersNextPath, compactJSON, 0o644); err != nil {
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
	if len(orders.Orders) != 1 {
		t.Fatalf("orders = %d", len(orders.Orders))
	}
	if k := stageKeys(orders.Orders[0]); !equalStrings(k, []string{"adversarial-review", "reflect"}) {
		t.Errorf("stages = %v", k)
	}
}
