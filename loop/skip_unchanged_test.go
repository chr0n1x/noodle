package loop

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/poteto/noodle/internal/state"
)

// newSkipUnchangedLoop parks order 42's execute stage for approval in a gated,
// supervised loop; meditate and reflect follow it.
func newSkipUnchangedLoop(t *testing.T, skip bool) gateTestLoop {
	t.Helper()
	g := newGateTestLoop(t, "supervised", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "meditate", Skill: "meditate", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "reflect", Skill: "reflect", Provider: "claude", Status: StageStatusPending},
	)
	g.l.config.Review.SkipStagesWithoutChanges = skip
	g.runStage(t)
	if _, parked := g.l.cooks.pendingReview["42"]; !parked {
		t.Fatal("execute stage should park for approve")
	}
	return g
}

// gitWorktreeAt makes the parked stage's worktree path a git repo, with an
// untracked file when dirty.
func gitWorktreeAt(t *testing.T, g gateTestLoop, dirty bool) {
	t.Helper()
	path := g.l.cooks.pendingReview["42"].worktreePath
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if out, err := exec.Command("git", "-C", path, "init", "-q").CombinedOutput(); err != nil {
		t.Fatalf("git init: %v\n%s", err, out)
	}
	if dirty {
		if err := os.WriteFile(filepath.Join(path, "wip.txt"), []byte("wip\n"), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func approve(g gateTestLoop) ControlAck {
	return g.l.applyControlCommand(ControlCommand{ID: "approve-42", Action: "approve", OrderID: "42"})
}

func assertLaterStagesPending(t *testing.T, g gateTestLoop) {
	t.Helper()
	orders, err := readOrders(g.l.deps.OrdersFile)
	if err != nil {
		t.Fatalf("read orders: %v", err)
	}
	if len(orders.Orders) != 1 {
		t.Fatalf("orders.json = %+v, want order 42", orders.Orders)
	}
	stages := orders.Orders[0].Stages
	if len(stages) != 3 || stages[1].Status != StageStatusPending || stages[2].Status != StageStatusPending {
		t.Fatalf("later stages = %q, %q, want pending", stages[1].Status, stages[2].Status)
	}
	if got := g.l.canonical.Orders["42"].Status; got != state.OrderActive {
		t.Fatalf("canonical status = %q, want active", got)
	}
	if len(g.ar.doneCalls) != 0 {
		t.Fatalf("done ran: %v", g.ar.doneCalls)
	}
}

func TestApproveWithoutChangesSkipsRemainingStages(t *testing.T) {
	g := newSkipUnchangedLoop(t, true)
	noCommits(g.wt, "42-0-execute")
	gitWorktreeAt(t, g, false)

	ack := approve(g)
	if ack.Status != "ok" || !strings.Contains(ack.Message, "remaining stages skipped") || !strings.Contains(ack.Message, "no changes") {
		t.Fatalf("ack = %+v, want it to say the remaining stages were skipped for no changes", ack)
	}
	if order, ok := g.l.canonical.Orders["42"]; ok {
		if order.Stages[1].Status != state.StageSkipped || order.Stages[2].Status != state.StageSkipped {
			t.Fatalf("canonical stages = %+v, want 1 and 2 skipped", order.Stages)
		}
	}
	assertPublishedCompleted(t, g, "42-0-execute")
	if !strings.Contains(g.loopEvents(t), "nothing to publish") {
		t.Fatalf("completion does not record nothing to publish:\n%s", g.loopEvents(t))
	}

	if err := g.l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	for _, name := range []string{"42-1-meditate", "42-2-reflect"} {
		if slices.Contains(g.wt.created, name) {
			t.Fatalf("skipped stage dispatched a cook: created=%v", g.wt.created)
		}
	}
}

func TestApproveWithCommitsKeepsRemainingStages(t *testing.T) {
	g := newSkipUnchangedLoop(t, true)
	gitWorktreeAt(t, g, false)
	if ack := approve(g); ack.Status != "ok" || strings.Contains(ack.Message, "skipped") {
		t.Fatalf("ack = %+v", ack)
	}
	assertLaterStagesPending(t, g)
}

func TestApproveWithUncommittedChangesKeepsRemainingStages(t *testing.T) {
	g := newSkipUnchangedLoop(t, true)
	noCommits(g.wt, "42-0-execute")
	gitWorktreeAt(t, g, true)
	if ack := approve(g); ack.Status != "ok" || strings.Contains(ack.Message, "skipped") {
		t.Fatalf("ack = %+v", ack)
	}
	assertLaterStagesPending(t, g)
}

func TestApproveWithoutChangesFlagOffKeepsRemainingStages(t *testing.T) {
	g := newSkipUnchangedLoop(t, false)
	noCommits(g.wt, "42-0-execute")
	gitWorktreeAt(t, g, false)
	if ack := approve(g); ack.Status != "ok" || strings.Contains(ack.Message, "skipped") {
		t.Fatalf("ack = %+v", ack)
	}
	assertLaterStagesPending(t, g)
}
