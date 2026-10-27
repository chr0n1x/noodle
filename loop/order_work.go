package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/poteto/noodle/event"
	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/worktree"
)

// LoopEventOrderWorkKept reports an order that finished while its work lives
// only in kept local branches.
const LoopEventOrderWorkKept event.LoopEventType = "order.work_kept"

// orderWorkRecord lists the stage branches kept for an order after an
// approve-without-publish, oldest first; the last one holds all the work.
//
//	{"42": {"branches": ["42-0-execute", "42-1-quality"]}}
//
// An order has an entry only between its first approve and a publish or reject.
type orderWorkRecord struct {
	Branches []string `json:"branches"`
}

type OrderWorkKeptPayload struct {
	OrderID  string   `json:"order_id"`
	Branches []string `json:"branches"`
}

func (l *Loop) orderWorkPath() string {
	return filepath.Join(l.runtimeDir, "order-work.json")
}

func (l *Loop) loadOrderWork() map[string]orderWorkRecord {
	records := map[string]orderWorkRecord{}
	data, err := os.ReadFile(l.orderWorkPath())
	if err != nil {
		if !os.IsNotExist(err) {
			l.logger.Warn("read order work failed", "error", err)
		}
		return records
	}
	if err := json.Unmarshal(data, &records); err != nil {
		l.logger.Warn("parse order work failed", "error", err)
	}
	return records
}

func (l *Loop) saveOrderWork(records map[string]orderWorkRecord) error {
	if len(records) == 0 {
		if err := os.Remove(l.orderWorkPath()); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove order work: %w", err)
		}
		return nil
	}
	data, err := json.MarshalIndent(records, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(l.runtimeDir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(l.orderWorkPath(), append(data, '\n'), 0o644)
}

func (l *Loop) keepOrderWork(orderID, branch string) error {
	records := l.loadOrderWork()
	rec := records[orderID]
	if !slices.Contains(rec.Branches, branch) {
		rec.Branches = append(rec.Branches, branch)
	}
	records[orderID] = rec
	if err := l.saveOrderWork(records); err != nil {
		return fmt.Errorf("keep work for order %q: %w", orderID, err)
	}
	return nil
}

// orderWorkBranch returns the newest kept branch for an order, which later
// stage worktrees start from.
func (l *Loop) orderWorkBranch(orderID string) (string, bool) {
	rec, ok := l.loadOrderWork()[orderID]
	if !ok || len(rec.Branches) == 0 {
		return "", false
	}
	return rec.Branches[len(rec.Branches)-1], true
}

// releaseOrderWork deletes an order's kept branches and worktrees, after its
// work was published or the user rejected it.
func (l *Loop) releaseOrderWork(orderID string) {
	records := l.loadOrderWork()
	rec, ok := records[orderID]
	if !ok {
		return
	}
	for _, branch := range rec.Branches {
		if err := l.deps.Worktree.Cleanup(branch, worktree.CleanupOpts{Force: true}); err != nil {
			l.logger.Warn("clean kept work failed", "order", orderID, "branch", branch, "error", err)
		}
	}
	delete(records, orderID)
	if err := l.saveOrderWork(records); err != nil {
		l.logger.Warn("save order work failed", "order", orderID, "error", err)
	}
}

func (l *Loop) noteKeptWork(orderID string) {
	rec, ok := l.loadOrderWork()[orderID]
	if !ok || len(rec.Branches) == 0 {
		return
	}
	_ = l.events.Emit(LoopEventOrderWorkKept, OrderWorkKeptPayload{OrderID: orderID, Branches: rec.Branches})
}

// controlApprove completes a parked stage and advances the order without
// merging or opening a PR. The stage branch is kept so later stages build on it.
func (l *Loop) controlApprove(orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return fmt.Errorf("approve: order ID empty")
	}
	pending, ok := l.cooks.pendingReview[orderID]
	if !ok {
		return fmt.Errorf("no pending review for %q", orderID)
	}
	if err := l.refuseBlockedReviewAction(orderID, "approve"); err != nil {
		return err
	}
	if err := l.ensureCanonicalOrderFromOrders(orderID); err != nil {
		return err
	}
	cook := &cookHandle{
		cookIdentity: pending.cookIdentity,
		orderStatus:  OrderStatusActive,
		worktreeName: pending.worktreeName,
		worktreePath: pending.worktreePath,
		session:      &adoptedSession{id: pending.sessionID, status: "completed"},
	}
	reason := "approved, not published"
	if branch := strings.TrimSpace(pending.worktreeName); branch != "" {
		if err := l.keepOrderWork(orderID, branch); err != nil {
			return err
		}
		reason += "; unpublished work kept at " + branch
	}
	if err := l.emitEventChecked(ingest.EventStageReviewApproved, map[string]any{
		"order_id":    cook.orderID,
		"stage_index": cook.stageIndex,
		"reason":      reason,
	}); err != nil {
		return err
	}
	if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, false)); err != nil {
		return err
	}
	if err := l.skipStagesWithoutChanges(cook, &reason); err != nil {
		return err
	}
	if err := l.advanceAndPersist(context.Background(), cook, &reason); err != nil {
		return err
	}
	l.emitReviewDecision(cook, "approved", strings.Replace(strings.TrimPrefix(reason, "approved, "), "unpublished work", "work", 1), "", "")
	l.cmds.ackMessage = reason
	return l.syncPendingReviewProjection()
}

// keepOrDiscardAcceptedWork handles an accepted stage's worktree. When the order
// has kept work, the stage branch holds it plus any reviewer commits, so it is
// kept as the newest branch; otherwise the worktree is removed as before.
// Returns the kept branch, or "".
func (l *Loop) keepOrDiscardAcceptedWork(orderID, worktreeName string) string {
	name := strings.TrimSpace(worktreeName)
	if name == "" {
		return ""
	}
	if _, ok := l.orderWorkBranch(orderID); ok {
		if err := l.keepOrderWork(orderID, name); err == nil {
			return name
		}
		l.logger.Warn("keep accepted work failed; leaving worktree in place", "order", orderID, "worktree", name)
		return name
	}
	_ = l.deps.Worktree.Cleanup(name, worktree.CleanupOpts{Force: true})
	return ""
}
