package worktree

import (
	"fmt"
	"os"
	"path/filepath"
)

// DiffWorktreeLocal is DiffWorktree with the base taken from diffBaseLocal,
// which never forks from origin/<default>.
func DiffWorktreeLocal(worktreePath string) (DiffResult, error) {
	if _, err := os.Stat(worktreePath); err != nil {
		return DiffResult{}, fmt.Errorf("worktree path not found: %s", worktreePath)
	}
	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return DiffResult{}, fmt.Errorf("resolve absolute path: %w", err)
	}
	base := diffBaseLocal(absPath)
	diff, err := gitOutput(absPath, "diff", "--no-ext-diff", "--no-textconv", base+"...HEAD")
	if err != nil {
		return DiffResult{}, fmt.Errorf("git diff: %w", err)
	}
	stat, err := gitOutput(absPath, "diff", "--no-ext-diff", "--no-textconv", "--stat", base+"...HEAD")
	if err != nil {
		return DiffResult{}, fmt.Errorf("git diff --stat: %w", err)
	}
	return DiffResult{Diff: diff, Stat: stat}, nil
}

// diffBaseLocal returns, for a linked worktree, its branch's creation commit
// when that is a fork point, else merge-base of HEAD and the local default
// branch. For the root checkout it returns the local default branch.
func diffBaseLocal(path string) string {
	local := discoverBaseBranch(path)
	rootPath := rootCheckoutPath(path)
	top, err := gitOutput(path, "rev-parse", "--show-toplevel")
	if err != nil || rootPath == "" || samePath(top, rootPath) {
		return local
	}
	if commit := branchCreationCommit(path); commit != "" && !createdAtOrAfterHead(path, commit) {
		return commit
	}
	if fork, err := gitOutput(path, "merge-base", "HEAD", local); err == nil && fork != "" {
		return fork
	}
	return local
}
