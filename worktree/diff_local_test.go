package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// staleOriginRepo builds a repo whose origin/main (with origin/HEAD pointing
// at it) is one commit behind local main, the commit that adds upstream.txt.
func staleOriginRepo(t *testing.T) (string, func(args ...string), func(path, body string)) {
	t.Helper()
	dir, err := os.MkdirTemp("", "TestDiffWorktreeStaleOrigin-*")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })

	run := func(args ...string) {
		t.Helper()
		cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
		cmd.Env = append(os.Environ(),
			"GIT_AUTHOR_NAME=test",
			"GIT_AUTHOR_EMAIL=test@test.com",
			"GIT_COMMITTER_NAME=test",
			"GIT_COMMITTER_EMAIL=test@test.com",
		)
		out, err := cmd.CombinedOutput()
		if err != nil {
			t.Fatalf("git %v failed: %v\n%s", args, err, out)
		}
	}
	write := func(path, body string) {
		t.Helper()
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	run("init", "-b", "main")
	run("config", "gc.auto", "0")
	run("config", "core.fsmonitor", "false")
	write(filepath.Join(dir, "README.md"), "# hello\n")
	run("add", "README.md")
	run("commit", "-m", "initial commit")
	run("update-ref", "refs/remotes/origin/main", "HEAD")
	run("symbolic-ref", "refs/remotes/origin/HEAD", "refs/remotes/origin/main")
	write(filepath.Join(dir, "upstream.txt"), "upstream work\n")
	run("add", "upstream.txt")
	run("commit", "-m", "local main moves past origin/main")
	return dir, run, write
}

func TestDiffWorktreeLocalNoCommitsWithStaleOrigin(t *testing.T) {
	dir, run, _ := staleOriginRepo(t)
	wt := filepath.Join(dir, ".worktrees", "cook")
	run("worktree", "add", "-b", "cook", wt, "main")

	result, err := DiffWorktreeLocal(wt)
	if err != nil {
		t.Fatalf("DiffWorktreeLocal: %v", err)
	}
	if result.Diff != "" || result.Stat != "" {
		t.Errorf("cook without commits should have an empty diff, got stat:\n%s", result.Stat)
	}
}

func TestDiffWorktreeLocalOneCommitWithStaleOrigin(t *testing.T) {
	dir, run, write := staleOriginRepo(t)
	wt := filepath.Join(dir, ".worktrees", "cook")
	run("worktree", "add", "-b", "cook", wt, "main")
	write(filepath.Join(wt, "cook.txt"), "cook work\n")
	run("-C", wt, "add", "cook.txt")
	run("-C", wt, "commit", "-m", "cook work")

	result, err := DiffWorktreeLocal(wt)
	if err != nil {
		t.Fatalf("DiffWorktreeLocal: %v", err)
	}
	if !strings.Contains(result.Stat, "cook.txt") {
		t.Errorf("stat should mention cook.txt, got:\n%s", result.Stat)
	}
	if strings.Contains(result.Stat, "upstream.txt") {
		t.Errorf("stat should not include commits between origin/main and main, got:\n%s", result.Stat)
	}
}

func TestDiffWorktreeLocalFromPushedBranch(t *testing.T) {
	dir, run, write := staleOriginRepo(t)
	run("checkout", "-b", "noodle/x")
	write(filepath.Join(dir, "order.txt"), "order work\n")
	run("add", "order.txt")
	run("commit", "-m", "order work")
	run("update-ref", "refs/remotes/origin/noodle/x", "HEAD")
	run("checkout", "main")
	run("branch", "-D", "noodle/x")

	wt := filepath.Join(dir, ".worktrees", "wt")
	run("worktree", "add", "-b", "wt", wt, "origin/noodle/x")

	result, err := DiffWorktreeLocal(wt)
	if err != nil {
		t.Fatalf("DiffWorktreeLocal: %v", err)
	}
	if !strings.Contains(result.Stat, "order.txt") {
		t.Errorf("stat should mention order.txt, got:\n%s", result.Stat)
	}
	if strings.Contains(result.Stat, "upstream.txt") {
		t.Errorf("stat should not include commits between origin/main and main, got:\n%s", result.Stat)
	}
}

func TestDiffWorktreeLocalRootCheckout(t *testing.T) {
	dir, run, write := staleOriginRepo(t)
	run("checkout", "-b", "feature")
	write(filepath.Join(dir, "feature.txt"), "feature work\n")
	run("add", "feature.txt")
	run("commit", "-m", "feature work")

	result, err := DiffWorktreeLocal(dir)
	if err != nil {
		t.Fatalf("DiffWorktreeLocal: %v", err)
	}
	if !strings.Contains(result.Stat, "feature.txt") || strings.Contains(result.Stat, "upstream.txt") {
		t.Errorf("stat should hold only feature.txt, got:\n%s", result.Stat)
	}
}

// Pins upstream DiffWorktree, which forks from origin/<default>: a cook
// without commits shows the commits between origin/main and main.
func TestDiffWorktreeUsesOriginWithStaleOrigin(t *testing.T) {
	dir, run, _ := staleOriginRepo(t)
	wt := filepath.Join(dir, ".worktrees", "cook")
	run("worktree", "add", "-b", "cook", wt, "main")

	result, err := DiffWorktree(wt)
	if err != nil {
		t.Fatalf("DiffWorktree: %v", err)
	}
	if !strings.Contains(result.Stat, "upstream.txt") {
		t.Errorf("upstream DiffWorktree should diff against origin/main, got stat:\n%s", result.Stat)
	}
}
