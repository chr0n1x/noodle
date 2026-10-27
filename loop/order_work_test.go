package loop

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

// newApproveTestLoop parks stage 0 (execute) of order 42 for review; stage 1 is
// an external review stage and stage 2 runs after it.
func newApproveTestLoop(t *testing.T) (*Loop, *fakeWorktree) {
	t.Helper()
	l, _, wt := newExternalReviewTestLoop(t, []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusActive},
		Stage{TaskKey: "review", Skill: "review", Status: StageStatusPending},
		Stage{TaskKey: "execute", Skill: "execute", Status: StageStatusPending},
	)
	l.cooks.pendingReview["42"] = &pendingReviewCook{
		cookIdentity: cookIdentity{
			orderID:    "42",
			stageIndex: 0,
			stage:      Stage{TaskKey: "execute", Skill: "execute"},
		},
		worktreeName: "42-0-execute",
		worktreePath: filepath.Join(l.projectDir, ".worktrees", "42-0-execute"),
		sessionID:    "sess-42",
	}
	if err := l.writePendingReview(); err != nil {
		t.Fatalf("write pending review: %v", err)
	}
	return l, wt
}

func createFrom(wt *fakeWorktree, name string) (string, bool) {
	i := slices.Index(wt.created, name)
	if i < 0 {
		return "", false
	}
	return wt.createOpts[i].From, true
}

func TestApproveAdvancesWithoutPublishingAndKeepsBranch(t *testing.T) {
	l, wt := newApproveTestLoop(t)

	if err := l.dispatchControlCommand(ControlCommand{Action: "approve", OrderID: "42"}); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if _, ok := l.cooks.pendingReview["42"]; ok {
		t.Fatal("pending review should be removed after approve")
	}
	if len(wt.merged) != 0 || len(wt.cleaned) != 0 {
		t.Fatalf("approve must not merge or clean: merged=%v cleaned=%v", wt.merged, wt.cleaned)
	}
	if !strings.Contains(l.cmds.ackMessage, "42-0-execute") {
		t.Fatalf("ack = %q, want it to name the kept branch", l.cmds.ackMessage)
	}
	orders, err := readOrders(l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if got := orders.Orders[0].Stages[0].Status; got != StageStatusCompleted {
		t.Fatalf("stage 0 status = %q, want completed", got)
	}

	holdExternal(t, l)
	if from, ok := createFrom(wt, "42-1-review"); !ok || from != "42-0-execute" {
		t.Fatalf("review worktree from = %q (created %v), want 42-0-execute", from, ok)
	}
}

func TestAcceptWithKeptWorkKeepsReviewBranchAndChainsNextStage(t *testing.T) {
	l, wt := newApproveTestLoop(t)
	if err := l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	holdExternal(t, l)

	if err := l.dispatchControlCommand(stageComplete("review", "accept", "looks good")); err != nil {
		t.Fatalf("stage-complete: %v", err)
	}
	if len(wt.cleaned) != 0 {
		t.Fatalf("accept must not discard unpublished work: cleaned=%v", wt.cleaned)
	}
	if !strings.Contains(l.cmds.ackMessage, "unpublished work kept at 42-1-review") {
		t.Fatalf("ack = %q", l.cmds.ackMessage)
	}

	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if from, ok := createFrom(wt, "42-2-execute"); !ok || from != "42-1-review" {
		t.Fatalf("next stage worktree from = %q (created %v), want 42-1-review", from, ok)
	}
}

func TestRejectDiscardsKeptWork(t *testing.T) {
	l, wt := newApproveTestLoop(t)
	if err := l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	holdExternal(t, l)

	if err := l.controlReject("42"); err != nil {
		t.Fatalf("reject: %v", err)
	}
	for _, name := range []string{"42-0-execute", "42-1-review"} {
		if !slices.Contains(wt.cleaned, name) {
			t.Fatalf("cleaned = %v, want %s", wt.cleaned, name)
		}
	}
	if _, ok := l.orderWorkBranch("42"); ok {
		t.Fatal("kept work record should be gone after reject")
	}
}

func TestMergeReleasesKeptWork(t *testing.T) {
	l, wt := newApproveTestLoop(t)
	wt.hasUnmergedCommits = map[string]bool{"42-1-review": true}
	if err := l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	holdExternal(t, l)

	if err := mergePending(l, "42"); err != nil {
		t.Fatalf("accept with merge: %v", err)
	}
	if !slices.Contains(wt.merged, "42-1-review") {
		t.Fatalf("merged = %v, want 42-1-review", wt.merged)
	}
	if !slices.Contains(wt.cleaned, "42-0-execute") {
		t.Fatalf("cleaned = %v, want the kept execute branch", wt.cleaned)
	}
	if _, ok := l.orderWorkBranch("42"); ok {
		t.Fatal("kept work record should be gone after merge")
	}
}

func TestOrderFinishedWithKeptWorkEmitsNotice(t *testing.T) {
	l := newControlTestLoop(t, &fakeWorktree{}, newMockRuntime())

	if err := l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(l.runtimeDir, "loop-events.ndjson"))
	if err != nil {
		t.Fatalf("read loop events: %v", err)
	}
	if !strings.Contains(string(data), `"order.work_kept"`) || !strings.Contains(string(data), "42-0-execute") {
		t.Fatalf("loop events missing work kept notice:\n%s", data)
	}
}
