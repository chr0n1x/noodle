package worktree

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCreateLinksConfiguredPaths(t *testing.T) {
	t.Parallel()
	skipWorktreeIntegrationShort(t)

	dir := setupTestRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "brain"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "brain", "todos.md"), []byte("1. [ ] item\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".noodle.toml"), []byte("mode = \"supervised\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	exclude := filepath.Join(dir, ".git", "info", "exclude")
	if err := os.WriteFile(exclude, []byte("brain/\n.noodle.toml\n.worktrees/\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	app := &App{Root: dir, LinkPaths: []string{"brain", ".noodle.toml", "missing"}}
	if err := app.Create("linked"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	wt := WorktreePath(dir, "linked")

	for _, name := range []string{"brain", ".noodle.toml"} {
		target, err := os.Readlink(filepath.Join(wt, name))
		if err != nil {
			t.Fatalf("%s is not a symlink in the worktree: %v", name, err)
		}
		if target != filepath.Join(dir, name) {
			t.Errorf("%s links to %q, want %q", name, target, filepath.Join(dir, name))
		}
	}
	if _, err := os.Lstat(filepath.Join(wt, "missing")); !os.IsNotExist(err) {
		t.Errorf("a path missing from the root was linked anyway")
	}
	if status := strings.TrimSpace(gitOutputIn(t, wt, "status", "--porcelain")); status != "" {
		t.Errorf("linked paths show up in the worktree's git status:\n%s", status)
	}
}

func TestCreateLinksNothingByDefault(t *testing.T) {
	t.Parallel()
	skipWorktreeIntegrationShort(t)

	dir := setupTestRepo(t)
	if err := os.MkdirAll(filepath.Join(dir, "brain"), 0o755); err != nil {
		t.Fatal(err)
	}
	app := &App{Root: dir}
	if err := app.Create("plain"); err != nil {
		t.Fatalf("Create: %v", err)
	}
	if _, err := os.Lstat(filepath.Join(WorktreePath(dir, "plain"), "brain")); !os.IsNotExist(err) {
		t.Errorf("brain was linked without link_paths configured")
	}
}
