package loop

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/state"
	loopruntime "github.com/poteto/noodle/runtime"
)

type gateTestLoop struct {
	l  *Loop
	rt *mockRuntime
	wt *fakeWorktree
	ar *fakeAdapterRunner
}

func newGateTestLoop(t *testing.T, mode string, external []string, stages ...Stage) gateTestLoop {
	t.Helper()
	return newGateTestLoopPublishing(t, mode, external, "", stages...)
}

func newGateTestLoopPublishing(t *testing.T, mode string, external []string, publishAction string, stages ...Stage) gateTestLoop {
	t.Helper()
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	ordersPath := writeTestOrders(t, runtimeDir, OrdersFile{Orders: []Order{{
		ID: "42", Title: "gate me", Status: OrderStatusActive, Stages: stages,
	}}})
	cfg := config.DefaultConfig()
	cfg.Mode = mode
	cfg.Review.ExternalStages = external
	cfg.Review.PublishAction = publishAction
	g := gateTestLoop{rt: newMockRuntime(), wt: &fakeWorktree{}, ar: &fakeAdapterRunner{}}
	g.l = New(projectDir, "noodle", cfg, g.deps(ordersPath))
	return g
}

func (g gateTestLoop) deps(ordersPath string) Dependencies {
	return Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": g.rt},
		Worktree:   g.wt,
		Adapter:    g.ar,
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: ordersPath,
	}
}

// runStage dispatches order 42's next stage and lets its session exit
// cleanly; the fake worktree reports changes for every stage.
func (g gateTestLoop) runStage(t *testing.T) {
	t.Helper()
	before := len(g.rt.sessions)
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("dispatch cycle: %v", err)
	}
	if len(g.rt.sessions) != before+1 {
		t.Fatalf("sessions = %d, want %d", len(g.rt.sessions), before+1)
	}
	g.rt.sessions[before].complete("completed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("completion cycle: %v", err)
	}
}

func (g gateTestLoop) loopEvents(t *testing.T) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(g.l.runtimeDir, "loop-events.ndjson"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatalf("read loop events: %v", err)
	}
	return string(data)
}

func (g gateTestLoop) assertAwaitingPublish(t *testing.T, wantBranches ...string) {
	t.Helper()
	l := g.l
	if got := l.canonical.Orders["42"].Status; got != state.OrderAwaitingPublish {
		t.Fatalf("canonical status = %q, want %q", got, state.OrderAwaitingPublish)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if len(orders.Orders) != 1 || orders.Orders[0].Status != OrderStatusAwaitingPublish {
		t.Fatalf("orders.json = %+v, want order 42 awaiting-publish", orders.Orders)
	}
	events := g.loopEvents(t)
	if strings.Contains(events, `"order.completed"`) {
		t.Fatalf("gated order emitted order.completed:\n%s", events)
	}
	if !strings.Contains(events, `"order.awaiting_publish"`) {
		t.Fatalf("missing order.awaiting_publish event:\n%s", events)
	}
	if len(g.ar.doneCalls) != 0 {
		t.Fatalf("backlog done ran for a gated order: %v", g.ar.doneCalls)
	}
	if len(g.wt.merged) != 0 {
		t.Fatalf("gated order merged: %v", g.wt.merged)
	}
	rec := l.loadOrderWork()["42"]
	if !slices.Equal(rec.Branches, wantBranches) {
		t.Fatalf("kept branches = %v, want %v", rec.Branches, wantBranches)
	}
	for _, b := range wantBranches {
		if slices.Contains(g.wt.cleaned, b) {
			t.Fatalf("kept branch %s was cleaned up (cleaned=%v)", b, g.wt.cleaned)
		}
	}
}

func TestGatedExternalOrderKeepsWorkAndAwaitsPublish(t *testing.T) {
	g := newGateTestLoop(t, "auto", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	if len(g.wt.merged) != 0 {
		t.Fatalf("gated stage merged: %v", g.wt.merged)
	}
	if branch, ok := g.l.orderWorkBranch("42"); !ok || branch != "42-0-execute" {
		t.Fatalf("kept branch = %q (%v), want 42-0-execute", branch, ok)
	}
	if p, parked := g.l.cooks.pendingReview["42"]; parked && p.stageIndex != 1 {
		t.Fatalf("auto-mode gated stage parked at stage %d; want it advanced to the review hold", p.stageIndex)
	}

	holdExternal(t, g.l)
	if from, ok := createFrom(g.wt, "42-1-review"); !ok || from != "42-0-execute" {
		t.Fatalf("review worktree from = %q (created %v), want 42-0-execute", from, ok)
	}
	if len(g.wt.merged) != 0 {
		t.Fatalf("merged before the external hold completed: %v", g.wt.merged)
	}
	if err := g.l.dispatchControlCommand(stageComplete("review", "accept", "looks good")); err != nil {
		t.Fatalf("stage-complete: %v", err)
	}
	if !slices.Equal(g.wt.merged, []string{"42-1-review"}) {
		t.Fatalf("merged = %v, want [42-1-review]", g.wt.merged)
	}
	assertPublishedCompleted(t, g, "42-0-execute", "42-1-review")
}

func TestGatedAutoOrderPublishFailureAwaitsPublish(t *testing.T) {
	g := newGateTestLoop(t, "auto", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
	)
	g.wt.mergeErr = errors.New("rebase conflict")
	g.runStage(t)
	if !slices.Equal(g.wt.merged, []string{"42-0-execute"}) {
		t.Fatalf("merged = %v, want one publish attempt of 42-0-execute", g.wt.merged)
	}
	assertStillAwaitingPublish(t, g, "rebase conflict")
	if !strings.Contains(g.loopEvents(t), `"order.awaiting_publish"`) {
		t.Fatalf("missing order.awaiting_publish:\n%s", g.loopEvents(t))
	}
	if _, ok := g.l.loadOrderWork()["42"]; !ok {
		t.Fatal("failed auto publish released kept work")
	}

	g.wt.mergeErr = nil
	if ack := g.l.applyControlCommand(approveAndPublish("42")); ack.Status != "ok" {
		t.Fatalf("retry ack = %+v", ack)
	}
	assertPublishedCompleted(t, g, "42-0-execute")
}

func TestGatedByPublishSkillAutoOrderPublishesItself(t *testing.T) {
	g := newGateTestLoopPublishing(t, "auto", nil, "ship-it",
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
	)
	withPublishSkill(t, g, "ship-it")
	g.runStage(t)
	if len(g.wt.merged) != 0 {
		t.Fatalf("gated stage merged: %v", g.wt.merged)
	}
	if stages := publishStages(t, g.l); len(stages) != 1 || stages[0].Skill != "ship-it" {
		t.Fatalf("publish stages = %+v, want one ship-it stage", stages)
	}

	if len(g.rt.sessions) != 2 || g.rt.calls[len(g.rt.calls)-1].Skill != "ship-it" {
		t.Fatalf("publish stage not dispatched after the last stage: %+v", g.rt.calls)
	}
	if from, ok := createFrom(g.wt, g.rt.calls[1].Name); !ok || from != "42-0-execute" {
		t.Fatalf("publish worktree from = %q (created %v), want 42-0-execute", from, ok)
	}
	g.rt.sessions[1].complete("completed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("completion cycle: %v", err)
	}
	if len(g.wt.merged) != 0 {
		t.Fatalf("publish stage merged: %v", g.wt.merged)
	}
	assertPublishedCompleted(t, g, "42-0-execute")
}

func TestGatedSupervisedOrderAwaitsPublishThenPublishes(t *testing.T) {
	g := newGateTestLoop(t, "supervised", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	if _, parked := g.l.cooks.pendingReview["42"]; !parked {
		t.Fatal("supervised stage with changes should park for approve")
	}
	if err := g.l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	g.assertAwaitingPublish(t, "42-0-execute")

	if ack := g.l.applyControlCommand(approveAndPublish("42")); ack.Status != "ok" {
		t.Fatalf("approve-and-publish ack = %+v", ack)
	}
	if !slices.Equal(g.wt.merged, []string{"42-0-execute"}) {
		t.Fatalf("merged = %v, want [42-0-execute]", g.wt.merged)
	}
	assertPublishedCompleted(t, g, "42-0-execute")
}

func TestAwaitingPublishOrderSurvivesRestart(t *testing.T) {
	for _, dropSnapshot := range []bool{false, true} {
		t.Run(fmt.Sprintf("dropSnapshot=%v", dropSnapshot), func(t *testing.T) {
			g := newGateTestLoop(t, "supervised", []string{"review"},
				Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
			)
			g.runStage(t)
			if err := g.l.controlApprove("42"); err != nil {
				t.Fatalf("approve: %v", err)
			}
			if dropSnapshot {
				if err := os.Remove(filepath.Join(g.l.runtimeDir, "state.snapshot.json")); err != nil {
					t.Fatalf("remove snapshot: %v", err)
				}
			}

			restarted := g
			restarted.rt = newMockRuntime()
			restarted.l = New(g.l.projectDir, "noodle", g.l.config, g.deps(g.l.deps.OrdersFile))
			if err := restarted.l.Cycle(context.Background()); err != nil {
				t.Fatalf("cycle after restart: %v", err)
			}
			restarted.assertAwaitingPublish(t, "42-0-execute")
			for _, call := range restarted.rt.calls {
				if call.Skill == "execute" {
					t.Fatalf("awaiting-publish order dispatched after restart: %+v", restarted.rt.calls)
				}
			}
		})
	}
}

// assertCompletedUngated checks an order without fork features finished the
// upstream way: merged per stage, completed, never awaiting publish.
func assertCompletedUngated(t *testing.T, g gateTestLoop, wantMerged ...string) {
	t.Helper()
	if !slices.Equal(g.wt.merged, wantMerged) {
		t.Fatalf("merged = %v, want %v", g.wt.merged, wantMerged)
	}
	if got := g.l.canonical.Orders["42"].Status; got != state.OrderCompleted {
		t.Fatalf("canonical status = %q, want completed", got)
	}
	orders, err := readOrders(g.l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if len(orders.Orders) != 0 {
		t.Fatalf("completed order should leave orders.json: %+v", orders.Orders)
	}
	events := g.loopEvents(t)
	if !strings.Contains(events, `"order.completed"`) || strings.Contains(events, `"order.awaiting_publish"`) {
		t.Fatalf("loop events:\n%s", events)
	}
	if !slices.Equal(g.ar.doneCalls, []string{"42"}) {
		t.Fatalf("done calls = %v, want [42]", g.ar.doneCalls)
	}
	if _, ok := g.l.loadOrderWork()["42"]; ok {
		t.Fatal("ungated order should keep no work")
	}
}

func TestAutoOrderWithoutExternalStagesMergesAndCompletes(t *testing.T) {
	g := newGateTestLoop(t, "auto", nil,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	if !slices.Equal(g.wt.merged, []string{"42-0-execute"}) {
		t.Fatalf("first stage merged = %v, want [42-0-execute]", g.wt.merged)
	}
	if len(g.rt.sessions) != 2 {
		t.Fatalf("sessions = %d, want stage 1 dispatched after the stage 0 merge", len(g.rt.sessions))
	}
	g.rt.sessions[1].complete("completed")
	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("completion cycle: %v", err)
	}
	assertCompletedUngated(t, g, "42-0-execute", "42-1-review")
}

func TestSupervisedUngatedOrderMergesPerStage(t *testing.T) {
	g := newGateTestLoop(t, "supervised", nil,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	if p, parked := g.l.cooks.pendingReview["42"]; !parked || p.stageIndex != 0 {
		t.Fatal("supervised stage 0 should park")
	}
	if block, ok := g.l.reviewActionBlock("42"); ok {
		t.Fatalf("ungated park blocked: %+v", block)
	}
	if ack := g.l.applyControlCommand(ControlCommand{ID: "m1", Action: "merge", OrderID: "42"}); ack.Status != "ok" {
		t.Fatalf("merge ack = %+v", ack)
	}
	if !slices.Equal(g.wt.merged, []string{"42-0-execute"}) {
		t.Fatalf("merged = %v, want [42-0-execute]", g.wt.merged)
	}
	if got := g.l.canonical.Orders["42"].Status; got == state.OrderCompleted || got == state.OrderAwaitingPublish {
		t.Fatalf("order status after first merge = %q, want still running", got)
	}

	g.runStage(t)
	if p, parked := g.l.cooks.pendingReview["42"]; !parked || p.stageIndex != 1 {
		t.Fatal("supervised stage 1 should park")
	}
	if ack := g.l.applyControlCommand(ControlCommand{ID: "m2", Action: "merge", OrderID: "42"}); ack.Status != "ok" {
		t.Fatalf("merge ack = %+v", ack)
	}
	assertCompletedUngated(t, g, "42-0-execute", "42-1-review")
}

func TestGatedOrderRefusesMerge(t *testing.T) {
	for _, tc := range []struct {
		name          string
		external      []string
		publishAction string
	}{
		{"external_stages", []string{"review"}, ""},
		{"publish_action", nil, "draft-pr"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			g := newGateTestLoopPublishing(t, "supervised", tc.external, tc.publishAction,
				Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
			)
			g.runStage(t)
			pendingBefore := *g.l.cooks.pendingReview["42"]
			ack := g.l.applyControlCommand(ControlCommand{ID: "m1", Action: "merge", OrderID: "42"})
			if ack.Status != "error" {
				t.Fatalf("merge ack = %+v, want error", ack)
			}
			for _, want := range []string{"merge refused", "publishes at the end", "approve-and-publish"} {
				if !strings.Contains(ack.Message, want) {
					t.Fatalf("ack message = %q, want %q", ack.Message, want)
				}
			}
			if len(g.wt.merged) != 0 {
				t.Fatalf("refused merge merged: %v", g.wt.merged)
			}
			if p, ok := g.l.cooks.pendingReview["42"]; !ok || !reflect.DeepEqual(*p, pendingBefore) {
				t.Fatal("refused merge changed the pending review")
			}
			block, ok := g.l.buildLoopStateSnapshot().ReviewBlocks["42"]
			if !ok || !slices.Contains(block.Actions, "merge") || slices.Contains(block.Actions, "approve") || !strings.Contains(block.Reason, "publishes at the end") {
				t.Fatalf("review block = %+v (ok=%v)", block, ok)
			}
		})
	}
}
