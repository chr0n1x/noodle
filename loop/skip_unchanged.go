package loop

import (
	"os"
	"os/exec"
	"strings"

	"github.com/poteto/noodle/internal/ingest"
)

const skippedWithoutChanges = "remaining stages skipped: order has no changes"

// skipStagesWithoutChanges runs after an approved stage completes. With
// [review] skip_stages_without_changes set, an order without commits whose
// approved worktree is clean has its remaining stages skipped; it is held for
// publish so advanceAndPersist completes it with nothing to publish. reason
// gains a note saying why.
func (l *Loop) skipStagesWithoutChanges(cook *cookHandle, reason *string) error {
	if !l.config.Review.SkipStagesWithoutChanges || !l.hasRemainingStages(cook) {
		return nil
	}
	if l.orderHasCommits(cook.orderID) || worktreeDirty(cook.worktreePath) {
		return nil
	}
	if err := l.emitEventChecked(ingest.EventStageRemainingSkipped, map[string]any{
		"order_id":         cook.orderID,
		"stage_index":      cook.stageIndex + 1,
		"hold_for_publish": true,
	}); err != nil {
		return err
	}
	l.logger.Info("remaining stages skipped", "order", cook.orderID, "after_stage", cook.stageIndex)
	*reason += "; " + skippedWithoutChanges
	return nil
}

func (l *Loop) hasRemainingStages(cook *cookHandle) bool {
	order, ok := l.canonical.Orders[cook.orderID]
	if !ok {
		return false
	}
	for i := cook.stageIndex + 1; i < len(order.Stages); i++ {
		if !order.Stages[i].Status.IsTerminal() {
			return true
		}
	}
	return false
}

// worktreeDirty reports whether path holds uncommitted or untracked changes.
// A missing path is clean; a failed check counts as dirty so nothing is
// skipped on doubt.
func worktreeDirty(path string) bool {
	path = strings.TrimSpace(path)
	if path == "" {
		return false
	}
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return false
	}
	out, err := exec.Command("git", "-C", path, "status", "--porcelain").Output()
	return err != nil || strings.TrimSpace(string(out)) != ""
}
