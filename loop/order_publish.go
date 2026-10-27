package loop

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/poteto/noodle/adapter"
	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/event"
	"github.com/poteto/noodle/internal/ingest"
	"github.com/poteto/noodle/internal/taskreg"
	"github.com/poteto/noodle/skill"
	"github.com/poteto/noodle/worktree"
)

// LoopEventOrderPublishFailed reports an approve-and-publish that did not
// deliver; the order stays awaiting-publish with its kept work.
const LoopEventOrderPublishFailed event.LoopEventType = "order.publish_failed"

type OrderPublishFailedPayload struct {
	OrderID string `json:"order_id"`
	Error   string `json:"error"`
}

// publishExtraKey marks the stage appended to run a publish_action skill.
const publishExtraKey = "publish"

func isPublishStage(stage Stage) bool {
	_, ok := stage.Extra[publishExtraKey]
	return ok
}

func (l *Loop) publishErrorsPath() string {
	return filepath.Join(l.runtimeDir, "publish-errors.json")
}

// ReadPublishErrors returns the last failed publish per order, keyed by order
// ID, from publish-errors.json:
//
//	{"42": "merge 42-1-review: rebase conflict"}
//
// An order has an entry only between a failed publish and its next attempt.
func ReadPublishErrors(runtimeDir string) map[string]string {
	errs := map[string]string{}
	data, err := os.ReadFile(filepath.Join(runtimeDir, "publish-errors.json"))
	if err != nil {
		return errs
	}
	_ = json.Unmarshal(data, &errs)
	return errs
}

func (l *Loop) setPublishError(orderID, msg string) {
	errs := ReadPublishErrors(l.runtimeDir)
	if msg == "" {
		if _, ok := errs[orderID]; !ok {
			return
		}
		delete(errs, orderID)
	} else {
		errs[orderID] = msg
	}
	if err := writeJSONMap(l.publishErrorsPath(), errs); err != nil {
		l.logger.Warn("save publish error failed", "order", orderID, "error", err)
	}
}

func writeJSONMap[V any](path string, m map[string]V) error {
	if len(m) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, append(data, '\n'), 0o644)
}

// publishFailed records a failed publish for the snapshot and returns it as
// the control error.
func (l *Loop) publishFailed(orderID string, err error) error {
	l.setPublishError(orderID, err.Error())
	_ = l.events.Emit(LoopEventOrderPublishFailed, OrderPublishFailedPayload{OrderID: orderID, Error: err.Error()})
	l.logger.Warn("publish failed", "order", orderID, "error", err)
	return err
}

// controlApproveAndPublish delivers an awaiting-publish order's kept work with
// the configured [review] publish_action and completes the order.
func (l *Loop) controlApproveAndPublish(orderID string) error {
	orderID = strings.TrimSpace(orderID)
	if orderID == "" {
		return fmt.Errorf("approve-and-publish: order ID empty")
	}
	order, err := l.publishableOrder(orderID)
	if err != nil {
		return err
	}
	action := strings.TrimSpace(l.config.Review.PublishAction)
	isSkill := action != "" && action != config.PublishActionDraftPR
	if isSkill {
		if _, err := (skill.Resolver{SearchPaths: l.config.Skills.Paths}).Resolve(action); err != nil {
			return fmt.Errorf("publish_action skill %s is not a registered skill", action)
		}
	}
	l.setPublishError(orderID, "")

	branch, hasWork := l.orderWorkBranch(orderID)
	switch {
	case !hasWork || !l.orderHasCommits(orderID):
		return l.finishPublishedOrder(context.Background(), orderID, nothingToPublish)
	case isSkill:
		return l.startPublishStage(order, action, branch)
	case action == config.PublishActionDraftPR:
		if l.deps.DraftPR == nil {
			return l.publishFailed(orderID, fmt.Errorf("publish_action draft-pr needs [delivery] push_remote, pr_repo and base"))
		}
		title := strings.TrimSpace(order.Title)
		if title == "" {
			title = orderID
		}
		res, err := l.deps.DraftPR.Publish(DraftPRRequest{
			OrderID: orderID,
			Branch:  branch,
			Title:   title,
			Body:    strings.TrimSpace(order.Rationale),
		})
		if err != nil {
			return l.publishFailed(orderID, fmt.Errorf("draft-pr: %w", err))
		}
		links := ReadDraftPRs(l.runtimeDir)
		links[orderID] = DraftPRLink{Branch: res.Branch, URL: res.URL}
		if err := writeJSONMap(filepath.Join(l.runtimeDir, "draft-prs.json"), links); err != nil {
			l.logger.Warn("save draft PR record failed", "order", orderID, "error", err)
		}
		l.logger.Info("order published as draft PR", "order", orderID, "branch", res.Branch, "url", res.URL)
		return l.finishPublishedOrder(context.Background(), orderID, "draft PR "+res.URL)
	default:
		// Kept branches are stage worktree names, and each stage starts from
		// the previous one, so the newest holds all of the order's work.
		if err := l.deps.Worktree.Merge(branch); err != nil {
			return l.publishFailed(orderID, fmt.Errorf("merge %s: %w", branch, err))
		}
		_ = l.events.Emit(LoopEventWorktreeMerged, WorktreeMergedPayload{OrderID: orderID, WorktreeName: branch})
		return l.finishPublishedOrder(context.Background(), orderID, "merged "+branch)
	}
}

const nothingToPublish = "nothing to publish; order completed"

// orderHasCommits reports whether any kept branch of the order has commits
// that the integration branch lacks (git cherry). A failed check counts as
// having commits, so the publish action decides.
func (l *Loop) orderHasCommits(orderID string) bool {
	for _, branch := range l.loadOrderWork()[orderID].Branches {
		has, err := l.deps.Worktree.HasUnmergedCommits(branch)
		if err != nil {
			l.logger.Warn("publish commit check failed", "order", orderID, "branch", branch, "error", err)
			return true
		}
		if has {
			return true
		}
	}
	return false
}

// publishableOrder returns the order when it is awaiting publish, else an
// error naming what still blocks it.
func (l *Loop) publishableOrder(orderID string) (Order, error) {
	orders, err := l.currentOrders()
	if err != nil {
		return Order{}, err
	}
	for _, order := range orders.Orders {
		if order.ID != orderID {
			continue
		}
		if reason := l.publishBlockReason(order); reason != "" {
			return Order{}, fmt.Errorf("%s", reason)
		}
		return order, nil
	}
	return Order{}, fmt.Errorf("approve-and-publish refused: order %s not found (unknown or already completed)", orderID)
}

// publishBlockReason returns why approve-and-publish refuses order, or "" when
// the order is awaiting publish. The snapshot shows the same text.
func (l *Loop) publishBlockReason(order Order) string {
	if order.Status == OrderStatusAwaitingPublish {
		return ""
	}
	orderID := order.ID
	var notDone []string
	for i, stage := range order.Stages {
		if isPublishStage(stage) && stage.Status != StageStatusCompleted && stage.Status != StageStatusFailed {
			return fmt.Sprintf("approve-and-publish refused: publish already running for order %s", orderID)
		}
		if stage.Status != StageStatusCompleted && stage.Status != StageStatusSkipped {
			notDone = append(notDone, fmt.Sprintf("%d:%s:%s", i, nonEmpty(stage.TaskKey, stage.Skill), stage.Status))
		}
	}
	msg := fmt.Sprintf("approve-and-publish refused: order %s is not awaiting publish (status %s)", orderID, order.Status)
	if len(notDone) > 0 {
		msg += "; stages not completed: " + strings.Join(notDone, ", ")
	}
	if pending, ok := l.cooks.pendingReview[orderID]; ok && l.config.Review.IsExternal(pending.stage.TaskKey, pending.stage.Skill) {
		msg += fmt.Sprintf("; held for external review at stage %s, finish it with %s", pending.stage.TaskKey, stageCompleteCLI(orderID, pending.stage.TaskKey))
	}
	return msg
}

// publishBlocks maps each refused order ID to its publishBlockReason, for
// gated configs only.
func (l *Loop) publishBlocks() map[string]string {
	if !l.orderGated() {
		return nil
	}
	orders, err := l.currentOrders()
	if err != nil {
		return nil
	}
	var out map[string]string
	for _, order := range orders.Orders {
		if reason := l.publishBlockReason(order); reason != "" {
			if out == nil {
				out = map[string]string{}
			}
			out[order.ID] = reason
		}
	}
	return out
}

// startPublishStage appends a stage running the publish_action skill and
// reactivates the order so dispatch starts it from the newest kept branch.
// The stage's task key is the skill name only when the skill is a task type;
// otherwise it is empty and its prompt keeps it through order normalization.
func (l *Loop) startPublishStage(order Order, skillName, branch string) error {
	taskKey := ""
	if _, ok := l.registry.ResolveStage(taskreg.StageInput{TaskKey: skillName}); ok {
		taskKey = skillName
	}
	prompt := fmt.Sprintf("Publish order %s: %s\n\nRationale: %s\n\nAll of the order's approved work is on branch %s, which this worktree starts from. Deliver that work with the %s skill.",
		order.ID, strings.TrimSpace(order.Title), strings.TrimSpace(order.Rationale), branch, skillName)
	stage := Stage{
		TaskKey: taskKey,
		Skill:   skillName,
		Prompt:  prompt,
		Status:  StageStatusPending,
		Extra:   map[string]json.RawMessage{publishExtraKey: json.RawMessage("true")},
	}
	if err := l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		for i := range orders.Orders {
			if orders.Orders[i].ID != order.ID {
				continue
			}
			orders.Orders[i].Stages = append(orders.Orders[i].Stages, stage)
			orders.Orders[i].Status = OrderStatusActive
			return true, nil
		}
		return false, fmt.Errorf("order %q not found", order.ID)
	}); err != nil {
		return err
	}
	l.cmds.ackMessage = fmt.Sprintf("publish stage %s queued from %s", skillName, branch)
	return nil
}

// completePublishStage finishes an order whose publish stage exited cleanly.
// The stage's own worktree is discarded, never merged.
func (l *Loop) completePublishStage(ctx context.Context, cook *cookHandle) error {
	if name := strings.TrimSpace(cook.worktreeName); name != "" {
		_ = l.deps.Worktree.Cleanup(name, worktree.CleanupOpts{Force: true})
	}
	if err := l.emitEventChecked(ingest.EventStageCompleted, l.mergeLifecyclePayload(cook, false)); err != nil {
		return err
	}
	l.releaseOrderWork(cook.orderID)
	l.setPublishError(cook.orderID, "")
	return l.advanceAndPersist(ctx, cook)
}

// failPublishStage drops a failed publish stage and returns the order to
// awaiting-publish so approve-and-publish can retry with a fresh stage.
func (l *Loop) failPublishStage(cook *cookHandle, reason string) error {
	if err := l.mutateOrdersState(func(orders *OrdersFile) (bool, error) {
		for i := range orders.Orders {
			if orders.Orders[i].ID != cook.orderID {
				continue
			}
			kept := orders.Orders[i].Stages[:0]
			for _, stage := range orders.Orders[i].Stages {
				if !isPublishStage(stage) {
					kept = append(kept, stage)
				}
			}
			orders.Orders[i].Stages = kept
			orders.Orders[i].Status = OrderStatusAwaitingPublish
			return true, nil
		}
		return false, nil
	}); err != nil {
		return err
	}
	if name := strings.TrimSpace(cook.worktreeName); name != "" {
		_ = l.deps.Worktree.Cleanup(name, worktree.CleanupOpts{Force: true})
	}
	_ = l.publishFailed(cook.orderID, fmt.Errorf("publish stage %s %s", cook.stage.Skill, reason))
	return l.syncPendingReviewProjection()
}

// finishPublishedOrder completes an order after its work was delivered.
func (l *Loop) finishPublishedOrder(ctx context.Context, orderID, ackMessage string) error {
	if err := l.ensureCanonicalOrderFromOrders(orderID); err != nil {
		return err
	}
	if err := l.emitEventChecked(ingest.EventOrderCompleted, map[string]any{"order_id": orderID}); err != nil {
		return err
	}
	if err := l.mirrorLegacyOrderFromCanonical(orderID); err != nil {
		return err
	}
	if err := l.syncPendingReviewProjection(); err != nil {
		return err
	}
	l.releaseOrderWork(orderID)
	l.setPublishError(orderID, "")
	_ = l.events.Emit(LoopEventOrderCompleted, OrderCompletedPayload{OrderID: orderID, Outcome: ackMessage})
	if _, err := l.deps.Adapter.Run(ctx, "backlog", "done", adapter.RunOptions{Args: []string{orderID}}); err != nil {
		if !isMissingAdapter(err) {
			return err
		}
	}
	l.cmds.ackMessage = ackMessage
	return nil
}
