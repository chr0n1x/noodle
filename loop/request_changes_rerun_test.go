package loop

import (
	"bufio"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poteto/noodle/internal/state"
	loopruntime "github.com/poteto/noodle/runtime"
)

func newGatedControlTestLoop(t *testing.T) (*Loop, *fakeWorktree, *mockRuntime) {
	t.Helper()
	wt := &fakeWorktree{}
	rt := newMockRuntime()
	l := newControlTestLoop(t, wt, rt)
	l.config.Review.PublishAction = "draft-pr"
	if err := os.MkdirAll(l.worktreePath("42-0-execute"), 0o755); err != nil {
		t.Fatalf("mkdir worktree: %v", err)
	}
	return l, wt, rt
}

func loopEventTypes(t *testing.T, l *Loop) []string {
	t.Helper()
	file, err := os.Open(filepath.Join(l.runtimeDir, "loop-events.ndjson"))
	if os.IsNotExist(err) {
		return nil
	}
	if err != nil {
		t.Fatalf("open loop events: %v", err)
	}
	defer file.Close()
	var out []string
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var raw struct {
			Type string `json:"type"`
		}
		if json.Unmarshal(scanner.Bytes(), &raw) == nil {
			out = append(out, raw.Type)
		}
	}
	return out
}

func requestChanges(feedback string) ControlCommand {
	return ControlCommand{ID: "rc", Action: "request-changes", OrderID: "42", Prompt: feedback}
}

func TestGatedRequestChangesRerunsStageInSameWorktree(t *testing.T) {
	l, wt, rt := newGatedControlTestLoop(t)
	keep := filepath.Join(l.worktreePath("42-0-execute"), "uncommitted.txt")
	if err := os.WriteFile(keep, []byte("wip"), 0o644); err != nil {
		t.Fatalf("write wip: %v", err)
	}

	ack := l.applyControlCommand(requestChanges("Add missing tests"))
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("pending review should be removed")
	}
	if _, ok := l.canonical.PendingReviews["42"]; ok {
		t.Fatal("canonical pending review should be removed")
	}
	co := l.canonical.Orders["42"]
	if co.Status != state.OrderActive || co.Stages[0].Status != state.StagePending {
		t.Fatalf("canonical order = %s stage = %s, want active/pending", co.Status, co.Stages[0].Status)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	o := orders.Orders[0]
	if o.Status != OrderStatusActive || o.Stages[0].Status != StageStatusPending || len(o.Stages) != 1 {
		t.Fatalf("orders.json = %+v, want active order with one pending stage", o)
	}
	if !strings.Contains(o.Stages[0].Prompt, "Changes requested (round 1 of 2): Add missing tests") {
		t.Fatalf("prompt = %q", o.Stages[0].Prompt)
	}
	if o.Stages[0].TaskKey != "execute" || o.Stages[0].Provider != "claude" || o.Stages[0].Model != "claude-opus-4-6" {
		t.Fatalf("stage identity changed: %+v", o.Stages[0])
	}
	if len(wt.cleaned) != 0 || len(wt.merged) != 0 {
		t.Fatalf("cleaned = %v merged = %v, want none", wt.cleaned, wt.merged)
	}
	for _, typ := range loopEventTypes(t, l) {
		if typ == "stage.failed" || typ == "order.failed" {
			t.Fatalf("unexpected loop event %s", typ)
		}
	}
	if d := lastDecision(t, l); d["verdict"] != "changes_requested" || !strings.Contains(d["outcome"].(string), "round 1 of 2") {
		t.Fatalf("decision = %v", d)
	}

	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(rt.calls))
	}
	req := rt.calls[0]
	if req.Name != "42-0-execute" || req.WorktreePath != l.worktreePath("42-0-execute") {
		t.Fatalf("dispatch = %s %s, want the parked worktree", req.Name, req.WorktreePath)
	}
	// RetryCount 0 means spawnCook skips resetWorktreeState.
	if req.RetryCount != 0 {
		t.Fatalf("retry count = %d, want 0", req.RetryCount)
	}
	if !strings.Contains(req.Prompt, "Add missing tests") || !strings.Contains(req.Prompt, "Continue in this worktree") {
		t.Fatalf("dispatch prompt lacks feedback: %q", req.Prompt)
	}
	if len(wt.created) != 0 || len(wt.cleaned) != 0 {
		t.Fatalf("created = %v cleaned = %v, want reuse", wt.created, wt.cleaned)
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("uncommitted work lost: %v", err)
	}
}

func TestGatedRequestChangesEmptyFeedbackReruns(t *testing.T) {
	l, _, _ := newGatedControlTestLoop(t)
	if err := l.controlRequestChanges("42", "  "); err != nil {
		t.Fatalf("request changes: %v", err)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if orders.Orders[0].Status != OrderStatusActive || !strings.Contains(orders.Orders[0].Stages[0].Prompt, "Changes requested (round 1 of 2)") {
		t.Fatalf("order = %+v", orders.Orders[0])
	}
}

func TestGatedRequestChangesForwardsRerunToScheduler(t *testing.T) {
	l, _, _ := newGatedControlTestLoop(t)
	sched := &mockController{steerable: true}
	l.cooks.activeCooksByOrder["schedule"] = &cookHandle{
		cookIdentity: cookIdentity{orderID: "schedule", stage: Stage{TaskKey: "schedule", Skill: "schedule"}},
		session: &steerableSession{
			mockSession: &mockSession{id: "schedule-session", status: "running", done: make(chan struct{})},
			ctrl:        sched,
		},
	}
	if err := l.controlRequestChanges("42", "Add missing tests"); err != nil {
		t.Fatalf("request changes: %v", err)
	}
	deadline := time.Now().Add(2 * time.Second)
	for {
		sched.mu.Lock()
		msg := sched.lastSentMessage
		sched.mu.Unlock()
		if strings.Contains(msg, "[request_changes_rerun]") && strings.Contains(msg, "Add missing tests") {
			return
		}
		if time.Now().After(deadline) {
			t.Fatalf("scheduler message = %q", msg)
		}
		time.Sleep(10 * time.Millisecond)
	}
}

// parkAgain puts stage 0 of order 42 back into review, as a finished re-run would.
func parkAgain(t *testing.T, l *Loop) {
	t.Helper()
	if err := l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		orders.Orders[0].Stages[0].Status = StageStatusActive
		return true, nil
	}); err != nil {
		t.Fatalf("mutate: %v", err)
	}
	l.cooks.pendingReview["42"] = &pendingReviewCook{
		cookIdentity: cookIdentity{orderID: "42", stageIndex: 0, stage: Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "claude-opus-4-6"}},
		worktreeName: "42-0-execute",
		worktreePath: l.worktreePath("42-0-execute"),
		sessionID:    "sess-42b",
	}
	if err := l.writePendingReview(); err != nil {
		t.Fatalf("write pending review: %v", err)
	}
}

func TestGatedRequestChangesSharesReworkBudget(t *testing.T) {
	l, wt, _ := newGatedControlTestLoop(t)
	if err := l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		orders.Orders[0].Stages = append(orders.Orders[0].Stages,
			Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusPending, Extra: reworkExtra(1)})
		return true, nil
	}); err != nil {
		t.Fatalf("mutate: %v", err)
	}

	if ack := l.applyControlCommand(requestChanges("round two")); ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if got := reworkRounds(orders.Orders[0]); got != 2 {
		t.Fatalf("rework rounds = %d, want 2", got)
	}
	if !strings.Contains(orders.Orders[0].Stages[0].Prompt, "round 2 of 2") {
		t.Fatalf("prompt = %q", orders.Orders[0].Stages[0].Prompt)
	}

	parkAgain(t, l)
	ack := l.applyControlCommand(requestChanges("round three"))
	if ack.Status != "ok" || !strings.Contains(ack.Message, "rework limit") {
		t.Fatalf("ack = %+v, want rework limit message", ack)
	}
	orders, err = readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if orders.Orders[0].Status != OrderStatusFailed || orders.Orders[0].Stages[0].Status != StageStatusFailed {
		t.Fatalf("order = %+v, want failed", orders.Orders[0])
	}
	if len(wt.cleaned) != 0 {
		t.Fatalf("cleaned = %v, want kept", wt.cleaned)
	}
	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("pending review should be removed")
	}
	if envelope := requireLastLoopFailureEnvelope(t, l); !strings.Contains(envelope.Message, "rework limit") {
		t.Fatalf("failure message = %q", envelope.Message)
	}
}

func TestGatedRequestChangesSurvivesRestart(t *testing.T) {
	l, _, _ := newGatedControlTestLoop(t)
	if err := l.controlRequestChanges("42", "Add missing tests"); err != nil {
		t.Fatalf("request changes: %v", err)
	}

	rt := newMockRuntime()
	restarted := New(l.projectDir, "noodle", l.config, Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": rt},
		Worktree:   &fakeWorktree{},
		Adapter:    &fakeAdapterRunner{},
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: l.deps.OrdersFile,
	})
	if err := restarted.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 1 || rt.calls[0].Name != "42-0-execute" || !strings.Contains(rt.calls[0].Prompt, "Add missing tests") {
		t.Fatalf("calls = %+v", rt.calls)
	}
}

func TestRequestChangesOnExternalHoldQueuesRework(t *testing.T) {
	l, rt, _ := newExternalReviewTestLoop(t, []string{"quality"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "m", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
	)
	withReviewSkills(l)
	holdExternal(t, l)
	before := len(rt.calls)

	ack := l.applyControlCommand(requestChanges("missing tests"))
	if ack.Status != "ok" || !strings.Contains(ack.Message, "rework round 1 of 2 queued") {
		t.Fatalf("ack = %+v", ack)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if keys := stageKeys(orders.Orders[0]); !equalStrings(keys, []string{"execute", "quality", "execute", "quality"}) {
		t.Fatalf("stages = %v", keys)
	}
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	found := false
	for _, c := range rt.calls[before:] {
		if c.Skill == "execute" && strings.Contains(c.Prompt, "missing tests") {
			found = true
		}
	}
	if !found {
		t.Fatal("rework execute with the feedback was not dispatched")
	}
}
