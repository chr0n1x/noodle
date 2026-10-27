package loop

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/orderx"
	"github.com/poteto/noodle/internal/state"
	loopruntime "github.com/poteto/noodle/runtime"
)

func TestReconcileStaleDispatchKeepsParkedStagesActive(t *testing.T) {
	projectDir := t.TempDir()
	ordersPath := filepath.Join(projectDir, ".noodle", "orders.json")
	orders := OrdersFile{Orders: []Order{
		{ID: "4", Title: "parked", Status: OrderStatusActive, Stages: []Stage{
			{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusActive},
			{TaskKey: "quality", Skill: "quality", Provider: "claude", Status: StageStatusPending},
		}},
		{ID: "7", Title: "stale", Status: OrderStatusActive, Stages: []Stage{
			{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusActive},
		}},
	}}
	if err := writeOrdersAtomic(ordersPath, orders); err != nil {
		t.Fatalf("write orders: %v", err)
	}
	l := New(projectDir, "noodle", config.DefaultConfig(), Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": newMockRuntime()},
		Worktree:   &fakeWorktree{},
		Adapter:    &fakeAdapterRunner{},
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: ordersPath,
	})
	l.canonical = synthesizeCanonicalState(orders, []PendingReviewItem{{OrderID: "4", StageIndex: 0, TaskKey: "execute"}}, state.RunModeSupervised, time.Now())
	stale := l.canonical.Orders["7"]
	stale.Stages[0].Status = state.StageRunning
	l.canonical.Orders["7"] = stale
	if err := l.loadPendingReview(); err != nil {
		t.Fatalf("loadPendingReview: %v", err)
	}

	if err := l.reconcileStaleDispatchStages(); err != nil {
		t.Fatalf("reconcileStaleDispatchStages: %v", err)
	}

	got, err := readOrders(ordersPath)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	status := map[string]orderx.StageStatus{}
	for _, order := range got.Orders {
		status[order.ID] = order.Stages[0].Status
	}
	if status["4"] != StageStatusActive {
		t.Errorf("parked stage of order 4 = %q, want %q", status["4"], StageStatusActive)
	}
	if status["7"] != StageStatusPending {
		t.Errorf("stale stage of order 7 = %q, want %q", status["7"], StageStatusPending)
	}
	if l.canonical.Orders["4"].Stages[0].Status != state.StageReview {
		t.Errorf("canonical parked stage = %q, want %q", l.canonical.Orders["4"].Stages[0].Status, state.StageReview)
	}
}
