package loop

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/poteto/noodle/adapter"
	"github.com/poteto/noodle/internal/state"
	"github.com/poteto/noodle/mise"
)

func TestNewOrderIgnoresKeptWorkOfEarlierOrderWithSameID(t *testing.T) {
	logger, _ := newTestLogger()
	tc := newTestLoop(t, logger, func(o *testLoopOpts) {
		brief := mise.Brief{Backlog: []adapter.BacklogItem{{ID: "18", Title: "rerun", Status: "open"}}}
		o.brief = &brief
	})
	if err := tc.loop.keepOrderWork("18", "18-2-reflect"); err != nil {
		t.Fatalf("keep work: %v", err)
	}
	ordersNextPath := filepath.Join(tc.runtimeDir, "orders-next.json")
	if err := os.WriteFile(ordersNextPath, []byte(`{"orders":[{"id":"18","title":"t","stages":[{"do":"execute","with":"claude","model":"m"}]}]}`), 0o644); err != nil {
		t.Fatalf("write orders-next: %v", err)
	}
	tc.loop.deps.OrdersNextFile = ordersNextPath

	if err := tc.loop.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if branch, ok := tc.loop.orderWorkBranch("18"); ok {
		t.Fatalf("new order 18 still has kept work %q from the earlier order", branch)
	}
	for i, name := range tc.worktree.created {
		if name == "18-0-execute" && tc.worktree.createOpts[i].From != "" {
			t.Fatalf("18-0-execute created from %q, want the default base", tc.worktree.createOpts[i].From)
		}
	}
}

func TestWorktreeCreateFailureFailsOrderAndKeepsLoopRunning(t *testing.T) {
	g := newGateTestLoop(t, "auto", nil,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "m", Status: StageStatusPending},
	)
	g.wt.createErr = errors.New("failed to create worktree: exit status 128: fatal: invalid reference: 42-2-reflect")

	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle returned %v; a worktree failure for one order should not stop the loop", err)
	}
	order, ok := g.l.canonical.Orders["42"]
	if !ok {
		t.Fatal("order 42 missing from canonical state")
	}
	if order.Status != state.OrderFailed {
		t.Fatalf("order status = %q, want failed", order.Status)
	}
	if len(g.rt.calls) != 0 {
		t.Fatalf("dispatch calls = %d, want none for the failed order", len(g.rt.calls))
	}
}
