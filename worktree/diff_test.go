package worktree

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestDiffWorktree(t *testing.T) {
	// Set up a temporary git repo with a main branch and a feature branch.
	// Use os.MkdirTemp instead of t.TempDir() because git background
	// processes on macOS can race with TempDir cleanup, failing the test.
	dir, err := os.MkdirTemp("", "TestDiffWorktree-*")
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

	// Initialize repo with an initial commit on main.
	run("init", "-b", "main")
	run("config", "gc.auto", "0")
	run("config", "core.fsmonitor", "false")
	if err := os.WriteFile(filepath.Join(dir, "README.md"), []byte("# hello\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "README.md")
	run("commit", "-m", "initial commit")

	// Create a feature branch and add a change.
	run("checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(dir, "new.txt"), []byte("new file\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	run("add", "new.txt")
	run("commit", "-m", "add new.txt")

	// DiffWorktree will fall back to "main" since there's no origin/HEAD.
	result, err := DiffWorktree(dir)
	if err != nil {
		t.Fatalf("DiffWorktree: %v", err)
	}

	if !strings.Contains(result.Diff, "new.txt") {
		t.Errorf("diff should mention new.txt, got:\n%s", result.Diff)
	}
	if !strings.Contains(result.Diff, "+new file") {
		t.Errorf("diff should contain added line, got:\n%s", result.Diff)
	}
	if !strings.Contains(result.Stat, "new.txt") {
		t.Errorf("stat should mention new.txt, got:\n%s", result.Stat)
	}
}

func TestDiffWorktree_NonexistentPath(t *testing.T) {
	_, err := DiffWorktree("/nonexistent/path/does/not/exist")
	if err == nil {
		t.Fatal("expected error for nonexistent path")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found', got: %v", err)
	}
}

func TestDiffWorktreeShowsOnlyCookCommits(t *testing.T) {
	dir, err := os.MkdirTemp("", "TestDiffWorktreeCookCommits-*")
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

	run("checkout", "-b", "dev")
	write(filepath.Join(dir, "dev.txt"), "dev work\n")
	run("add", "dev.txt")
	run("commit", "-m", "dev work")

	wt := filepath.Join(dir, ".worktrees", "cook")
	run("worktree", "add", wt, "-b", "cook")
	write(filepath.Join(wt, "cook.txt"), "cook work\n")
	run("-C", wt, "add", "cook.txt")
	run("-C", wt, "commit", "-m", "cook work")
	run("checkout", "main")

	result, err := DiffWorktree(wt)
	if err != nil {
		t.Fatalf("DiffWorktree: %v", err)
	}
	if !strings.Contains(result.Stat, "cook.txt") {
		t.Errorf("stat should mention cook.txt, got:\n%s", result.Stat)
	}
	if strings.Contains(result.Stat, "dev.txt") {
		t.Errorf("stat should not include the root branch's own commits, got:\n%s", result.Stat)
	}
}

func TestDiffWorktreeFromPushedBranch(t *testing.T) {
	dir, err := os.MkdirTemp("", "TestDiffWorktreePushedBranch-*")
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

	// The order's commit already exists on a remote-tracking ref.
	run("checkout", "-b", "noodle/x")
	write(filepath.Join(dir, "order.txt"), "order work\n")
	run("add", "order.txt")
	run("commit", "-m", "order work")
	run("update-ref", "refs/remotes/origin/noodle/x", "HEAD")
	run("checkout", "main")
	run("branch", "-D", "noodle/x")

	wt := filepath.Join(dir, ".worktrees", "wt")
	run("worktree", "add", "-b", "wt", wt, "origin/noodle/x")

	result, err := DiffWorktree(wt)
	if err != nil {
		t.Fatalf("DiffWorktree: %v", err)
	}
	if !strings.Contains(result.Diff, "order.txt") {
		t.Errorf("diff should contain order.txt, got:\n%s", result.Diff)
	}
	if !strings.Contains(result.Stat, "order.txt") {
		t.Errorf("stat should mention order.txt, got:\n%s", result.Stat)
	}
}
