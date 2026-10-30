package snapshot

import "github.com/poteto/noodle/internal/state"

// setOutcomeFields fills the stage's user-facing state and outcomes from its
// status and extra.
func setOutcomeFields(s *Stage) {
	recorded := state.Outcomes(s.Extra)
	s.Outcomes = nil
	for _, o := range recorded {
		s.Outcomes = append(s.Outcomes, StageOutcome{State: o.State, Action: o.Action, Feedback: o.Feedback})
	}
	s.State = state.UserState(s.Status, recorded)
}
