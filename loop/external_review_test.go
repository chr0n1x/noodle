package loop

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/event"
	"github.com/poteto/noodle/internal/state"
	loopruntime "github.com/poteto/noodle/runtime"
)

func newExternalReviewTestLoop(t *testing.T, external []string, stages ...Stage) (*Loop, *mockRuntime, *fakeWorktree) {
	t.Helper()
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	if len(stages) == 0 {
		stages = []Stage{{TaskKey: "execute", Skill: "execute", Provider: "claude", Model: "claude-opus-4-6", Status: StageStatusPending}}
	}
	ordersPath := writeTestOrders(t, runtimeDir, OrdersFile{Orders: []Order{{
		ID: "42", Title: "review me", Status: OrderStatusActive, Stages: stages,
	}}})
	cfg := config.DefaultConfig()
	cfg.Review.ExternalStages = external
	rt := newMockRuntime()
	wt := &fakeWorktree{}
	l := New(projectDir, "noodle", cfg, Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": rt},
		Worktree:   wt,
		Adapter:    &fakeAdapterRunner{},
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: ordersPath,
	})
	return l, rt, wt
}

func TestNoReviewConfigDispatchesStageNormally(t *testing.T) {
	l, rt, _ := newExternalReviewTestLoop(t, nil)
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(rt.calls))
	}
	if _, err := os.Stat(filepath.Join(l.runtimeDir, "reviews")); !os.IsNotExist(err) {
		t.Fatalf("reviews dir created without [review] config: %v", err)
	}
	if len(l.cooks.pendingReview) != 0 {
		t.Fatalf("pending reviews = %v, want none", l.cooks.pendingReview)
	}
}

func TestExternalStageIsHeldInsteadOfDispatched(t *testing.T) {
	l, rt, _ := newExternalReviewTestLoop(t, []string{"execute"})
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 0 {
		t.Fatalf("dispatch calls = %d, want 0", len(rt.calls))
	}

	promptPath := filepath.Join(l.runtimeDir, "reviews", "42-0.prompt.md")
	data, err := os.ReadFile(promptPath)
	if err != nil {
		t.Fatalf("read prompt file: %v", err)
	}
	prompt := string(data)
	worktreePath := l.worktreePath("42-0-execute")
	for _, want := range []string{"noodle order 42 complete execute --file", "--reject", worktreePath} {
		if !strings.Contains(prompt, want) {
			t.Fatalf("prompt missing %q:\n%s", want, prompt)
		}
	}

	items, err := ReadPendingReview(l.runtimeDir)
	if err != nil {
		t.Fatalf("read pending review: %v", err)
	}
	if len(items) != 1 || !strings.HasPrefix(items[0].Reason, "external review") {
		t.Fatalf("pending review = %+v, want one external review", items)
	}
	if items[0].WorktreePath != worktreePath || items[0].SessionID != "external-42-0" {
		t.Fatalf("pending review = %+v", items[0])
	}

	for i := 0; i < 2; i++ {
		if err := l.Cycle(context.Background()); err != nil {
			t.Fatalf("cycle %d: %v", i, err)
		}
	}
	if len(rt.calls) != 0 {
		t.Fatalf("held stage redispatched: %d calls", len(rt.calls))
	}
}

// holdWithPreviousYield holds the external review stage of a two-stage order
// whose completed execute stage yielded the given message.
func holdWithPreviousYield(t *testing.T, yield string) PendingReviewItem {
	t.Helper()
	l, _, _ := newExternalReviewTestLoop(t, []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusCompleted},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
	)
	if err := l.ensureCanonicalOrderFromOrders("42"); err != nil {
		t.Fatalf("ensure canonical: %v", err)
	}
	order := l.canonical.Orders["42"]
	order.Stages[0].Attempts = append(order.Stages[0].Attempts, state.AttemptNode{AttemptID: "a0", SessionID: "exec-session"})
	l.canonical.Orders["42"] = order

	writer, err := event.NewEventWriter(l.runtimeDir, "exec-session")
	if err != nil {
		t.Fatalf("new event writer: %v", err)
	}
	payload, _ := json.Marshal(event.StageYieldPayload{Message: yield})
	if err := writer.Append(context.Background(), event.Event{Type: event.EventStageYield, Payload: payload}); err != nil {
		t.Fatalf("append yield: %v", err)
	}

	holdExternal(t, l)
	items, err := ReadPendingReview(l.runtimeDir)
	if err != nil || len(items) != 1 {
		t.Fatalf("pending review = %+v, %v", items, err)
	}
	return items[0]
}

func TestExternalReviewReasonIncludesPreviousStageYield(t *testing.T) {
	msg := "Prompt edit committed but NOT verified: tests were not run"
	item := holdWithPreviousYield(t, "  "+msg+"\n")
	if !strings.HasPrefix(item.Reason, "external review: run /noodle-review") {
		t.Fatalf("reason does not start with the review line: %q", item.Reason)
	}
	for _, want := range []string{"\n\nPrevious stage (execute) reported:\n", msg} {
		if !strings.Contains(item.Reason, want) {
			t.Fatalf("reason missing %q:\n%s", want, item.Reason)
		}
	}
}

func TestExternalReviewReasonCapsLongPreviousStageYield(t *testing.T) {
	item := holdWithPreviousYield(t, strings.Repeat("x", 5000))
	if !strings.HasSuffix(item.Reason, strings.Repeat("x", 4000)+"...") {
		t.Fatalf("reason tail = %q", item.Reason[len(item.Reason)-20:])
	}
	if strings.Contains(item.Reason, strings.Repeat("x", 4001)) {
		t.Fatal("yield message was not capped at 4000 characters")
	}
}

func holdExternal(t *testing.T, l *Loop) {
	t.Helper()
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if _, ok := l.cooks.pendingReview["42"]; !ok {
		t.Fatal("stage was not held for external review")
	}
}

func TestStageCompleteAcceptAdvancesOrder(t *testing.T) {
	l, rt, wt := newExternalReviewTestLoop(t, []string{"review"},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusPending},
	)
	holdExternal(t, l)

	if err := l.dispatchControlCommand(stageComplete("review", "accept", "looks good")); err != nil {
		t.Fatalf("stage-complete: %v", err)
	}
	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("pending review should be removed after accept")
	}
	saved, err := os.ReadFile(filepath.Join(l.projectDir, "brain", "reviews", "42-review.md"))
	if err != nil || string(saved) != "looks good" {
		t.Fatalf("saved review = %q, %v", saved, err)
	}
	assertVerdict(t, l.runtimeDir, "accepted", filepath.Join(l.projectDir, "brain", "reviews", "42-review.md"))
	if len(wt.cleaned) != 1 || wt.cleaned[0] != "42-0-review" {
		t.Fatalf("cleaned = %v", wt.cleaned)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if got := orders.Orders[0].Stages[0].Status; got != StageStatusCompleted {
		t.Fatalf("stage 0 status = %q, want completed", got)
	}

	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 1 || rt.calls[0].Skill != "execute" {
		t.Fatalf("next stage dispatch = %+v, want execute", rt.calls)
	}
}

func TestStageCompleteRejectBlocksLikeStageMessage(t *testing.T) {
	l, rt, _ := newExternalReviewTestLoop(t, []string{"review"},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusPending},
	)
	holdExternal(t, l)

	if err := l.dispatchControlCommand(stageComplete("review", "reject", "missing tests")); err != nil {
		t.Fatalf("stage-complete: %v", err)
	}
	pending, ok := l.cooks.pendingReview["42"]
	if !ok {
		t.Fatal("rejected stage should stay parked like a blocking stage message")
	}
	if pending.reason != "blocked by stage message: missing tests" {
		t.Fatalf("reason = %q", pending.reason)
	}
	if _, err := os.Stat(filepath.Join(l.projectDir, "brain", "reviews", "42-review.md")); err != nil {
		t.Fatalf("review not saved: %v", err)
	}
	assertVerdict(t, l.runtimeDir, "rejected", filepath.Join(l.projectDir, "brain", "reviews", "42-review.md"))
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 0 {
		t.Fatalf("rejected order advanced: %+v", rt.calls)
	}
	// A second stage-complete is refused: the stage now waits on a human.
	if err := l.controlStageComplete("42", "review", "again", false); err == nil {
		t.Fatal("expected error completing a non-external review")
	}
}

func TestStageCompleteErrors(t *testing.T) {
	l := newControlTestLoop(t, &fakeWorktree{}, newMockRuntime())
	if err := l.controlStageComplete("42", "execute", "ok", false); err == nil {
		t.Fatal("expected error for non-external pending review")
	}
	if err := l.controlStageComplete("99", "execute", "ok", false); err == nil {
		t.Fatal("expected error for unknown order")
	}
}

func TestAppendControlCommandWritesLine(t *testing.T) {
	dir := t.TempDir()
	cmd := ControlCommand{ID: "c1", Action: "stage-complete", OrderID: "42", TaskKey: "review", Prompt: "text", Value: "accept"}
	if err := AppendControlCommand(dir, cmd); err != nil {
		t.Fatalf("append: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "control.ndjson"))
	if err != nil {
		t.Fatalf("read: %v", err)
	}
	var got ControlCommand
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("parse %q: %v", data, err)
	}
	if got.ID != "c1" || got.Action != "stage-complete" || got.TaskKey != "review" || got.Value != "accept" {
		t.Fatalf("command = %+v", got)
	}
}

func assertVerdict(t *testing.T, runtimeDir, want, reviewFile string) {
	t.Helper()
	v, err := ReadExternalReviewVerdict(runtimeDir, "42")
	if err != nil {
		t.Fatalf("read verdict: %v", err)
	}
	if v.OrderID != "42" || v.StageIndex != 0 || v.TaskKey != "review" || v.Verdict != want || v.ReviewFile != reviewFile {
		t.Fatalf("verdict = %+v", v)
	}
}

func TestExternalPromptPointsAtSkillFile(t *testing.T) {
	l, _, _ := newExternalReviewTestLoop(t, []string{"execute"})
	skillDir := filepath.Join(l.projectDir, ".agents", "skills", "execute")
	if err := os.MkdirAll(skillDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(skillDir, "SKILL.md"), []byte("# execute"), 0o644); err != nil {
		t.Fatal(err)
	}
	l.config.Skills.Paths = []string{".agents/skills"}
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(l.runtimeDir, "reviews", "42-0.prompt.md"))
	if err != nil {
		t.Fatalf("read prompt: %v", err)
	}
	prompt := string(data)
	if strings.Contains(prompt, "Use Skill(execute)") {
		t.Fatalf("external prompt still asks for the Skill tool:\n%s", prompt)
	}
	if !strings.Contains(prompt, filepath.Join(skillDir, "SKILL.md")) {
		t.Fatalf("external prompt missing skill path:\n%s", prompt)
	}
	if !strings.Contains(prompt, "instead of `noodle event emit`") {
		t.Fatalf("external prompt missing stage_message override:\n%s", prompt)
	}
}
