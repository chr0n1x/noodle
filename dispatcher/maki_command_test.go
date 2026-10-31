package dispatcher

import (
	"slices"
	"strings"
	"testing"
)

func TestComposePromptsUsesSystemPromptForMaki(t *testing.T) {
	system, final := composePrompts("maki", "do it", "preamble", "skill text", "stage override")
	if final != "do it" || system != "preamble\n\nskill text\n\nstage override" {
		t.Fatalf("system %q final %q", system, final)
	}
}

func TestBuildCmdMaki(t *testing.T) {
	d := NewProcessDispatcher(ProcessDispatcherConfig{
		ProviderConfigs: ProviderConfigs{"maki": {Args: []string{"--trust"}}},
	})
	cmd, err := d.buildCmd(DispatchRequest{
		Provider: "maki", Model: "anthropic/claude-opus-5.5", MaxTurns: 7,
		BudgetCap: 3, ReasoningLevel: "high",
	}, "SYS")
	if err != nil {
		t.Fatal(err)
	}
	if cmd.Args[0] != "maki" {
		t.Fatalf("binary = %q", cmd.Args[0])
	}
	got := strings.Join(cmd.Args[1:], " ")
	for _, want := range []string{
		"-p", "--input-format stream-json", "--output-format stream-json", "--verbose", "--yolo",
		"--max-turns 7", "--model anthropic/claude-opus-5.5", "--append-system-prompt SYS",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("args %q missing %q", got, want)
		}
	}
	for _, omit := range []string{"--max-budget-usd", "--effort", "--permission-mode"} {
		if strings.Contains(got, omit) {
			t.Errorf("args %q contain unsupported %q", got, omit)
		}
	}
	if cmd.Args[len(cmd.Args)-1] != "--trust" {
		t.Errorf("extra args not appended last: %v", cmd.Args)
	}
	if slices.Contains(cmd.Args, "") {
		t.Errorf("empty arg in %v", cmd.Args)
	}
}
