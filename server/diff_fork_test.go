package server

import (
	"reflect"
	"testing"

	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/worktree"
)

func TestDiffWorktreeFuncPicksLocalWhenConfigured(t *testing.T) {
	for _, tc := range []struct {
		local bool
		want  any
	}{
		{false, worktree.DiffWorktree},
		{true, worktree.DiffWorktreeLocal},
	} {
		got := diffWorktreeFunc(config.WorktreeConfig{DiffAgainstLocalOnly: tc.local})
		if reflect.ValueOf(got).Pointer() != reflect.ValueOf(tc.want).Pointer() {
			t.Errorf("diff_against_local_only=%v picked the wrong diff function", tc.local)
		}
	}
}
