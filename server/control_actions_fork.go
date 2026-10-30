package server

import "slices"

// forkControlActions are control actions added by this fork, accepted by
// POST /api/control and the websocket next to the upstream list.
var forkControlActions = []string{"approve", "stage-complete", "approve-and-publish", "resolve"}

func isForkControlAction(action string) bool {
	return slices.Contains(forkControlActions, action)
}
