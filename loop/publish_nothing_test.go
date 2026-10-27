package loop

import (
	"errors"
	"strings"
	"testing"
)

func noCommits(wt *fakeWorktree, names ...string) {
	wt.hasUnmergedCommits = map[string]bool{}
	for _, n := range names {
		wt.hasUnmergedCommits[n] = false
	}
}

func assertNothingPublished(t *testing.T, g gateTestLoop, pr *fakeDraftPR) {
	t.Helper()
	assertPublishedCompleted(t, g)
	if len(g.wt.merged) != 0 {
		t.Fatalf("merged = %v, want none", g.wt.merged)
	}
	if pr != nil && len(pr.requests) != 0 {
		t.Fatalf("draft PR requests = %d, want none", len(pr.requests))
	}
	if stages := publishStages(t, g.l); len(stages) != 0 {
		t.Fatalf("publish stage appended: %+v", stages)
	}
	if !strings.Contains(g.loopEvents(t), "nothing to publish") {
		t.Fatalf("completion does not record nothing to publish:\n%s", g.loopEvents(t))
	}
}

func TestAwaitingPublishWithoutCommitsClosesOnApproveAndPublish(t *testing.T) {
	for _, action := range []string{"", "draft-pr", "ship-it"} {
		t.Run("publish_action="+action, func(t *testing.T) {
			g := newAwaitingPublishLoop(t, action, "42-0-execute", "42-1-review")
			if action == "ship-it" {
				withPublishSkill(t, g, "ship-it")
			}
			pr := &fakeDraftPR{err: errors.New("No commits between main and noodle/42")}
			g.l.deps.DraftPR = pr
			noCommits(g.wt, "42-0-execute", "42-1-review")

			ack := g.l.applyControlCommand(approveAndPublish("42"))
			if ack.Status != "ok" || !strings.Contains(ack.Message, "nothing to publish") {
				t.Fatalf("ack = %+v", ack)
			}
			if msg, ok := ReadPublishErrors(g.l.runtimeDir)["42"]; ok {
				t.Fatalf("publish error recorded: %q", msg)
			}
			assertNothingPublished(t, g, pr)
		})
	}
}

func TestLastStageWithoutCommitsCompletesInsteadOfAwaitingPublish(t *testing.T) {
	g := newGateTestLoop(t, "supervised", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	noCommits(g.wt, "42-0-execute")
	if err := g.l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if strings.Contains(g.loopEvents(t), `"order.awaiting_publish"`) {
		t.Fatalf("no-commit order entered awaiting-publish:\n%s", g.loopEvents(t))
	}
	assertNothingPublished(t, g, nil)
}

func TestPublishCheckErrorStillPublishes(t *testing.T) {
	g := newAwaitingPublishLoop(t, "", "42-0-execute", "42-1-review")
	g.wt.cherryErr = errors.New("git cherry failed")
	if ack := g.l.applyControlCommand(approveAndPublish("42")); ack.Status != "ok" {
		t.Fatalf("ack = %+v", ack)
	}
	if len(g.wt.merged) != 1 || g.wt.merged[0] != "42-1-review" {
		t.Fatalf("merged = %v, want [42-1-review]", g.wt.merged)
	}
}
