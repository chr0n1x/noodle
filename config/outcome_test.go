package config

import (
	"slices"
	"testing"
)

func TestStageActionUnsetByDefault(t *testing.T) {
	cfg := DefaultConfig()
	for _, state := range []StageState{StageStateAccept, StageStateReject} {
		if got, ok := cfg.StageAction("execute", state); ok {
			t.Fatalf("unset %s action = %q, want none", state, got)
		}
	}
}

func TestParseStageActions(t *testing.T) {
	cfg, err := Parse([]byte(`
[stages.execute]
accept = "promote"
reject = "retry"

[stages.adversarial-review]
reject = "retry-previous"
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	cases := []struct {
		task  string
		state StageState
		want  StageAction
		set   bool
	}{
		{"execute", StageStateAccept, StageActionPromote, true},
		{"execute", StageStateReject, StageActionRetry, true},
		{"adversarial-review", StageStateAccept, "", false},
		{"adversarial-review", StageStateReject, StageActionRetryPrevious, true},
		{"quality", StageStateReject, "", false},
	}
	for _, c := range cases {
		got, ok := cfg.StageAction(c.task, c.state)
		if ok != c.set || (ok && got != c.want) {
			t.Errorf("%s %s = %q, %v; want %q, %v", c.task, c.state, got, ok, c.want, c.set)
		}
	}
	if d := findDiagnostic(cfg, DiagnosticCodeStageActionUnknown); d != nil {
		t.Fatalf("unexpected diagnostic %+v", *d)
	}
}

func TestValidateRejectsUnknownStageAction(t *testing.T) {
	cfg, err := Parse([]byte("[stages.execute]\nreject = \"retry-last\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	d := findDiagnostic(cfg, DiagnosticCodeStageActionUnknown)
	if d == nil {
		t.Fatal("expected a stages.execute.reject diagnostic")
	}
	if d.Severity != DiagnosticSeverityFatal || d.FieldPath != "stages.execute.reject" || d.Fix == "" {
		t.Fatalf("diagnostic = %+v", *d)
	}
	if Validate(cfg).CanSpawn() {
		t.Fatal("an unknown action should stop the loop from spawning")
	}
}

func TestParseActionAcceptsSpacesAndCase(t *testing.T) {
	for in, want := range map[string]StageAction{
		"promote": StageActionPromote, " Exit ": StageActionExit, "RETRY-previous": StageActionRetryPrevious,
	} {
		got, ok := ParseStageAction(in)
		if !ok || got != want {
			t.Errorf("ParseStageAction(%q) = %q, %v", in, got, ok)
		}
	}
	if _, ok := ParseStageAction("stop"); ok {
		t.Error("stop should not be an action")
	}
}

func TestHooksDefaults(t *testing.T) {
	h := DefaultConfig().Hooks
	if got := h.AfterOrder(); !slices.Equal(got, []string{"reflect"}) {
		t.Fatalf("default after = %v, want [reflect]", got)
	}
	if len(h.Before) != 0 || len(h.BeforeEach) != 0 || len(h.AfterEach) != 0 {
		t.Fatalf("other hooks should default empty: %+v", h)
	}
}

func TestParseHooks(t *testing.T) {
	cfg, err := Parse([]byte(`
[hooks]
before = ["prep"]
after = []
before_each = ["lint"]
after_each = ["note"]
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	h := cfg.Hooks
	if got := h.AfterOrder(); len(got) != 0 {
		t.Fatalf("after = [] should turn reflect off, got %v", got)
	}
	if !slices.Equal(h.Before, []string{"prep"}) || !slices.Equal(h.BeforeEach, []string{"lint"}) ||
		!slices.Equal(h.AfterEach, []string{"note"}) {
		t.Fatalf("hooks = %+v", h)
	}

	cfg, err = Parse([]byte("[hooks]\nbefore = [\"prep\"]\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Hooks.AfterOrder(); !slices.Equal(got, []string{"reflect"}) {
		t.Fatalf("unset after = %v, want [reflect]", got)
	}
}

func TestHooksIncludes(t *testing.T) {
	h := DefaultConfig().Hooks
	h.BeforeEach = []string{"lint"}
	for name, want := range map[string]bool{"reflect": true, "lint": true, "execute": false, "": false} {
		if got := h.Includes(name); got != want {
			t.Errorf("Includes(%q) = %v, want %v", name, got, want)
		}
	}
}

func TestParseStageRouting(t *testing.T) {
	cfg, err := Parse([]byte(`
[stages.adversarial-review]
harness = "codex"
provider = "openai"
model = "gpt-5.5"
system_prompt = """
Use pi for reviewers."""
`))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	s := cfg.Stages["adversarial-review"]
	if s.Harness != "codex" || s.Provider != "openai" || s.Model != "gpt-5.5" || s.SystemPrompt != "Use pi for reviewers." {
		t.Fatalf("stage config = %+v", s)
	}
	for _, code := range []string{DiagnosticCodeStageHarnessUnknown, DiagnosticCodeStageProviderUnsupported} {
		if d := findDiagnostic(cfg, code); d != nil {
			t.Fatalf("unexpected diagnostic %+v", *d)
		}
	}
}

func TestValidateRejectsStageRouting(t *testing.T) {
	cases := []struct {
		name, toml, code, path string
	}{
		{"unknown harness", "[stages.review]\nharness = \"pi\"\n", DiagnosticCodeStageHarnessUnknown, "stages.review.harness"},
		{"provider with claude", "[stages.review]\nharness = \"claude\"\nprovider = \"bedrock\"\n", DiagnosticCodeStageProviderUnsupported, "stages.review.provider"},
		{"provider without harness", "[stages.review]\nprovider = \"openai\"\n", DiagnosticCodeStageProviderUnsupported, "stages.review.provider"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			cfg, err := Parse([]byte(c.toml))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			d := findDiagnostic(cfg, c.code)
			if d == nil {
				t.Fatalf("expected a %s diagnostic", c.code)
			}
			if d.Severity != DiagnosticSeverityFatal || d.FieldPath != c.path || d.Fix == "" {
				t.Fatalf("diagnostic = %+v", *d)
			}
		})
	}
}
