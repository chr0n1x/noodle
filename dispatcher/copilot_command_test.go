package dispatcher

import (
	"strings"
	"testing"
)

func TestComposePromptsInlinesEverythingForCopilot(t *testing.T) {
	system, final := composePrompts("copilot", "do it", "preamble", "skill text", "stage override")
	if system != "" {
		t.Fatalf("system prompt = %q, want none for copilot", system)
	}
	if final != "do it\n\n---\n\npreamble\n\nskill text\n\nstage override" {
		t.Fatalf("final prompt = %q", final)
	}
}

func TestBuildCmdCopilot(t *testing.T) {
	d := NewProcessDispatcher(ProcessDispatcherConfig{})
	cmd, err := d.buildCmd(DispatchRequest{
		Provider: "copilot", Model: "gpt-6.1-sol", ReasoningLevel: "high", MaxTurns: 3,
	}, "")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Args[0] != "copilot" {
		t.Fatalf("binary = %q", cmd.Args[0])
	}
	got := strings.Join(cmd.Args[1:], " ")
	for _, want := range []string{"--output-format json", "--yolo", "--no-color", "--model gpt-6.1-sol", "--reasoning-effort high"} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
	for _, omit := range []string{"-p", "--max-turns", "stream-json"} {
		for _, a := range cmd.Args {
			if a == omit {
				t.Errorf("args %q contain %q", got, omit)
			}
		}
	}
}
