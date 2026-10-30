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

type outcomeFixture struct {
	t  *testing.T
	l  *Loop
	wt *fakeWorktree
}

func newOutcomeFixture(t *testing.T, stages []Stage, actions map[string]config.StageOutcomeConfig) *outcomeFixture {
	t.Helper()
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	ordersPath := filepath.Join(runtimeDir, "orders.json")
	if err := writeOrdersAtomic(ordersPath, OrdersFile{Orders: []Order{{
		ID: "order-1", Title: "test", Status: OrderStatusActive, Stages: stages,
	}}}); err != nil {
		t.Fatalf("write orders: %v", err)
	}
	cfg := config.DefaultConfig()
	cfg.Stages = actions
	wt := &fakeWorktree{}
	l := New(projectDir, "noodle", cfg, Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": newMockRuntime()},
		Worktree:   wt,
		Adapter:    &fakeAdapterRunner{},
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: ordersPath,
	})
	return &outcomeFixture{t: t, l: l, wt: wt}
}

// finishWithMessage records a stage_message from the cook of stage index,
// then ends it with result.
func (f *outcomeFixture) finishWithMessage(index int, result StageResultStatus, message string, blocking bool) {
	f.t.Helper()
	orders, err := f.l.currentOrders()
	if err != nil {
		f.t.Fatalf("read orders: %v", err)
	}
	name := worktreeNameFor("order-1", index, orders.Orders[0].Stages[index].TaskKey)
	writer, err := event.NewEventWriter(f.l.runtimeDir, name+"-session")
	if err != nil {
		f.t.Fatalf("event writer: %v", err)
	}
	payload, _ := json.Marshal(event.StageMessagePayload{Message: message, Blocking: &blocking})
	if err := writer.Append(context.Background(), event.Event{Type: event.EventStageMessage, Payload: payload}); err != nil {
		f.t.Fatalf("append stage_message: %v", err)
	}
	f.finish(index, result)
}

// finish ends the cook of stage index with result, as a cook exit would.
func (f *outcomeFixture) finish(index int, result StageResultStatus) {
	f.t.Helper()
	orders, err := f.l.currentOrders()
	if err != nil {
		f.t.Fatalf("read orders: %v", err)
	}
	stage := orders.Orders[0].Stages[index]
	name := worktreeNameFor("order-1", index, stage.TaskKey)
	cook := &cookHandle{
		cookIdentity: cookIdentity{orderID: "order-1", stageIndex: index, stage: stage},
		orderStatus:  OrderStatusActive,
		worktreeName: name,
		worktreePath: f.l.worktreePath(name),
		session:      &mockSession{id: name + "-session", status: string(result), done: make(chan struct{})},
	}
	if err := f.l.handleCompletion(context.Background(), cook, result, string(result)); err != nil {
		f.t.Fatalf("handleCompletion: %v", err)
	}
}

func worktreeNameFor(orderID string, index int, task string) string {
	return orderID + "-" + string(rune('0'+index)) + "-" + task
}

func (f *outcomeFixture) order() state.OrderNode {
	f.t.Helper()
	order, ok := f.l.canonical.Orders["order-1"]
	if !ok {
		f.t.Fatal("order-1 missing from canonical state")
	}
	return order
}

func (f *outcomeFixture) legacyStages() []Stage {
	f.t.Helper()
	orders, err := f.l.currentOrders()
	if err != nil {
		f.t.Fatalf("read orders: %v", err)
	}
	for _, o := range orders.Orders {
		if o.ID == "order-1" {
			return o.Stages
		}
	}
	return nil
}

func (f *outcomeFixture) wantStatuses(order state.OrderLifecycleStatus, stages ...state.StageLifecycleStatus) {
	f.t.Helper()
	got := f.order()
	if got.Status != order {
		f.t.Fatalf("order status = %q, want %q", got.Status, order)
	}
	if len(got.Stages) != len(stages) {
		f.t.Fatalf("stages = %d, want %d", len(got.Stages), len(stages))
	}
	for i, want := range stages {
		if got.Stages[i].Status != want {
			f.t.Fatalf("stage %d (%s) status = %q, want %q", i, got.Stages[i].TaskKey, got.Stages[i].Status, want)
		}
	}
}

func pendingStages(keys ...string) []Stage {
	out := make([]Stage, len(keys))
	for i, k := range keys {
		out[i] = Stage{TaskKey: k, Skill: k, Status: StageStatusPending}
	}
	out[0].Status = StageStatusActive
	return out
}

func TestRejectPromoteContinuesAndFailsOrderAtEnd(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute":            {Reject: "promote"},
		"adversarial-review": {Accept: "promote"},
	})
	f.finish(0, StageResultFailed)
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StagePending)
	outs := stageOutcomes(f.legacyStages()[0])
	if len(outs) != 1 || outs[0].State != string(config.StageStateReject) || outs[0].Action != string(config.StageActionPromote) ||
		!strings.Contains(outs[0].Feedback, "cook exited") {
		t.Fatalf("outcomes = %+v", outs)
	}
	if branch, ok := f.l.orderWorkBranch("order-1"); !ok || branch != "order-1-0-execute" {
		t.Fatalf("kept work = %q, %v; want the execute branch", branch, ok)
	}

	f.finish(1, StageResultCompleted)
	f.wantStatuses(state.OrderFailed, state.StageCompleted, state.StageCompleted)
}

func TestAcceptPromoteSkipsSupervisedPark(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute": {Accept: "promote"},
	})
	f.wt.hasUnmergedCommits = map[string]bool{"order-1-0-execute": true}
	f.finish(0, StageResultCompleted)
	if _, parked := f.l.cooks.pendingReview["order-1"]; parked {
		t.Fatal("accept promote should not park")
	}
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StagePending)
	if len(f.wt.merged) != 0 {
		t.Fatalf("promote merged %v; it should keep the work", f.wt.merged)
	}
	if _, ok := f.l.orderWorkBranch("order-1"); !ok {
		t.Fatal("promote should keep the stage branch")
	}
}

func TestUnsetActionKeepsSupervisedPark(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), nil)
	f.wt.hasUnmergedCommits = map[string]bool{"order-1-0-execute": true}
	f.finish(0, StageResultCompleted)
	if _, parked := f.l.cooks.pendingReview["order-1"]; !parked {
		t.Fatal("a stage with changes should park in supervised mode")
	}
	if outs := stageOutcomes(f.legacyStages()[0]); len(outs) != 0 {
		t.Fatalf("unset action recorded outcomes %+v", outs)
	}
}

func TestRejectParkParksFailedCook(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute": {Reject: "park"},
	})
	f.finish(0, StageResultFailed)
	pending, parked := f.l.cooks.pendingReview["order-1"]
	if !parked {
		t.Fatal("reject park should park the stage")
	}
	if !strings.HasPrefix(pending.reason, "rejected") {
		t.Fatalf("park reason = %q", pending.reason)
	}
	if f.order().Status == state.OrderFailed {
		t.Fatal("a parked stage should not fail the order")
	}
}

func TestRejectExitSkipsWorkAndFailsOrderKeepingWorktree(t *testing.T) {
	stages := pendingStages("execute", "adversarial-review", "document")
	stages[0].Status = StageStatusCompleted
	stages[1].Status = StageStatusActive
	f := newOutcomeFixture(t, stages, map[string]config.StageOutcomeConfig{
		"adversarial-review": {Reject: "exit"},
	})
	f.finish(1, StageResultFailed)
	f.wantStatuses(state.OrderFailed, state.StageCompleted, state.StageCompleted, state.StageSkipped)
	for _, c := range f.wt.cleaned {
		if c == "order-1-1-adversarial-review" {
			t.Fatal("reject exit should keep the worktree")
		}
	}
}

func TestAcceptExitSkipsRemainingAndCompletesOrder(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute": {Accept: "exit"},
	})
	f.finish(0, StageResultCompleted)
	f.wantStatuses(state.OrderCompleted, state.StageCompleted, state.StageSkipped)
}

func TestRejectRetryRerunsStageWithFeedback(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute": {Reject: "retry"},
	})
	f.finish(0, StageResultFailed)
	f.wantStatuses(state.OrderActive, state.StagePending, state.StagePending)
	s := f.legacyStages()[0]
	if !strings.Contains(s.Prompt, "cook exited") || changesRequested(s) != 1 {
		t.Fatalf("retried stage prompt = %q, rounds = %d", s.Prompt, changesRequested(s))
	}
	for _, c := range f.wt.cleaned {
		if c == "order-1-0-execute" {
			t.Fatal("retry should keep the worktree")
		}
	}
}

func TestRetryAtLimitFailsOrderKeepingWorktree(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute": {Reject: "retry"},
	})
	zero := 0
	f.l.config.Review.MaxRework = &zero
	f.finish(0, StageResultFailed)
	f.wantStatuses(state.OrderFailed, state.StageFailed, state.StagePending)
	if len(f.wt.cleaned) != 0 {
		t.Fatalf("cleaned %v; the worktree should be kept", f.wt.cleaned)
	}
}

func TestRejectRetryPreviousReworksStageBefore(t *testing.T) {
	stages := pendingStages("execute", "adversarial-review")
	stages[0].Status = StageStatusCompleted
	stages[1].Status = StageStatusActive
	f := newOutcomeFixture(t, stages, map[string]config.StageOutcomeConfig{
		"adversarial-review": {Reject: "retry-previous"},
	})
	f.finish(1, StageResultFailed)
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StageCompleted, state.StagePending, state.StagePending)
	got := f.legacyStages()
	if got[2].TaskKey != "execute" || !strings.Contains(got[2].Prompt, "cook exited") {
		t.Fatalf("rework stage = %+v", got[2])
	}
	if got[3].TaskKey != "adversarial-review" {
		t.Fatalf("stage after rework = %q, want a copy of adversarial-review", got[3].TaskKey)
	}

	// The rework attempt's accept supersedes the earlier reject.
	f.l.config.Stages["adversarial-review"] = config.StageOutcomeConfig{Accept: "promote", Reject: "retry-previous"}
	f.l.config.Stages["execute"] = config.StageOutcomeConfig{Accept: "promote"}
	f.finish(2, StageResultCompleted)
	f.finishWithMessage(3, StageResultCompleted, "PASS: no high findings", false)
	f.wantStatuses(state.OrderCompleted, state.StageCompleted, state.StageCompleted, state.StageCompleted, state.StageCompleted)
}

func TestRetryPreviousAtLimitFailsOrder(t *testing.T) {
	stages := pendingStages("execute", "adversarial-review")
	stages[0].Status = StageStatusCompleted
	stages[1].Status = StageStatusActive
	f := newOutcomeFixture(t, stages, map[string]config.StageOutcomeConfig{
		"adversarial-review": {Reject: "retry-previous"},
	})
	zero := 0
	f.l.config.Review.MaxRework = &zero
	f.finish(1, StageResultFailed)
	f.wantStatuses(state.OrderFailed, state.StageCompleted, state.StageFailed)
}

func TestCancelledCookIgnoresConfiguredAction(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute": {Reject: "promote"},
	})
	f.finish(0, StageResultCancelled)
	if f.order().Status != state.OrderFailed {
		t.Fatalf("order status = %q, want failed", f.order().Status)
	}
}

func TestOutcomeContextListsVerdictsUpToStage(t *testing.T) {
	stages := []Stage{
		{TaskKey: "execute", Extra: map[string]json.RawMessage{state.OutcomesExtraKey: json.RawMessage(
			`[{"state":"reject","action":"retry","feedback":"no test"},{"state":"accept","action":"promote"}]`)}},
		{TaskKey: "adversarial-review", Extra: map[string]json.RawMessage{state.OutcomesExtraKey: json.RawMessage(
			`[{"state":"reject","action":"retry-previous","feedback":"you hallucinated the API"}]`)}},
		{TaskKey: "execute"},
		{TaskKey: "later", Extra: map[string]json.RawMessage{state.OutcomesExtraKey: json.RawMessage(`[{"state":"accept","action":"promote"}]`)}},
	}
	got := outcomeContext(stages, 2)
	for _, want := range []string{
		"execute (stage 1): reject, retry: no test",
		"execute (stage 1): accept, promote",
		"adversarial-review (stage 2): reject, retry-previous: you hallucinated the API",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("context missing %q:\n%s", want, got)
		}
	}
	if strings.Contains(got, "later") {
		t.Errorf("context lists a later stage:\n%s", got)
	}
	if outcomeContext(stages[2:3], 0) != "" {
		t.Error("no outcomes should give no context")
	}
}

func resolveCmd(stage, st, do, feedback string) ControlCommand {
	return ControlCommand{ID: "r", Action: "resolve", OrderID: "order-1", TaskKey: stage, State: st, Do: do, Prompt: feedback}
}

// parkedAt parks the stage at index through a configured reject park, so the
// resolve tests start from a parked stage.
func parkedAt(t *testing.T, stages []Stage, index int) *outcomeFixture {
	t.Helper()
	task := stages[index].TaskKey
	f := newOutcomeFixture(t, stages, map[string]config.StageOutcomeConfig{task: {Reject: "park"}})
	f.finish(index, StageResultFailed)
	if _, parked := f.l.cooks.pendingReview["order-1"]; !parked {
		t.Fatal("setup: stage did not park")
	}
	return f
}

func TestResolveAcceptPromoteAdvancesParkedStage(t *testing.T) {
	f := parkedAt(t, pendingStages("execute", "adversarial-review"), 0)
	ack := f.l.applyControlCommand(resolveCmd("execute", "accept", "promote", "looks fine"))
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	if _, parked := f.l.cooks.pendingReview["order-1"]; parked {
		t.Fatal("resolve should clear the parked item")
	}
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StagePending)
	outs := stageOutcomes(f.legacyStages()[0])
	if len(outs) != 2 || outs[1].State != "accept" || outs[1].Action != "promote" || outs[1].Feedback != "looks fine" {
		t.Fatalf("outcomes = %+v", outs)
	}
}

func TestResolveRejectRetryPreviousReworks(t *testing.T) {
	stages := pendingStages("execute", "adversarial-review")
	stages[0].Status = StageStatusCompleted
	stages[1].Status = StageStatusActive
	f := parkedAt(t, stages, 1)
	ack := f.l.applyControlCommand(resolveCmd("adversarial-review", "reject", "retry-previous", "you hallucinated the API"))
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	got := f.legacyStages()
	if len(got) != 4 || got[2].TaskKey != "execute" || !strings.Contains(got[2].Prompt, "you hallucinated the API") {
		t.Fatalf("stages = %v", taskKeys(got))
	}
}

func TestResolveRejectExitFailsOrderWithFeedback(t *testing.T) {
	f := parkedAt(t, pendingStages("execute", "adversarial-review"), 0)
	ack := f.l.applyControlCommand(resolveCmd("execute", "reject", "exit", "not possible per the docs"))
	if ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	f.wantStatuses(state.OrderFailed, state.StageCompleted, state.StageSkipped)
	rejected, ok := state.RejectedThrough(f.order())
	if !ok || rejected.Feedback != "not possible per the docs" {
		t.Fatalf("rejected = %+v, %v", rejected, ok)
	}
}

func TestResolveRefusesWrongStageAndBadNames(t *testing.T) {
	f := parkedAt(t, pendingStages("execute", "adversarial-review"), 0)
	for _, c := range []struct {
		cmd  ControlCommand
		want string
	}{
		{resolveCmd("adversarial-review", "accept", "promote", ""), "parked at stage execute"},
		{resolveCmd("execute", "maybe", "promote", ""), "state"},
		{resolveCmd("execute", "accept", "retry-last", ""), "action"},
	} {
		ack := f.l.applyControlCommand(c.cmd)
		if ack.Status == "ok" || !strings.Contains(ack.Message, c.want) {
			t.Errorf("%+v: ack = %+v, want an error naming %q", c.cmd, ack, c.want)
		}
	}
	if _, parked := f.l.cooks.pendingReview["order-1"]; !parked {
		t.Fatal("a refused resolve should leave the stage parked")
	}
}

func TestApproveRecordsAcceptPromote(t *testing.T) {
	f := parkedAt(t, pendingStages("execute", "adversarial-review"), 0)
	if ack := f.l.applyControlCommand(ControlCommand{ID: "a", Action: "approve", OrderID: "order-1"}); ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	outs := stageOutcomes(f.legacyStages()[0])
	if len(outs) != 2 || outs[1].State != "accept" || outs[1].Action != "promote" {
		t.Fatalf("outcomes = %+v", outs)
	}
}

func reviewWithVerdictRequired(t *testing.T) *outcomeFixture {
	t.Helper()
	stages := pendingStages("execute", "adversarial-review")
	stages[0].Status = StageStatusCompleted
	stages[1].Status = StageStatusActive
	return newOutcomeFixture(t, stages, map[string]config.StageOutcomeConfig{
		"adversarial-review": {Accept: "promote", Reject: "retry-previous"},
	})
}

func TestMissingVerdictRetriesStageWithReminder(t *testing.T) {
	f := reviewWithVerdictRequired(t)
	f.finish(1, StageResultCompleted)
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StagePending)
	s := f.legacyStages()[1]
	if !strings.Contains(s.Prompt, "without a verdict") || changesRequested(s) != 1 {
		t.Fatalf("retried review prompt = %q, rounds = %d", s.Prompt, changesRequested(s))
	}
	if outs := stageOutcomes(s); len(outs) != 0 {
		t.Fatalf("a missing verdict recorded outcomes %+v", outs)
	}
}

func TestMissingVerdictAtLimitParks(t *testing.T) {
	f := reviewWithVerdictRequired(t)
	zero := 0
	f.l.config.Review.MaxRework = &zero
	f.finish(1, StageResultCompleted)
	pending, parked := f.l.cooks.pendingReview["order-1"]
	if !parked || !strings.Contains(pending.reason, "without a verdict") {
		t.Fatalf("pending = %+v, parked = %v; want a park naming the missing verdict", pending, parked)
	}
	if f.order().Status != state.OrderActive {
		t.Fatalf("order status = %q, want active while parked", f.order().Status)
	}
}

func TestVerdictMessageAcceptsAndPromotes(t *testing.T) {
	f := reviewWithVerdictRequired(t)
	f.finishWithMessage(1, StageResultCompleted, "PASS: no high findings", false)
	f.wantStatuses(state.OrderCompleted, state.StageCompleted, state.StageCompleted)
}

func TestMissingVerdictWithoutRejectStillAccepts(t *testing.T) {
	f := newOutcomeFixture(t, pendingStages("execute", "adversarial-review"), map[string]config.StageOutcomeConfig{
		"execute": {Accept: "promote"},
	})
	f.finish(0, StageResultCompleted)
	f.wantStatuses(state.OrderActive, state.StageCompleted, state.StagePending)
}
