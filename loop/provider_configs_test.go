package loop

import (
	"testing"

	"github.com/poteto/noodle/config"
)

func TestProviderConfigsFromCoversEveryProvider(t *testing.T) {
	agents := config.AgentsConfig{
		Claude: config.ProviderConfig{Path: "/c", ExitAfterTurn: true},
		Maki:   config.ProviderConfig{Path: "/m", Args: []string{"--yolo"}, ExitAfterTurn: true},
		Pig:    config.ProviderConfig{Path: "/g"},
	}
	got := ProviderConfigsFrom(agents)
	if len(got) != len(config.Providers) {
		t.Fatalf("len = %d, want %d", len(got), len(config.Providers))
	}
	if !got["claude"].ExitAfterTurn || got["claude"].Path != "/c" {
		t.Errorf("claude = %+v", got["claude"])
	}
	if m := got["maki"]; m.Path != "/m" || len(m.Args) != 1 || !m.ExitAfterTurn {
		t.Errorf("maki = %+v", m)
	}
	if got["pig"].Path != "/g" {
		t.Errorf("pig = %+v", got["pig"])
	}
}

func TestProviderConfigsFromResolvesInheritEnv(t *testing.T) {
	agents := config.AgentsConfig{
		InheritEnv: []string{"GITHUB_TOKEN"},
		Copilot:    config.ProviderConfig{InheritEnv: []string{}},
	}
	got := ProviderConfigsFrom(agents)
	if c := got["copilot"]; c.InheritEnv == nil || len(c.InheritEnv) != 0 {
		t.Errorf("copilot InheritEnv = %#v, want empty list", c.InheritEnv)
	}
	if c := got["claude"]; len(c.InheritEnv) != 1 || c.InheritEnv[0] != "GITHUB_TOKEN" {
		t.Errorf("claude InheritEnv = %#v, want global list", c.InheritEnv)
	}
	if c := ProviderConfigsFrom(config.AgentsConfig{})["pi"]; c.InheritEnv != nil {
		t.Errorf("pi InheritEnv = %#v, want nil", c.InheritEnv)
	}
}
