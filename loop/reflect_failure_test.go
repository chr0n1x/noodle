package loop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/state"
	loopruntime "github.com/poteto/noodle/runtime"
)

func failLastStage(t *testing.T, taskKey string, result StageResultStatus) state.OrderNode {
	t.Helper()
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	ordersPath := filepath.Join(runtimeDir, "orders.json")
	if err := writeOrdersAtomic(ordersPath, OrdersFile{
		Orders: []Order{{
			ID:     "order-1",
			Status: OrderStatusActive,
			Stages: []Stage{
				{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusCompleted},
				{TaskKey: taskKey, Skill: taskKey, Status: StageStatusActive},
			},
		}},
	}); err != nil {
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
	cook := &cookHandle{
		cookIdentity: cookIdentity{orderID: "order-1", stageIndex: 1, stage: Stage{TaskKey: taskKey, Skill: taskKey}},
		session:      &mockSession{id: "order-1-session", status: string(result), done: make(chan struct{})},
	}
	if err := l.handleCompletion(context.Background(), cook, result, string(result)); err != nil {
		t.Fatalf("handleCompletion: %v", err)
	}
	order, ok := l.canonical.Orders["order-1"]
	if !ok {
		t.Fatal("order-1 missing from canonical state")
	}
	return order
}

func TestFailedReflectSkipsStageAndCompletesOrder(t *testing.T) {
	order := failLastStage(t, "reflect", StageResultFailed)
	if order.Status != state.OrderCompleted {
		t.Fatalf("order status = %q, want %q", order.Status, state.OrderCompleted)
	}
	if got := order.Stages[1].Status; got != state.StageSkipped {
		t.Fatalf("reflect stage status = %q, want %q", got, state.StageSkipped)
	}
}

func TestCancelledReflectStillFailsOrder(t *testing.T) {
	order := failLastStage(t, "reflect", StageResultCancelled)
	if order.Status != state.OrderFailed {
		t.Fatalf("order status = %q, want %q", order.Status, state.OrderFailed)
	}
}

func TestFailedNonReflectStageStillFailsOrder(t *testing.T) {
	order := failLastStage(t, "quality", StageResultFailed)
	if order.Status != state.OrderFailed {
		t.Fatalf("order status = %q, want %q", order.Status, state.OrderFailed)
	}
}
