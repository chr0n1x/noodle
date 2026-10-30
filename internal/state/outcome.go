package state

import "encoding/json"

// OutcomesExtraKey is the stage Extra key holding the stage's outcomes, one
// per finished attempt or human decision, oldest first:
//
//	[
//	  {"state": "reject", "action": "retry-previous", "feedback": "the test asserts nothing"},
//	  {"state": "accept", "action": "promote"}
//	]
//
// A stage that never finished, and every stage the loop ran under its
// mode-driven defaults, has no entry.
const OutcomesExtraKey = "outcomes"

// StageOutcome is one verdict on a stage: its state (accept or reject), the
// action taken (promote, exit, park, retry, retry-previous) and feedback.
type StageOutcome struct {
	State    string `json:"state"`
	Action   string `json:"action"`
	Feedback string `json:"feedback,omitempty"`
}

// Outcomes decodes the stage's outcomes from extra.
func Outcomes(extra map[string]json.RawMessage) []StageOutcome {
	var out []StageOutcome
	_ = json.Unmarshal(extra[OutcomesExtraKey], &out)
	return out
}

// RejectedThrough returns the latest outcome of the first stage whose
// latest outcome rejected it and still let the order go on (promote or exit).
// A rejection that led to a retry or a park was superseded by what followed.
func RejectedThrough(order OrderNode) (StageOutcome, bool) {
	for _, s := range order.Stages {
		outs := Outcomes(s.Extra)
		if len(outs) == 0 {
			continue
		}
		last := outs[len(outs)-1]
		if last.State == "reject" && (last.Action == "promote" || last.Action == "exit") {
			return last, true
		}
	}
	return StageOutcome{}, false
}

// UserState maps an internal stage status, in either the canonical or the
// orders.json spelling, to the user-facing state: pending, in-progress,
// accept, reject or skipped. A finished stage with outcomes takes the state
// of its latest one.
func UserState(status string, outcomes []StageOutcome) string {
	switch status {
	case "pending", "":
		return "pending"
	case "completed", "failed":
		if len(outcomes) > 0 {
			return outcomes[len(outcomes)-1].State
		}
		if status == "completed" {
			return "accept"
		}
		return "reject"
	case "skipped", "cancelled":
		return "skipped"
	default:
		return "in-progress"
	}
}
