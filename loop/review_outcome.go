package loop

import (
	"strings"

	"github.com/poteto/noodle/event"
)

// LoopEventReviewDecided records a human or external review decision on a
// parked stage, shown in the feed.
const LoopEventReviewDecided event.LoopEventType = "review.decided"

type ReviewDecidedPayload struct {
	OrderID    string `json:"order_id"`
	StageIndex int    `json:"stage_index"`
	TaskKey    string `json:"task_key"`
	Verdict    string `json:"verdict"`
	Outcome    string `json:"outcome,omitempty"`
	Summary    string `json:"summary,omitempty"`
	ReviewFile string `json:"review_file,omitempty"`
}

// reviewSummary returns the first three non-empty lines of a review.
func reviewSummary(review string) string {
	var lines []string
	for _, line := range strings.Split(review, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			lines = append(lines, line)
			if len(lines) == 3 {
				break
			}
		}
	}
	return strings.Join(lines, "\n")
}

func (l *Loop) emitReviewDecision(cook *cookHandle, verdict, outcome, review, reviewFile string) {
	_ = l.events.Emit(LoopEventReviewDecided, ReviewDecidedPayload{
		OrderID:    cook.orderID,
		StageIndex: cook.stageIndex,
		TaskKey:    cook.stage.TaskKey,
		Verdict:    verdict,
		Outcome:    outcome,
		Summary:    reviewSummary(review),
		ReviewFile: reviewFile,
	})
}
