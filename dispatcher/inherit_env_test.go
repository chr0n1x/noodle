package dispatcher

import (
	"slices"
	"testing"
)

func TestFilterEnvKeepsBaseAndAllowed(t *testing.T) {
	environ := []string{
		"PATH=/bin", "HOME=/h", "LC_ALL=C", "TMPDIR=/t",
		"GITHUB_TOKEN=x", "GH_TOKEN=y", "AWS_PROFILE=p", "AWS_REGION=r", "SECRET=s",
	}
	got := filterEnv(environ, []string{"aws_*", "GH_TOKEN"})
	want := []string{"PATH=/bin", "HOME=/h", "LC_ALL=C", "TMPDIR=/t", "GH_TOKEN=y", "AWS_PROFILE=p", "AWS_REGION=r"}
	if !slices.Equal(got, want) {
		t.Fatalf("filterEnv = %v, want %v", got, want)
	}
}

func TestFilterEnvNilListKeepsEverything(t *testing.T) {
	environ := []string{"PATH=/bin", "GITHUB_TOKEN=x"}
	if got := filterEnv(environ, nil); !slices.Equal(got, environ) {
		t.Fatalf("filterEnv = %v", got)
	}
}

func TestSessionEnvAppliesProviderInheritEnv(t *testing.T) {
	t.Setenv("GITHUB_TOKEN", "x")
	t.Setenv("NOODLE_TEST_KEEP", "k")
	d := NewProcessDispatcher(ProcessDispatcherConfig{
		ProviderConfigs: ProviderConfigs{"copilot": {InheritEnv: []string{"NOODLE_TEST_KEEP"}}},
	})
	env := d.sessionEnv(DispatchRequest{Provider: "copilot", EnvVars: map[string]string{"EXTRA": "e"}}, "s1")
	for _, want := range []string{"NOODLE_TEST_KEEP=k", "NOODLE_PROVIDER=copilot", "NOODLE_SESSION_ID=s1", "EXTRA=e"} {
		if !slices.Contains(env, want) {
			t.Errorf("env missing %q", want)
		}
	}
	if slices.Contains(env, "GITHUB_TOKEN=x") {
		t.Error("env kept GITHUB_TOKEN outside the allow-list")
	}
	if claude := d.sessionEnv(DispatchRequest{Provider: "claude"}, "s2"); !slices.Contains(claude, "GITHUB_TOKEN=x") {
		t.Error("claude without inherit_env lost GITHUB_TOKEN")
	}
}
