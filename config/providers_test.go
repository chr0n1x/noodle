package config

import "testing"

var newProviders = []string{"pi", "pig", "maki", "copilot"}

func TestProvidersListsEveryHarness(t *testing.T) {
	want := append([]string{"claude", "codex"}, newProviders...)
	if len(Providers) != len(want) {
		t.Fatalf("Providers = %v, want %v", Providers, want)
	}
	for i, p := range want {
		if Providers[i] != p {
			t.Fatalf("Providers = %v, want %v", Providers, want)
		}
	}
}

func TestRoutingAcceptsNewProviders(t *testing.T) {
	for _, p := range newProviders {
		t.Run(p, func(t *testing.T) {
			cfg, err := Parse([]byte("[routing.defaults]\nprovider = \"" + p + "\"\nmodel = \"m\"\n"))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if cfg.Routing.Defaults.Provider != p {
				t.Fatalf("provider = %q, want %q", cfg.Routing.Defaults.Provider, p)
			}
			if d := findDiagnostic(cfg, DiagnosticCodeProviderUnknown); d != nil {
				t.Fatalf("unexpected diagnostic %+v", *d)
			}
		})
	}
}

func TestStageHarnessAcceptsNewProviders(t *testing.T) {
	for _, p := range newProviders {
		t.Run(p, func(t *testing.T) {
			cfg, err := Parse([]byte("[stages.review]\nharness = \"" + p + "\"\n"))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			if d := findDiagnostic(cfg, DiagnosticCodeStageHarnessUnknown); d != nil {
				t.Fatalf("unexpected diagnostic %+v", *d)
			}
		})
	}
}

func TestAgentsTablesForNewProviders(t *testing.T) {
	cfg, err := Parse([]byte(`
[agents.pi]
path = "/opt/pi"
args = ["--tools", "read,bash"]

[agents.pig]
args = ["--no-stdin"]

[agents.maki]
exit_after_turn = true

[agents.copilot]
args = ["--allow-all-paths"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Agents.For("pi"); got.Path != "/opt/pi" || len(got.Args) != 2 {
		t.Fatalf("pi = %+v", got)
	}
	if got := cfg.Agents.For("pig"); len(got.Args) != 1 || got.Args[0] != "--no-stdin" {
		t.Fatalf("pig = %+v", got)
	}
	if !cfg.Agents.For("maki").ExitAfterTurn {
		t.Fatal("maki exit_after_turn not parsed")
	}
	if got := cfg.Agents.For("copilot"); len(got.Args) != 1 {
		t.Fatalf("copilot = %+v", got)
	}
	if got := cfg.Agents.For("unknown"); got.Path != "" || got.Args != nil {
		t.Fatalf("unknown = %+v, want zero", got)
	}
}

func TestInheritEnvProviderListReplacesGlobal(t *testing.T) {
	cfg, err := Parse([]byte(`
[agents]
inherit_env = ["GITHUB_TOKEN", "AWS_*"]

[agents.copilot]
inherit_env = []

[agents.pi]
inherit_env = ["OPENAI_API_KEY"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cases := map[string][]string{
		"claude":  {"GITHUB_TOKEN", "AWS_*"},
		"copilot": {},
		"pi":      {"OPENAI_API_KEY"},
	}
	for provider, want := range cases {
		got, ok := cfg.Agents.InheritEnvFor(provider)
		if !ok || len(got) != len(want) {
			t.Fatalf("%s: InheritEnvFor = %v, %v, want %v", provider, got, ok, want)
		}
		for i := range want {
			if got[i] != want[i] {
				t.Fatalf("%s: InheritEnvFor = %v, want %v", provider, got, want)
			}
		}
	}
}

func TestInheritEnvUnsetInheritsEverything(t *testing.T) {
	cfg, err := Parse([]byte("[agents.copilot]\npath = \"/bin\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got, ok := cfg.Agents.InheritEnvFor("copilot"); ok {
		t.Fatalf("InheritEnvFor = %v, true; want no list", got)
	}
}
