package dispatcher

import (
	"context"
	"io"
	"os/exec"
	"strings"
	"testing"
)

func TestComposePromptsUsesSystemPromptForPi(t *testing.T) {
	for _, provider := range []string{"pi", "pig"} {
		system, final := composePrompts(provider, "do it", "preamble", "skill text", "stage override")
		if final != "do it" || system != "preamble\n\nskill text\n\nstage override" {
			t.Fatalf("%s: system %q final %q", provider, system, final)
		}
	}
}

func TestBuildCmdPi(t *testing.T) {
	d := NewProcessDispatcher(ProcessDispatcherConfig{
		ProviderConfigs: ProviderConfigs{"pig": {Args: []string{"--no-mcp"}}},
	})
	for _, provider := range []string{"pi", "pig"} {
		cmd, err := d.buildCmd(DispatchRequest{
			Provider: provider, Model: "github-copilot/claude-sonnet-5.5", ReasoningLevel: "high",
			MaxTurns: 4, BudgetCap: 2,
		}, "SYS")
		if err != nil {
			t.Fatal(err)
		}
		if cmd.Args[0] != provider {
			t.Fatalf("binary = %q", cmd.Args[0])
		}
		got := strings.Join(cmd.Args[1:], " ")
		for _, want := range []string{"-p", "--mode json", "--model github-copilot/claude-sonnet-5.5", "--thinking high", "--append-system-prompt SYS"} {
			if !strings.Contains(got, want) {
				t.Errorf("%s args %q missing %q", provider, got, want)
			}
		}
		for _, omit := range []string{"--max-turns", "--max-budget-usd", "stream-json"} {
			if strings.Contains(got, omit) {
				t.Errorf("%s args %q contain unsupported %q", provider, got, omit)
			}
		}
		if provider == "pig" && cmd.Args[len(cmd.Args)-1] != "--no-mcp" {
			t.Errorf("pig extra args not appended: %v", cmd.Args)
		}
	}
}

func TestConfigureStdinPipesPromptForPi(t *testing.T) {
	process, err := StartProcess(exec.Command("cat"))
	if err != nil {
		t.Fatal(err)
	}
	d := NewProcessDispatcher(ProcessDispatcherConfig{})
	controller, err := d.configureStdin(context.Background(), process, "pi", "hello pi", false)
	if err != nil || controller != nil {
		t.Fatalf("controller %v err %v, want one-shot stdin", controller, err)
	}
	out, _ := io.ReadAll(process.Stdout())
	if string(out) != "hello pi" {
		t.Fatalf("stdin got %q", out)
	}
}
