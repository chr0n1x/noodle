package server

import (
	"github.com/poteto/noodle/config"
	"github.com/poteto/noodle/worktree"
)

// diffWorktree builds a review diff, against the local default branch when
// [worktree] diff_against_local_only is set.
func (s *Server) diffWorktree(path string) (worktree.DiffResult, error) {
	return diffWorktreeFunc(s.config.Worktree)(path)
}

func diffWorktreeFunc(cfg config.WorktreeConfig) func(string) (worktree.DiffResult, error) {
	if cfg.DiffAgainstLocalOnly {
		return worktree.DiffWorktreeLocal
	}
	return worktree.DiffWorktree
}
