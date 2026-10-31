package dispatcher

import (
	"os"
	"path/filepath"
	"slices"
	"testing"
)

func TestResolveAgentBinaryPerProvider(t *testing.T) {
	dir := t.TempDir()
	for _, name := range []string{"pi", "maki"} {
		if err := os.WriteFile(filepath.Join(dir, name), nil, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	d := NewProcessDispatcher(ProcessDispatcherConfig{
		ProviderConfigs: ProviderConfigs{
			"pi":   {Path: dir},
			"maki": {Path: dir},
		},
	})
	cases := map[string]string{
		"pi":      filepath.Join(dir, "pi"),
		"maki":    filepath.Join(dir, "maki"),
		"pig":     "pig",
		"copilot": "copilot",
		"codex":   "codex",
		"":        "claude",
	}
	for provider, want := range cases {
		if got := d.resolveAgentBinary(provider); got != want {
			t.Errorf("resolveAgentBinary(%q) = %q, want %q", provider, got, want)
		}
	}
}

func TestResolveExtraArgsPerProvider(t *testing.T) {
	d := NewProcessDispatcher(ProcessDispatcherConfig{
		ProviderConfigs: ProviderConfigs{
			"claude":  {Args: []string{"--c"}},
			"copilot": {Args: []string{"--model", "gpt-5"}},
		},
	})
	if got := d.resolveExtraArgs("copilot"); !slices.Equal(got, []string{"--model", "gpt-5"}) {
		t.Errorf("copilot args = %v", got)
	}
	if got := d.resolveExtraArgs(""); !slices.Equal(got, []string{"--c"}) {
		t.Errorf("default args = %v", got)
	}
	if got := d.resolveExtraArgs("pi"); got != nil {
		t.Errorf("pi args = %v, want nil", got)
	}
}
