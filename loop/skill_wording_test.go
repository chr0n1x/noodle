package loop

import (
	"context"
	"strings"
	"testing"
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
