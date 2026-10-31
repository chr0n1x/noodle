package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	loopruntime "github.com/poteto/noodle/runtime"
)

func newExitAfterTurnLoop(t *testing.T, exitAfterTurn bool) (*Loop, *cookHandle) {
	t.Helper()
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	ordersPath := filepath.Join(runtimeDir, "orders.json")
	if err := writeOrdersAtomic(ordersPath, OrdersFile{Orders: []Order{{
		ID: "7", Title: "abstention", Status: OrderStatusActive,
		Stages: []Stage{
			{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusActive},
			{TaskKey: "quality", Skill: "quality", Provider: "claude", Status: StageStatusPending},
		},
	}}}); err != nil {
		t.Fatalf("write orders: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Agents.Claude.ExitAfterTurn = exitAfterTurn
	l := New(projectDir, "noodle", cfg, Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": newMockRuntime()},
		Worktree:   &fakeWorktree{hasUnmergedCommits: map[string]bool{"7-0-execute": false}},
		Adapter:    &fakeAdapterRunner{},
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: ordersPath,
	})

	sessionDir := filepath.Join(runtimeDir, "sessions", "7-0-execute-sess")
	if err := os.MkdirAll(sessionDir, 0o755); err != nil {
		t.Fatal(err)
	}
	canonical := `{"provider":"claude","type":"action","message":"$ grep -n retry docs/prompt.md","timestamp":"2026-10-26T18:09:00Z"}
{"provider":"claude","type":"action","message":"text:The fix is outside my allowed paths. Should I edit docs/prompt.md?","timestamp":"2026-10-26T18:09:30Z"}
{"provider":"claude","type":"result","message":"turn complete","timestamp":"2026-10-26T18:09:33Z"}
`
	if err := os.WriteFile(filepath.Join(sessionDir, "canonical.ndjson"), []byte(canonical), 0o644); err != nil {
		t.Fatal(err)
	}

	cook := &cookHandle{
		cookIdentity: cookIdentity{
			orderID:    "7",
			stageIndex: 0,
			stage:      Stage{TaskKey: "execute", Skill: "execute", Provider: "claude"},
		},
		orderStatus:  OrderStatusActive,
		worktreeName: "7-0-execute",
		worktreePath: filepath.Join(projectDir, ".worktrees", "7-0-execute"),
		session:      &mockSession{id: "7-0-execute-sess", status: "completed", done: make(chan struct{})},
	}
	return l, cook
}

func TestExitAfterTurnHoldsNoChangeExitForReview(t *testing.T) {
	l, cook := newExitAfterTurnLoop(t, true)

	if err := l.handleCompletion(context.Background(), cook, StageResultCompleted, "completed"); err != nil {
		t.Fatalf("handleCompletion: %v", err)
	}
	pending, ok := l.cooks.pendingReview["7"]
	if !ok {
		t.Fatal("a no-change exit with exit_after_turn should wait for review")
	}
	if !strings.Contains(pending.reason, "Should I edit docs/prompt.md?") {
		t.Fatalf("review reason should carry the agent's last message, got %q", pending.reason)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if len(orders.Orders) != 1 || orders.Orders[0].Stages[1].Status != StageStatusPending {
		t.Fatalf("order advanced past the held stage: %+v", orders.Orders)
	}
}

func TestNoChangeExitAdvancesWithoutExitAfterTurn(t *testing.T) {
	l, cook := newExitAfterTurnLoop(t, false)

	if err := l.handleCompletion(context.Background(), cook, StageResultCompleted, "completed"); err != nil {
		t.Fatalf("handleCompletion: %v", err)
	}
	if _, ok := l.cooks.pendingReview["7"]; ok {
		t.Fatal("default behavior changed: no-change exit was held for review")
	}
}

func TestExitAfterTurnAdvancesNoChangeExitWithNonBlockingStageMessage(t *testing.T) {
	l, cook := newExitAfterTurnLoop(t, true)
	events := `{"type":"stage_message","payload":{"message":"Accepted: review written, checks quoted.","blocking":false},"timestamp":"2026-10-26T18:09:31Z","session_id":"7-0-execute-sess"}
`
	path := filepath.Join(l.runtimeDir, "sessions", "7-0-execute-sess", "events.ndjson")
	if err := os.WriteFile(path, []byte(events), 0o644); err != nil {
		t.Fatal(err)
	}

	if err := l.handleCompletion(context.Background(), cook, StageResultCompleted, "completed"); err != nil {
		t.Fatalf("handleCompletion: %v", err)
	}
	if pending, ok := l.cooks.pendingReview["7"]; ok {
		t.Fatalf("a non-blocking stage_message marks the turn finished, but the stage parked: %q", pending.reason)
	}
}

func TestExitAfterTurnUsesTheStageProvidersSetting(t *testing.T) {
	l, cook := newExitAfterTurnLoop(t, false)
	cook.stage.Provider = "maki"
	l.config.Agents.Maki.ExitAfterTurn = true
	if !l.holdsNoChangeExit(cook) {
		t.Fatal("maki exit_after_turn should hold a no-change exit")
	}

	l, cook = newExitAfterTurnLoop(t, true)
	cook.stage.Provider = "maki"
	if l.holdsNoChangeExit(cook) {
		t.Fatal("claude exit_after_turn should not apply to a maki stage")
	}
}
