package dispatcher

import (
	"slices"
	"strings"
	"testing"
)

func TestComposePromptsStagePromptFollowsSkillForClaude(t *testing.T) {
	system, final := composePrompts("claude", "do it", "preamble", "skill text", "stage override")
	if final != "do it" {
		t.Fatalf("final prompt = %q", final)
	}
	if system != "preamble\n\nskill text\n\nstage override" {
		t.Fatalf("system prompt = %q", system)
	}
}

func TestComposePromptsStagePromptInlinedForCodex(t *testing.T) {
	system, final := composePrompts("codex", "do it", "preamble", "skill text", "stage override")
	if system != "" {
		t.Fatalf("system prompt = %q, want none for codex", system)
	}
	if !strings.HasPrefix(final, "do it\n\n---\n\n") || !strings.HasSuffix(final, "preamble\n\nstage override") {
		t.Fatalf("final prompt = %q", final)
	}
}

func TestCodexArgsSetModelProvider(t *testing.T) {
	args := codexBaseArgs(DispatchRequest{Model: "gpt-5.5", ModelProvider: "openai"})
	i := slices.Index(args, "-c")
	if i < 0 || i+1 >= len(args) || args[i+1] != `model_provider="openai"` {
		t.Fatalf("args = %v, want -c model_provider=\"openai\"", args)
	}
	if slices.Contains(codexBaseArgs(DispatchRequest{Model: "gpt-5.5"}), "-c") {
		t.Fatal("no provider should add no -c")
	}
}
