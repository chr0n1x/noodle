package dispatcher

import (
	"slices"
	"testing"
)

func TestSessionEnvPointsNoodleAtProjectDir(t *testing.T) {
	d := NewProcessDispatcher(ProcessDispatcherConfig{ProjectDir: "/repo"})
	env := d.sessionEnv(DispatchRequest{WorktreePath: "/repo/.worktrees/2-0-execute"}, "2-0-execute-abc")

	for _, want := range []string{
		"NOODLE_SESSION_ID=2-0-execute-abc",
		"NOODLE_PROJECT_DIR=/repo",
		"NOODLE_WORKTREE=/repo/.worktrees/2-0-execute",
	} {
		if !slices.Contains(env, want) {
			t.Errorf("session env missing %q", want)
		}
	}
}
