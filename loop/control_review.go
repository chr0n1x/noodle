package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/stringx"
	"github.com/poteto/noodle/worktree"
)

func (l *Loop) controlMerge(orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return fmt.Errorf("merge: order ID empty")
	}
	pending, ok := l.cooks.pendingReview[orderID]
	if !ok {
		return fmt.Errorf("no pending review for %q", orderID)
	}
	if err := l.refuseBlockedReviewAction(orderID, "merge"); err != nil {
		return err
	}
	if err := l.ensureCanonicalOrderFromOrders(orderID); err != nil {
		return err
	}

	// Merge the worktree.
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		orderStatus:  OrderStatusActive,
		worktreeName: pending.worktreeName,
		worktreePath: pending.worktreePath,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	return l.approveAndMerge(cook, "approved by user")
}

// approveAndMerge approves a parked stage with reason, merges its worktree when
// it has changes, and advances the order.
func (l *Loop) approveAndMerge(cook *cookHandle, reason string) error {
	canMerge, err := l.worktreeHasChanges(cook)
	if err != nil {
		return fmt.Errorf("merge check: %w", err)
	}
	if err := l.emitEventChecked(ingest.EventStageReviewApproved, map[string]any{
		"order_id":    cook.orderID,
		"stage_index": cook.stageIndex,
		"reason":      reason,
	}); err != nil {
		return err
	}
	if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, canMerge)); err != nil {
		return err
	}

	if !canMerge {
		if err := l.advanceAndPersist(context.Background(), cook); err != nil {
			return err
		}
	} else {
		if l.mergeQueue == nil {
			if err := l.mergeCookWorktree(context.Background(), cook); err != nil {
				return err
			}
			if err := l.emitEventChecked(ingest.EventMergeCompleted, map[string]any{
				"order_id":    cook.orderID,
				"stage_index": cook.stageIndex,
			}); err != nil {
				return err
			}
			if err := l.advanceAndPersist(context.Background(), cook); err != nil {
				return err
			}
		} else {
			// Queued path: drainMergeResults emits merge_completed.
			l.mergeQueue.Enqueue(MergeRequest{Cook: cook})
			if err := l.drainMergeResults(context.Background()); err != nil {
				return err
			}
		}
	}
	return l.syncPendingReviewProjection()
}

func (l *Loop) controlReject(orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return fmt.Errorf("reject: order ID empty")
	}
	pending, ok := l.cooks.pendingReview[orderID]
	if !ok {
		return fmt.Errorf("no pending review for %q", orderID)
	}
	if err := l.ensureCanonicalOrderFromOrders(orderID); err != nil {
		return err
	}
	if strings.TrimSpace(pending.worktreeName) != "" {
		_ = l.deps.Worktree.Cleanup(pending.worktreeName, worktree.CleanupOpts{Force: true})
	}
	l.releaseOrderWork(orderID)
	if err := l.emitEventChecked(ingest.EventStageReviewRejected, map[string]any{
		"order_id":    orderID,
		"stage_index": pending.stageIndex,
		"reason":      "rejected by user",
	}); err != nil {
		return err
	}
	if err := l.mirrorLegacyOrderFromCanonical(orderID); err != nil {
		return err
	}
	if err := l.syncPendingReviewProjection(); err != nil {
		return err
	}
	mistake := newCookMistakeEnvelope(cookRejectReasonForTask(pending.stage.TaskKey), orderID, pending.stageIndex)
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	l.recordStageFailure(cook, "rejected by user", OrderFailureClassOrderTerminal, &mistake)
	l.classifyCookMistake(
		"control.review_reject",
		OrderFailureClassOrderTerminal,
		orderID,
		pending.stageIndex,
		"rejected by user",
		mistake.CookReason,
	)
	l.forwardToScheduler(cook, "review_rejected", "rejected by user", &mistake)
	return nil
}

func (l *Loop) controlRequestChanges(orderID, feedback string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return fmt.Errorf("request-changes: order ID empty")
	}
	pending, ok := l.cooks.pendingReview[orderID]
	if !ok {
		return fmt.Errorf("no pending review for %q", orderID)
	}
	if err := l.ensureCanonicalOrderFromOrders(orderID); err != nil {
		return err
	}
	if l.atMaxConcurrency() {
		return fmt.Errorf("request-changes for %q not applied: at max concurrency, try again when a cook finishes", orderID)
	}

	trimmedFeedback := strings.TrimSpace(feedback)
	limitReached := ""
	if l.orderGated() {
		if l.config.Review.IsExternal(pending.stage.TaskKey, pending.stage.Skill) {
			return l.controlStageComplete(orderID, pending.stage.TaskKey, feedback, true)
		}
		round, limit, err := l.rerunForChanges(pending, trimmedFeedback)
		if err != nil || round > 0 {
			return err
		}
		limitReached = fmt.Sprintf("rework limit %d reached", limit)
	}
	reason := "changes requested"
	if trimmedFeedback != "" {
		reason += ": " + trimmedFeedback
	}
	if limitReached != "" {
		reason += " (" + limitReached + ")"
		l.cmds.ackMessage = limitReached + "; order failed, worktree kept"
	}
	mistake := newCookMistakeEnvelope(CookMistakeReasonRequestChanges, orderID, pending.stageIndex)
	if err := l.emitEventChecked(ingest.EventStageReviewChangesRequested, map[string]any{
		"order_id":    orderID,
		"stage_index": pending.stageIndex,
		"reason":      reason,
	}); err != nil {
		return err
	}
	if err := l.mirrorLegacyOrderFromCanonical(orderID); err != nil {
		return err
	}
	if err := l.syncPendingReviewProjection(); err != nil {
		return err
	}
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		worktreeName: pending.worktreeName,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	l.recordStageFailure(cook, reason, OrderFailureClassStageTerminal, &mistake)
	l.classifyCookMistake(
		"control.request_changes",
		OrderFailureClassStageTerminal,
		orderID,
		pending.stageIndex,
		reason,
		mistake.CookReason,
	)
	l.forwardToScheduler(cook, "request_changes", reason, &mistake)

	// The worktree and its branch stay so the uncommitted work survives the failure.
	return nil
}

// rerunForChanges sends a parked stage of a gated order back to pending at the
// same index, so it re-dispatches into the same worktree with the feedback
// appended to its prompt. Returns the round started, or 0 when the
// [review] max_rework limit is reached.
func (l *Loop) rerunForChanges(pending *pendingReviewCook, feedback string) (round, limit int, err error) {
	limit = l.config.Review.MaxReworkRounds()
	err = l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		for i := range orders.Orders {
			order := &orders.Orders[i]
			if order.ID != pending.orderID || pending.stageIndex >= len(order.Stages) {
				continue
			}
			if reworkRounds(*order) >= limit {
				return false, nil
			}
			round = reworkRounds(*order) + 1
			stage := &order.Stages[pending.stageIndex]
			line := fmt.Sprintf("Changes requested (round %d of %d)", round, limit)
			if feedback != "" {
				line += ": " + feedback
			} else {
				line += ". Review your previous work and improve it."
			}
			line += "\n\nContinue in this worktree; your previous work is still here."
			stage.Prompt = stringx.JoinNonEmpty("\n\n", stage.Prompt, line)
			extra := maps.Clone(stage.Extra)
			if extra == nil {
				extra = map[string]json.RawMessage{}
			}
			extra[changesRequestedExtraKey] = json.RawMessage(fmt.Sprint(changesRequested(*stage) + 1))
			stage.Extra = extra
			stage.Status = StageStatusPending
			order.Status = OrderStatusActive
			return true, nil
		}
		return false, nil
	})
	if err != nil || round == 0 {
		return 0, limit, err
	}

	outcome := fmt.Sprintf("stage re-run round %d of %d queued", round, limit)
	reason := "changes requested; " + outcome
	if feedback != "" {
		reason += ": " + feedback
	}
	if err := l.emitEventChecked(ingest.EventStageReviewChangesRequested, map[string]any{
		"order_id":    pending.orderID,
		"stage_index": pending.stageIndex,
		"reason":      reason,
		"rerun":       true,
	}); err != nil {
		return 0, limit, err
	}
	if err := l.mirrorLegacyOrderFromCanonical(pending.orderID); err != nil {
		return 0, limit, err
	}
	if err := l.syncPendingReviewProjection(); err != nil {
		return 0, limit, err
	}
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		worktreeName: pending.worktreeName,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	l.emitReviewDecision(cook, "changes_requested", outcome, feedback, "")
	l.cmds.ackMessage = "changes requested; " + outcome
	l.forwardToScheduler(cook, "request_changes_rerun", reason, nil)
	return round, limit, nil
}
