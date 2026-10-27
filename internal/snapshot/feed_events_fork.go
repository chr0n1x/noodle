package snapshot

import (
	"encoding/json"
	"fmt"
	"strings"
)

// forkFeedEvent renders loop events added by this fork. ok is false for
// unknown types, which the feed skips.
func forkFeedEvent(eventType string, payload json.RawMessage) (label, body, category, taskType string, ok bool) {
	switch eventType {
	case "review.decided":
		var p struct {
			OrderID    string `json:"order_id"`
			TaskKey    string `json:"task_key"`
			Verdict    string `json:"verdict"`
			Outcome    string `json:"outcome"`
			Summary    string `json:"summary"`
			ReviewFile string `json:"review_file"`
		}
		_ = json.Unmarshal(payload, &p)
		label = "Review " + p.Verdict
		parts := []string{fmt.Sprintf("Order %s %s: %s", p.OrderID, p.TaskKey, p.Verdict)}
		if p.Outcome != "" {
			parts[0] += ", " + p.Outcome
		}
		if s := strings.TrimSpace(p.Summary); s != "" {
			parts = append(parts, s)
		}
		if p.ReviewFile != "" {
			parts = append(parts, "Review: "+p.ReviewFile)
		}
		return label, strings.Join(parts, "\n"), "review_decided", p.TaskKey, true
	case "order.work_kept":
		var p struct {
			OrderID  string   `json:"order_id"`
			Branches []string `json:"branches"`
		}
		_ = json.Unmarshal(payload, &p)
		body = fmt.Sprintf("Order %s finished with unpublished work kept at %s. Merge or open a PR from the newest branch, or delete the branches.",
			p.OrderID, strings.Join(p.Branches, ", "))
		return "Work kept", body, "order_work_kept", "", true
	case "order.awaiting_publish":
		var p struct {
			OrderID  string   `json:"order_id"`
			Branches []string `json:"branches"`
		}
		_ = json.Unmarshal(payload, &p)
		body = fmt.Sprintf("Order %s finished all stages and is waiting to be published.", p.OrderID)
		if len(p.Branches) > 0 {
			body += " Work is on " + p.Branches[len(p.Branches)-1] + "."
		}
		return "Awaiting publish", body, "order_awaiting_publish", "", true
	}
	return "", "", "", "", false
}
