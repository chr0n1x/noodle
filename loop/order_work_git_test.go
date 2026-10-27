package loop

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	loopruntime "github.com/poteto/noodle/runtime"
	"github.com/poteto/noodle/worktree"
)

func TestKeptWorkChainsAndCleansUpWithRealGit(t *testing.T) {
	repo := t.TempDir()
	runGitT(t, repo, "init", "-q", "-b", "main")
	runGitT(t, repo, "config", "user.email", "t@example.com")
	runGitT(t, repo, "config", "user.name", "t")
	if err := os.WriteFile(filepath.Join(repo, ".gitignore"), []byte(".worktrees\n.noodle/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitT(t, repo, "add", ".gitignore")
	runGitT(t, repo, "commit", "-q", "-m", "init")

	runtimeDir := filepath.Join(repo, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatal(err)
	}
	ordersPath := writeTestOrders(t, runtimeDir, OrdersFile{Orders: []Order{{
		ID: "42", Title: "chain", Status: OrderStatusActive, Stages: []Stage{
			{TaskKey: "execute", Skill: "execute", Status: StageStatusActive},
			{TaskKey: "review", Skill: "review", Status: StageStatusPending},
			{TaskKey: "execute", Skill: "execute", Status: StageStatusPending},
		},
	}}})
	cfg := config.DefaultConfig()
	cfg.Review.ExternalStages = []string{"review"}
	wt := &worktree.App{Root: repo, IntegrationBranch: "main", Quiet: true}
	l := New(repo, "noodle", cfg, Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": newMockRuntime()},
		Worktree:   wt,
		Adapter:    &fakeAdapterRunner{},
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: ordersPath,
	})

	if err := wt.Create("42-0-execute"); err != nil {
		t.Fatalf("create execute worktree: %v", err)
	}
	execDir := filepath.Join(repo, ".worktrees", "42-0-execute")
	if err := os.WriteFile(filepath.Join(execDir, "work.txt"), []byte("work\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	runGitT(t, execDir, "add", "work.txt")
	runGitT(t, execDir, "commit", "-q", "-m", "execute work")
	l.cooks.pendingReview["42"] = &pendingReviewCook{
		cookIdentity: cookIdentity{orderID: "42", stageIndex: 0, stage: Stage{TaskKey: "execute", Skill: "execute"}},
		worktreeName: "42-0-execute",
		worktreePath: execDir,
		sessionID:    "sess-42",
	}

	if err := l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	reviewDir := filepath.Join(repo, ".worktrees", "42-1-review")
	if _, err := os.Stat(filepath.Join(reviewDir, "work.txt")); err != nil {
		t.Fatalf("review worktree is missing the execute commit: %v", err)
	}

	if err := mergePending(l, "42"); err != nil {
		t.Fatalf("accept with merge: %v", err)
	}
	if got := runGitT(t, repo, "show", "main:work.txt"); got != "work" {
		t.Fatalf("main:work.txt = %q, want the execute work merged", got)
	}
	if got := runGitT(t, repo, "branch", "--list", "42-*"); got != "" {
		t.Fatalf("kept branches left after publish:\n%s", got)
	}
	if got := runGitT(t, repo, "worktree", "list", "--porcelain"); containsLine(got, execDir) || containsLine(got, reviewDir) {
		t.Fatalf("kept worktrees left after publish:\n%s", got)
	}
}

func containsLine(out, path string) bool {
	resolved, err := filepath.EvalSymlinks(filepath.Dir(path))
	if err == nil {
		path = filepath.Join(resolved, filepath.Base(path))
	}
	for _, line := range splitLines(out) {
		if line == "worktree "+path {
			return true
		}
	}
	return false
}

func splitLines(s string) []string {
	var lines []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			lines = append(lines, s[start:i])
			start = i + 1
		}
	}
	return append(lines, s[start:])
}
