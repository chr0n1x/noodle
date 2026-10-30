package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"strings"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/state"
)

func stageOutcomes(s Stage) []state.StageOutcome {
	return state.Outcomes(s.Extra)
}

// configuredAction returns the [stages.<task>] action for a finished stage,
// and false when the loop's mode-driven behavior applies.
func (l *Loop) configuredAction(stage Stage, st config.StageState) (config.StageAction, bool) {
	if isScheduleStage(stage) || isPublishStage(stage) || isHookStage(stage) {
		return "", false
	}
	return l.config.StageAction(stage.TaskKey, st)
}

// missingVerdictFeedback is the feedback a stage re-runs with when it ended
// without the stage_message its configured reject action depends on.
const missingVerdictFeedback = "Your previous attempt ended without a verdict: it finished without emitting a stage_message. Run the stage again and end it with exactly one stage_message, blocking to reject or non-blocking to accept. Do not end your turn while a command you started is still running."

// applyConfiguredCompletion applies the configured action to a cook that
// finished cleanly. Its state is reject when the cook left a blocking stage
// message, accept otherwise; the message is the feedback. A stage whose task
// has a reject action must leave a message: without one it runs again with
// missingVerdictFeedback, and parks once max_rework is used up. Returns false
// when no action is configured for that state.
func (l *Loop) applyConfiguredCompletion(ctx context.Context, cook *cookHandle) (bool, error) {
	st, feedback := config.StageStateAccept, ""
	m := l.readStageMessage(cook.session.ID())
	if m == nil {
		if _, needsVerdict := l.configuredAction(cook.stage, config.StageStateReject); needsVerdict {
			return true, l.retryMissingVerdict(cook)
		}
	}
	if m != nil {
		feedback = m.Message
		if m.IsBlocking() {
			st = config.StageStateReject
		}
	}
	action, ok := l.configuredAction(cook.stage, st)
	if !ok {
		return false, nil
	}
	if m != nil {
		l.forwardToScheduler(cook, "stage_message", m.Message, nil)
	}
	return true, l.applyOutcome(ctx, cook, st, action, feedback)
}

// retryMissingVerdict re-runs a stage that ended without its verdict, and
// parks it for a human once max_rework is used up. No outcome is recorded:
// the stage gave no verdict.
func (l *Loop) retryMissingVerdict(cook *cookHandle) error {
	if err := l.ensureCanonicalOrderFromOrders(cook.orderID); err != nil {
		return err
	}
	l.logger.Warn("stage ended without a verdict", "order", cook.orderID, "stage", cook.stageIndex, "task", cook.stage.TaskKey)
	round, limit, err := l.rerunForChanges(cook, missingVerdictFeedback)
	if err != nil || round > 0 {
		return err
	}
	return l.parkPendingReview(cook, fmt.Sprintf("ended without a verdict (rework limit %d reached)", limit))
}

// applyOutcome records a verdict on the cook's stage and carries out action.
func (l *Loop) applyOutcome(ctx context.Context, cook *cookHandle, st config.StageState, action config.StageAction, feedback string) error {
	feedback = strings.TrimSpace(feedback)
	if err := l.ensureCanonicalOrderFromOrders(cook.orderID); err != nil {
		return err
	}
	if err := l.recordOutcome(cook.orderID, cook.stageIndex, state.StageOutcome{
		State: string(st), Action: string(action), Feedback: feedback,
	}); err != nil {
		return err
	}
	l.logger.Info("stage outcome", "order", cook.orderID, "stage", cook.stageIndex,
		"task", cook.stage.TaskKey, "state", st, "action", action)
	verdict := "accepted"
	if st == config.StageStateReject {
		verdict = "rejected"
	}
	msg := verdict + ", " + string(action)
	if feedback != "" {
		msg += ": " + feedback
	}

	switch action {
	case config.StageActionPark:
		return l.parkPendingReview(cook, msg)
	case config.StageActionPromote:
		return l.promoteStage(ctx, cook, msg, true)
	case config.StageActionExit:
		return l.exitOrder(ctx, cook, msg)
	case config.StageActionRetry:
		round, limit, err := l.rerunForChanges(cook, nonEmpty(feedback, verdict))
		if err != nil || round > 0 {
			return err
		}
		return l.failRejected(cook, fmt.Sprintf("%s (rework limit %d reached)", msg, limit))
	case config.StageActionRetryPrevious:
		round, limit, err := l.reworkFrom(cook, nonEmpty(feedback, verdict), isReworkTarget)
		if err != nil || round > 0 {
			return err
		}
		if reworkTargetMissing(l, cook) {
			return l.failRejected(cook, msg+" (no earlier stage to retry)")
		}
		return l.failRejected(cook, fmt.Sprintf("%s (rework limit %d reached)", msg, limit))
	}
	return fmt.Errorf("unknown stage action %q", action)
}

// isReworkTarget picks the stage retry-previous sends work back to: the
// nearest earlier work stage, past hooks and loop-inserted quality checks.
func isReworkTarget(s Stage) bool {
	return !isHookStage(s) && !isAutoQuality(s)
}

func reworkTargetMissing(l *Loop, cook *cookHandle) bool {
	orders, err := l.currentOrders()
	if err != nil {
		return false
	}
	for _, o := range orders.Orders {
		if o.ID != cook.orderID {
			continue
		}
		for j := cook.stageIndex - 1; j >= 0 && j < len(o.Stages); j-- {
			if isReworkTarget(o.Stages[j]) {
				return false
			}
		}
		return true
	}
	return false
}

// recordOutcome appends o to the stage's outcomes in the orders state.
func (l *Loop) recordOutcome(orderID string, index int, o state.StageOutcome) error {
	return l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		for i := range orders.Orders {
			order := &orders.Orders[i]
			if order.ID != orderID || index >= len(order.Stages) {
				continue
			}
			stage := &order.Stages[index]
			raw, err := json.Marshal(append(stageOutcomes(*stage), o))
			if err != nil {
				return false, fmt.Errorf("encode stage outcomes: %w", err)
			}
			extra := maps.Clone(stage.Extra)
			if extra == nil {
				extra = map[string]json.RawMessage{}
			}
			extra[state.OutcomesExtraKey] = raw
			stage.Extra = extra
			return true, nil
		}
		return false, nil
	})
}

// promoteStage completes the stage, keeping its branch so later stages build
// on it, and starts the next stage. Nothing is merged or published here.
func (l *Loop) promoteStage(ctx context.Context, cook *cookHandle, msg string, insertQuality bool) error {
	if err := l.ensureCanonicalOrderFromOrders(cook.orderID); err != nil {
		return err
	}
	if err := l.emitEventChecked(ingest.EventStageReviewApproved, map[string]any{
		"order_id":    cook.orderID,
		"stage_index": cook.stageIndex,
		"reason":      msg,
	}); err != nil {
		return err
	}
	if insertQuality && l.autoQualityOn() && cook.stage.TaskKey == "execute" {
		if err := l.insertAutoQuality(cook); err != nil {
			return err
		}
	}
	if err := l.completeKeepingWork(ctx, cook, &msg); err != nil {
		return err
	}
	return l.syncPendingReviewProjection()
}

// exitOrder skips the order's remaining work stages, keeping [hooks] stages,
// and completes this one. The order then ends once its hooks have run: failed
// when this or an earlier stage was rejected and let through, otherwise
// completed or awaiting publish. The worktree is kept either way.
func (l *Loop) exitOrder(ctx context.Context, cook *cookHandle, msg string) error {
	order, ok := l.canonical.Orders[cook.orderID]
	if !ok {
		return fmt.Errorf("exit: order %q missing from canonical state", cook.orderID)
	}
	for i := cook.stageIndex + 1; i < len(order.Stages); i++ {
		s := order.Stages[i]
		if s.Status.IsTerminal() {
			continue
		}
		if _, hook := s.Extra[hookExtraKey]; hook {
			continue
		}
		if err := l.emitEventChecked(ingest.EventStageSkipped, map[string]any{
			"order_id":         cook.orderID,
			"stage_index":      i,
			"hold_for_publish": l.orderGated(),
		}); err != nil {
			return err
		}
	}
	return l.promoteStage(ctx, cook, msg, false)
}

// failRejected fails the order with reason, keeping the stage's worktree and
// branch so the work can be inspected or resumed.
func (l *Loop) failRejected(cook *cookHandle, reason string) error {
	if err := l.emitEventChecked(ingest.EventStageReviewChangesRequested, map[string]any{
		"order_id":    cook.orderID,
		"stage_index": cook.stageIndex,
		"reason":      reason,
	}); err != nil {
		return err
	}
	if err := l.mirrorLegacyOrderFromCanonical(cook.orderID); err != nil {
		return err
	}
	if err := l.syncPendingReviewProjection(); err != nil {
		return err
	}
	l.recordStageFailure(cook, reason, OrderFailureClassStageTerminal, nil)
	l.forwardToScheduler(cook, "stage_rejected", reason, nil)
	l.cmds.ackMessage = reason + "; order failed, worktree kept"
	return nil
}

// outcomeContext lists the verdicts recorded on stages up to and including
// index, for the prompt of the stage at index: a later stage sees why earlier
// ones were accepted or rejected, and a retried stage sees its own. Returns ""
// when none has a verdict.
func outcomeContext(stages []Stage, index int) string {
	var lines []string
	for i := 0; i <= index && i < len(stages); i++ {
		for _, o := range stageOutcomes(stages[i]) {
			line := fmt.Sprintf("- %s (stage %d): %s, %s", stages[i].TaskKey, i+1, o.State, o.Action)
			if f := strings.TrimSpace(o.Feedback); f != "" {
				line += ": " + f
			}
			lines = append(lines, line)
		}
	}
	if len(lines) == 0 {
		return ""
	}
	return "Verdicts so far in this order, oldest first:\n" + strings.Join(lines, "\n")
}

// controlResolve applies a human verdict to the stage an order is parked or
// held at: the stage must match by name, state is accept or reject, and do is
// the action. feedback is recorded and reaches later stages.
func (l *Loop) controlResolve(orderID, stage, st, do, feedback string) error {
	orderID = strings.TrimSpace(orderID)
	pending, ok := l.cooks.pendingReview[orderID]
	if !ok {
		return fmt.Errorf("no pending review for %q", orderID)
	}
	held := strings.TrimSpace(pending.stage.TaskKey)
	if strings.TrimSpace(stage) != held {
		return fmt.Errorf("resolve refused: order %s is parked at stage %s; use noodle order %s resolve %s <accept|reject> <action>", orderID, held, orderID, held)
	}
	verdict := config.StageState(strings.ToLower(strings.TrimSpace(st)))
	if verdict != config.StageStateAccept && verdict != config.StageStateReject {
		return fmt.Errorf("resolve refused: state %q is not accept or reject", st)
	}
	action, ok := config.ParseStageAction(do)
	if !ok {
		return fmt.Errorf("resolve refused: unknown action %q (promote, exit, park, retry, retry-previous)", do)
	}
	if (action == config.StageActionRetry || action == config.StageActionRetryPrevious) && l.atMaxConcurrency() {
		return fmt.Errorf("resolve for %q not applied: at max concurrency, try again when a cook finishes", orderID)
	}
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		orderStatus:  OrderStatusActive,
		worktreeName: pending.worktreeName,
		worktreePath: pending.worktreePath,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	if strings.HasPrefix(pending.reason, externalReviewReasonPrefix) && strings.TrimSpace(feedback) != "" {
		l.recordExternalReview(cook, feedback, string(verdict)+"ed")
	}
	if err := l.applyOutcome(context.Background(), cook, verdict, action, feedback); err != nil {
		return err
	}
	if l.cmds.ackMessage == "" {
		l.cmds.ackMessage = fmt.Sprintf("%s %s: %s, %s", orderID, held, verdict, action)
	}
	return nil
}
