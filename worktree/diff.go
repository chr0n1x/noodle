package worktree

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
)

// DiffResult holds the unified diff and diffstat for a worktree branch.
type DiffResult struct {
	Diff string `json:"diff"`
	Stat string `json:"stat"`
}

// DiffWorktree computes the diff between the worktree's branch HEAD and the
// base branch from diffBase.
func DiffWorktree(worktreePath string) (DiffResult, error) {
	if _, err := os.Stat(worktreePath); err != nil {
		return DiffResult{}, fmt.Errorf("worktree path not found: %s", worktreePath)
	}

	absPath, err := filepath.Abs(worktreePath)
	if err != nil {
		return DiffResult{}, fmt.Errorf("resolve absolute path: %w", err)
	}

	base := diffBase(absPath)

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

// diffBase returns the commit a linked worktree's branch (a cook's) was
// created from, so the diff shows only the cook's commits no matter which
// branch the root checkout is on now. For the root checkout itself, or when
// the creation commit is unknown, it returns discoverBaseBranch.
func diffBase(path string) string {
	rootPath := rootCheckoutPath(path)
	top, err := gitOutput(path, "rev-parse", "--show-toplevel")
	if err == nil && rootPath != "" && !samePath(top, rootPath) {
		if commit := branchCreationCommit(path); commit != "" {
			if !createdAtOrAfterHead(path, commit) {
				return commit
			}
			// The branch was cut from a ref that already held its own
			// commits (e.g. a pushed remote branch), so the creation commit
			// is no fork point. Fork from the default branch instead.
			if fork, err := gitOutput(path, "merge-base", "HEAD", defaultBaseRef(path)); err == nil && fork != "" {
				return fork
			}
		}
	}
	return discoverBaseBranch(path)
}

// createdAtOrAfterHead reports whether commit is HEAD or a descendant of it,
// i.e. merge-base of the two is HEAD.
func createdAtOrAfterHead(path, commit string) bool {
	head, err := gitOutput(path, "rev-parse", "HEAD")
	if err != nil {
		return false
	}
	fork, err := gitOutput(path, "merge-base", commit, "HEAD")
	return err == nil && fork == head
}

// defaultBaseRef returns origin/<default branch> when that ref exists, else
// the local default branch.
func defaultBaseRef(path string) string {
	name := discoverBaseBranch(path)
	if _, err := gitOutput(path, "rev-parse", "--verify", "-q", "refs/remotes/origin/"+name); err == nil {
		return "origin/" + name
	}
	return name
}

// discoverBaseBranch returns the default branch name from origin/HEAD,
// falling back to "main" on any error.
func discoverBaseBranch(repoPath string) string {
	ref, err := gitOutput(repoPath, "symbolic-ref", "refs/remotes/origin/HEAD")
	if err != nil {
		return "main"
	}
	// ref looks like "refs/remotes/origin/main" — extract branch name.
	if name := strings.TrimPrefix(ref, "refs/remotes/origin/"); name != ref {
		// Guard against ref names starting with "-" which git would
		// interpret as option flags.
		if strings.HasPrefix(name, "-") {
			return "main"
		}
		return name
	}
	return "main"
}

// rootCheckoutPath returns the path of the main worktree, the first
// `worktree <path>` line of `git worktree list --porcelain`.
func rootCheckoutPath(repoPath string) string {
	out, err := gitOutput(repoPath, "worktree", "list", "--porcelain")
	if err != nil {
		return ""
	}
	first, _, _ := strings.Cut(out, "\n")
	path, _ := strings.CutPrefix(first, "worktree ")
	return path
}

// branchCreationCommit returns the commit the branch checked out at path
// was created from: the oldest entry in its reflog. Returns "" when HEAD is
// detached or the reflog is empty or expired.
func branchCreationCommit(path string) string {
	branch, err := gitOutput(path, "symbolic-ref", "--short", "-q", "HEAD")
	if err != nil || branch == "" {
		return ""
	}
	out, err := gitOutput(path, "reflog", "show", "--format=%H", "refs/heads/"+branch, "--")
	if err != nil || out == "" {
		return ""
	}
	lines := strings.Split(out, "\n")
	return strings.TrimSpace(lines[len(lines)-1])
}

func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA != nil || errB != nil {
		return filepath.Clean(a) == filepath.Clean(b)
	}
	return ra == rb
}

func gitOutput(repoPath string, args ...string) (string, error) {
	full := append([]string{"-C", repoPath}, args...)
	cmd := exec.Command("git", full...)
	out, err := cmd.Output()
	return strings.TrimSpace(string(out)), err
}
