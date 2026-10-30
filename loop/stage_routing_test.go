package loop

import (
	"context"
	"strings"
	"testing"

	"github.com/poteto/noodle/config"
)

var opusReview = map[string]config.StageOutcomeConfig{
	"adversarial-review": {Harness: "codex", Model: "claude-opus-5-5"},
}

func TestApplyStageRoutingSetsPendingStagesOnly(t *testing.T) {
	orders := OrdersFile{Orders: []Order{{ID: "o1", Status: OrderStatusActive, Stages: []Stage{
		{TaskKey: "execute", Provider: "claude", Model: "sonnet", Status: StageStatusPending},
		{TaskKey: "adversarial-review", Provider: "claude", Model: "sonnet", Status: StageStatusActive},
		{TaskKey: "adversarial-review", Provider: "claude", Model: "sonnet", Status: StageStatusPending},
	}}}}
	got := applyStageRouting(orders, opusReview).Orders[0].Stages
	if got[0].Provider != "claude" || got[0].Model != "sonnet" {
		t.Fatalf("unconfigured stage changed: %+v", got[0])
	}
	if got[1].Provider != "claude" || got[1].Model != "sonnet" {
		t.Fatalf("started stage changed: %+v", got[1])
	}
	if got[2].Provider != "codex" || got[2].Model != "claude-opus-5-5" {
		t.Fatalf("pending stage = %+v, want codex claude-opus-5-5", got[2])
	}
}

func TestPromotionRoutesConfiguredStage(t *testing.T) {
	order := promoteOneOrder(t, func(c *config.Config) {
		c.Stages = map[string]config.StageOutcomeConfig{"reflect": {Model: "claude-opus-5-5"}}
	})
	r := order.Stages[len(order.Stages)-1]
	if r.TaskKey != "reflect" || r.Model != "claude-opus-5-5" {
		t.Fatalf("reflect stage = %+v, want model claude-opus-5-5", r)
	}
	if order.Stages[0].Model != "m" {
		t.Fatalf("execute model = %q, want the scheduler's m", order.Stages[0].Model)
	}
}

func TestSpawnUsesStageRoutingAndSystemPrompt(t *testing.T) {
	g := newGateTestLoop(t, "auto", nil,
		Stage{TaskKey: "adversarial-review", Skill: "adversarial-review", Provider: "claude", Model: "sonnet", Status: StageStatusPending},
	)
	g.l.registry = registryWithQuality()
	g.l.config.Stages = map[string]config.StageOutcomeConfig{
		"adversarial-review": {Harness: "codex", Provider: "openai", Model: "gpt-5.5", SystemPrompt: "Run reviewers with pi."},
	}
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(g.rt.calls) == 0 {
		t.Fatal("no dispatch")
	}
	req := g.rt.calls[len(g.rt.calls)-1]
	if req.Provider != "codex" || req.Model != "gpt-5.5" || req.ModelProvider != "openai" {
		t.Fatalf("request routing = %s %s %s", req.Provider, req.Model, req.ModelProvider)
	}
	if strings.TrimSpace(req.StageSystemPrompt) != "Run reviewers with pi." {
		t.Fatalf("stage system prompt = %q", req.StageSystemPrompt)
	}
}
