package loop

import (
	"errors"
	"slices"
	"strings"
)

// ReviewActionBlock names control actions refused for a pending review and why.
type ReviewActionBlock struct {
	Actions []string `json:"actions"`
	Reason  string   `json:"reason"`
}

// reviewActionBlock refuses approve and merge for a stage held for external
// review; the held stage is completed with stage-complete instead. In a gated
// order any other parked stage refuses merge, since the order publishes once
// at the end.
func (l *Loop) reviewActionBlock(orderID string) (ReviewActionBlock, bool) {
	pending, ok := l.cooks.pendingReview[orderID]
	if !ok {
		return ReviewActionBlock{}, false
	}
	if strings.HasPrefix(pending.reason, externalReviewReasonPrefix) {
		held := strings.TrimSpace(pending.stage.TaskKey)
		return ReviewActionBlock{
			Actions: []string{"approve", "merge"},
			Reason:  "order " + orderID + " is held at stage " + held + " for external review; complete it with " + stageCompleteCLI(orderID, held) + " (action stage-complete)",
		}, true
	}
	if l.orderGated() {
		return ReviewActionBlock{Actions: []string{"merge"}, Reason: gatedMergeBlockReason}, true
	}
	return ReviewActionBlock{}, false
}

// refuseBlockedReviewAction returns an error when action is blocked for the
// order's pending review.
func (l *Loop) refuseBlockedReviewAction(orderID, action string) error {
	if block, ok := l.reviewActionBlock(orderID); ok && slices.Contains(block.Actions, action) {
		return errors.New(action + " refused: " + block.Reason)
	}
	return nil
}

func (l *Loop) reviewActionBlocks() map[string]ReviewActionBlock {
	var out map[string]ReviewActionBlock
	for orderID := range l.cooks.pendingReview {
		if block, ok := l.reviewActionBlock(orderID); ok {
			if out == nil {
				out = map[string]ReviewActionBlock{}
			}
			out[orderID] = block
		}
	}
	return out
}

const gatedMergeBlockReason = "this order publishes at the end; approve the stage, then approve-and-publish"
