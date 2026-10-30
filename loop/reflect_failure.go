package loop

import (
	"context"

	"github.com/poteto/noodle/internal/ingest"
)

const reflectTaskKey = "reflect"

// skipFailedReflect skips a reflect stage whose cook failed, so the order
// goes on: reflect only records learnings, and failing the order would drop
// the kept work of the stages before it.
func (l *Loop) skipFailedReflect(ctx context.Context, cook *cookHandle, reason string) error {
	if err := l.ensureCanonicalOrderFromOrders(cook.orderID); err != nil {
		return err
	}
	if err := l.emitEventChecked(ingest.EventStageSkipped, map[string]any{
		"order_id":         cook.orderID,
		"stage_index":      cook.stageIndex,
		"hold_for_publish": l.orderGated(),
	}); err != nil {
		return err
	}
	l.logger.Warn("reflect stage failed; skipped so the order continues",
		"order", cook.orderID, "stage_index", cook.stageIndex, "reason", reason)
	l.forwardToScheduler(cook, "reflect_skipped", reason, nil)
	l.cleanupCookWorktree(cook)
	msg := "reflect skipped: " + reason
	return l.advanceAndPersist(ctx, cook, &msg)
}
