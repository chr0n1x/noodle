package dispatcher

import (
	"slices"
	"strings"
	"testing"
)

func TestSpriteAgentArgsPerProvider(t *testing.T) {
	cases := map[string]struct{ contains, omits []string }{
		"claude":  {contains: []string{"claude", "-p", "--append-system-prompt"}},
		"codex":   {contains: []string{"codex", "exec"}},
		"maki":    {contains: []string{"maki", "-p", "--output-format", "--append-system-prompt"}, omits: []string{"--input-format"}},
		"pi":      {contains: []string{"pi", "-p", "--mode", "--append-system-prompt"}},
		"pig":     {contains: []string{"pig", "-p", "--mode"}},
		"copilot": {contains: []string{"copilot", "--output-format", "--yolo"}, omits: []string{"--append-system-prompt"}},
	}
	for provider, tc := range cases {
		args := spriteAgentArgs(DispatchRequest{Provider: provider}, "SYS")
		if args[0] != provider {
			t.Errorf("%s: binary = %q", provider, args[0])
		}
		for _, want := range tc.contains {
			if !slices.Contains(args, want) {
				t.Errorf("%s: args %v missing %q", provider, args, want)
			}
		}
		for _, omit := range tc.omits {
			if slices.Contains(args, omit) {
				t.Errorf("%s: args %s contain %q", provider, strings.Join(args, " "), omit)
			}
		}
	}
}
