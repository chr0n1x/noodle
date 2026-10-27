package loop

import (
	"strings"
	"testing"
)

func assertPublishBlock(t *testing.T, l *Loop, orderID string, want ...string) {
	t.Helper()
	state := l.buildLoopStateSnapshot()
	if !state.PublishGated {
		t.Fatalf("PublishGated = false, want true")
	}
	reason := state.PublishBlocks[orderID]
	if reason == "" {
		t.Fatalf("PublishBlocks[%s] empty, want reason; blocks=%v", orderID, state.PublishBlocks)
	}
	err := l.controlApproveAndPublish(orderID)
	if err == nil || err.Error() != reason {
		t.Fatalf("ack error %v differs from snapshot reason %q", err, reason)
	}
	for _, w := range want {
		if !strings.Contains(reason, w) {
			t.Fatalf("reason %q missing %q", reason, w)
		}
	}
}

func TestPublishBlocksEmptyWhenAwaitingPublish(t *testing.T) {
	g := newAwaitingPublishLoop(t, "draft-pr", "42-0-execute")
	state := g.l.buildLoopStateSnapshot()
	if !state.PublishGated {
		t.Fatalf("PublishGated = false, want true")
	}
	if reason, ok := state.PublishBlocks["42"]; ok {
		t.Fatalf("PublishBlocks[42] = %q, want none", reason)
	}
}

func TestPublishBlocksListIncompleteStages(t *testing.T) {
	g := newGateTestLoopPublishing(t, "supervised", nil, "draft-pr",
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusCompleted},
		Stage{TaskKey: "quality", Skill: "quality", Provider: "claude", Status: StageStatusPending},
	)
	assertPublishBlock(t, g.l, "42", "not awaiting publish", "1:quality:pending")
}

func TestPublishBlocksNameExternalHold(t *testing.T) {
	g := newGateTestLoop(t, "auto", []string{"review"},
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
		Stage{TaskKey: "review", Skill: "review", Provider: "claude", Status: StageStatusPending},
	)
	g.runStage(t)
	holdExternal(t, g.l)
	assertPublishBlock(t, g.l, "42", "noodle order 42 complete review")
}

func TestPublishBlocksWhilePublishRunning(t *testing.T) {
	g := newAwaitingPublishLoop(t, "ship-it", "42-0-execute", "42-1-review")
	withPublishSkill(t, g, "ship-it")
	startSkillPublish(t, g)
	assertPublishBlock(t, g.l, "42", "publish already running")
}

func TestPublishBlocksAbsentWhenUngated(t *testing.T) {
	g := newGateTestLoop(t, "supervised", nil,
		Stage{TaskKey: "execute", Skill: "execute", Provider: "claude", Status: StageStatusPending},
	)
	state := g.l.buildLoopStateSnapshot()
	if state.PublishGated || state.PublishBlocks != nil {
		t.Fatalf("ungated state gated=%v blocks=%v", state.PublishGated, state.PublishBlocks)
	}
}
