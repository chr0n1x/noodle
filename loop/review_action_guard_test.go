package loop

import (
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
)

// guardTestLoop builds order 42 with external_stages quality and
// adversarial-review. When parkIndex >= 0 the stage at that index is parked
// as a supervised (non-external) review; otherwise the loop cycles so the
// first pending external stage is held.
func guardTestLoop(t *testing.T, parkIndex int, stages ...Stage) (*Loop, *fakeWorktree, *fakeDraftPR) {
	t.Helper()
	l, _, wt := newExternalReviewTestLoop(t, []string{"quality", "adversarial-review"}, stages...)
	withReviewSkills(l)
	pr := &fakeDraftPR{}
	l.deps.DraftPR = pr
	wt.hasUnmergedCommits = map[string]bool{}
	for i, s := range stages {
		wt.hasUnmergedCommits[cookBaseName("42", i, s.TaskKey)] = true
	}
	if parkIndex < 0 {
		holdExternal(t, l)
		return l, wt, pr
	}
	s := stages[parkIndex]
	cook := &cookHandle{
		cookIdentity: cookIdentity{orderID: "42", stageIndex: parkIndex, stage: s},
		session:      &adoptedSession{id: "sess-42", status: "completed"},
		worktreeName: cookBaseName("42", parkIndex, s.TaskKey),
		worktreePath: filepath.Join(l.projectDir, ".worktrees", cookBaseName("42", parkIndex, s.TaskKey)),
	}
	if err := l.parkPendingReview(cook, "supervised review"); err != nil {
		t.Fatalf("park: %v", err)
	}
	return l, wt, pr
}

type guardState struct {
	orders  OrdersFile
	pending pendingReviewCook
	work    bool
}

func captureGuardState(t *testing.T, l *Loop) guardState {
	t.Helper()
	orders, err := l.currentOrders()
	if err != nil {
		t.Fatalf("orders: %v", err)
	}
	pending, ok := l.cooks.pendingReview["42"]
	if !ok {
		t.Fatal("order 42 is not pending review")
	}
	_, statErr := os.Stat(filepath.Join(l.runtimeDir, "order-work.json"))
	return guardState{orders: orders, pending: *pending, work: statErr == nil}
}

func reviewStagesAfterExecute() []Stage {
	return []Stage{
		{TaskKey: "execute", Skill: "execute", Status: StageStatusActive},
		{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
		{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusPending},
	}
}

func TestExternalHoldRefusesApprove(t *testing.T) {
	l, wt, pr := guardTestLoop(t, -1,
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
		Stage{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusPending},
	)
	before := captureGuardState(t, l)
	ack := l.applyControlCommand(ControlCommand{ID: "c1", Action: "approve", OrderID: "42"})
	if ack.Status != "error" {
		t.Fatalf("approve ack = %+v, want error", ack)
	}
	for _, want := range []string{"approve refused", "noodle order 42 complete quality", "stage-complete"} {
		if !strings.Contains(ack.Message, want) {
			t.Fatalf("ack message = %q, want %q", ack.Message, want)
		}
	}
	if after := captureGuardState(t, l); !reflect.DeepEqual(before, after) {
		t.Fatalf("approve changed state:\nbefore %+v\nafter  %+v", before, after)
	}
	if len(wt.merged) != 0 || len(pr.requests) != 0 {
		t.Fatalf("approve published: merged %v, PR requests %d", wt.merged, len(pr.requests))
	}
}

func TestParkedStageBeforeReviewsAllowsPlainApprove(t *testing.T) {
	l, wt, pr := guardTestLoop(t, 0, reviewStagesAfterExecute()...)

	ack := l.applyControlCommand(ControlCommand{ID: "c1", Action: "approve", OrderID: "42"})
	if ack.Status != "ok" {
		t.Fatalf("approve ack = %+v, want ok", ack)
	}
	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("approved stage should leave pending review")
	}
	if len(wt.merged) != 0 || len(pr.requests) != 0 {
		t.Fatalf("approve published: merged %v, PR requests %d", wt.merged, len(pr.requests))
	}
}

func TestLoopStateReportsBlockedReviewActions(t *testing.T) {
	held, _, _ := guardTestLoop(t, -1,
		Stage{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
	)
	block, ok := held.buildLoopStateSnapshot().ReviewBlocks["42"]
	if !ok {
		t.Fatal("no review block for the held order")
	}
	if !reflect.DeepEqual(block.Actions, []string{"approve", "merge"}) || !strings.Contains(block.Reason, "noodle order 42 complete quality") {
		t.Fatalf("block = %+v", block)
	}

	parked, _, _ := guardTestLoop(t, 0, reviewStagesAfterExecute()...)
	block, ok = parked.buildLoopStateSnapshot().ReviewBlocks["42"]
	if !ok || !reflect.DeepEqual(block.Actions, []string{"merge"}) || block.Reason != gatedMergeBlockReason {
		t.Fatalf("gated supervised park block = %+v (ok=%v), want merge blocked", block, ok)
	}

	ungated, _, _ := newExternalReviewTestLoop(t, nil, reviewStagesAfterExecute()...)
	cook := &cookHandle{
		cookIdentity: cookIdentity{orderID: "42", stageIndex: 0, stage: reviewStagesAfterExecute()[0]},
		session:      &adoptedSession{id: "sess-42", status: "completed"},
		worktreeName: cookBaseName("42", 0, "execute"),
	}
	if err := ungated.parkPendingReview(cook, "supervised review"); err != nil {
		t.Fatalf("park: %v", err)
	}
	if block, ok := ungated.buildLoopStateSnapshot().ReviewBlocks["42"]; ok {
		t.Fatalf("ungated park blocked: %+v", block)
	}
}

func qualityHeldBeforeAdversarial(t *testing.T) (*Loop, *fakeWorktree, *fakeDraftPR) {
	t.Helper()
	return guardTestLoop(t, -1,
		Stage{TaskKey: "quality", Skill: "quality", Status: StageStatusPending},
		Stage{TaskKey: "adversarial-review", Skill: "adversarial-review", Status: StageStatusPending},
	)
}
