package loop

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/poteto/noodle/config"
	loopruntime "github.com/poteto/noodle/runtime"
)

func TestClaudeCookPromptPointsAtInlinedSkill(t *testing.T) {
	l, rt, _ := newExternalReviewTestLoop(t, nil)
	if err := l.Cycle(context.Background()); err != nil {
		t.Fatalf("cycle: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(rt.calls))
	}
	prompt := rt.calls[0].Prompt
	if strings.Contains(prompt, "Use Skill(execute)") {
		t.Fatalf("claude prompt still asks for the Skill tool:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Follow the execute skill in your system prompt") {
		t.Fatalf("claude prompt missing inlined-skill wording:\n%s", prompt)
	}
}

func TestNonClaudeCookPromptKeepsSkillTool(t *testing.T) {
	got := skillWordingForProvider("Use Skill(execute) for methodology.", "codex", "execute")
	if got != "Use Skill(execute) for methodology." {
		t.Fatalf("codex prompt changed: %q", got)
	}
}

func TestClaudeSchedulerPromptPointsAtInlinedSkill(t *testing.T) {
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	if err := os.MkdirAll(runtimeDir, 0o755); err != nil {
		t.Fatalf("mkdir runtime: %v", err)
	}
	ordersPath := filepath.Join(runtimeDir, "orders.json")
	cfg := config.DefaultConfig()
	order := scheduleOrder(cfg, "")
	if err := writeOrdersAtomic(ordersPath, OrdersFile{Orders: []Order{order}}); err != nil {
		t.Fatalf("write orders: %v", err)
	}
	rt := newMockRuntime()
	l := New(projectDir, "noodle", cfg, Dependencies{
		Runtimes:   map[string]loopruntime.Runtime{"process": rt},
		Worktree:   &fakeWorktree{},
		Adapter:    &fakeAdapterRunner{},
		Mise:       &fakeMise{},
		Monitor:    fakeMonitor{},
		Registry:   testLoopRegistry(),
		Now:        time.Now,
		OrdersFile: ordersPath,
	})
	if err := l.spawnSchedule(context.Background(), order, 0, ""); err != nil {
		t.Fatalf("spawnSchedule: %v", err)
	}
	if len(rt.calls) != 1 {
		t.Fatalf("dispatch calls = %d, want 1", len(rt.calls))
	}
	prompt := rt.calls[0].Prompt
	if strings.Contains(prompt, "Skill(schedule)") {
		t.Fatalf("claude scheduler prompt still asks for the Skill tool:\n%s", prompt)
	}
	if !strings.Contains(prompt, "Follow the schedule skill in your system prompt") {
		t.Fatalf("claude scheduler prompt missing inlined-skill wording:\n%s", prompt)
	}
}

func TestSkillWordingRewritesAnySkillToolLine(t *testing.T) {
	got := skillWordingForProvider("Use Skill(schedule) to refresh the schedule from mise.json.", "claude", "schedule")
	want := "Follow the schedule skill in your system prompt (it is already loaded; do not call the Skill tool) to refresh the schedule from mise.json."
	if got != want {
		t.Fatalf("got %q\nwant %q", got, want)
	}
}
