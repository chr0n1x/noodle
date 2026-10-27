package config

import (
	"strings"
	"testing"
)

func TestParseReviewExternalStages(t *testing.T) {
	cfg, err := Parse([]byte(`
[review]
external_stages = ["quality", "adversarial-review"]
`))
	if err != nil {
		t.Fatalf("Parse config: %v", err)
	}
	r := cfg.Review
	if len(r.ExternalStages) != 2 {
		t.Fatalf("external_stages = %v", r.ExternalStages)
	}
	if !r.IsExternal("quality", "") {
		t.Fatal("quality task key should be external")
	}
	if !r.IsExternal("review", "adversarial-review") {
		t.Fatal("adversarial-review skill should be external")
	}
	if r.IsExternal("execute", "execute") {
		t.Fatal("execute should not be external")
	}
	if DefaultConfig().Review.IsExternal("quality", "quality") {
		t.Fatal("no stage should be external by default")
	}
}

func findDiagnostic(cfg Config, code string) *ConfigDiagnostic {
	for _, d := range Validate(cfg).Diagnostics {
		if d.Code == code {
			return &d
		}
	}
	return nil
}

func TestReviewAcceptSettingsWarnAndAreIgnored(t *testing.T) {
	cases := []struct{ key, code string }{
		{"on_accept", DiagnosticCodeReviewOnAcceptIgnored},
		{"on_quality_accept", DiagnosticCodeReviewOnQualityAcceptIgnored},
		{"on_adversarial_review_accept", DiagnosticCodeReviewOnAdversarialReviewAcceptIgnored},
	}
	for _, c := range cases {
		t.Run(c.key, func(t *testing.T) {
			cfg, err := Parse([]byte("[review]\n" + c.key + " = \"draft-pr\"\n"))
			if err != nil {
				t.Fatalf("Parse: %v", err)
			}
			d := findDiagnostic(cfg, c.code)
			if d == nil {
				t.Fatalf("expected a review.%s diagnostic", c.key)
			}
			if d.Severity != DiagnosticSeverityWarning || d.FieldPath != "review."+c.key || d.Fix == "" ||
				!strings.Contains(d.Message, c.key+" is ignored") {
				t.Fatalf("diagnostic = %+v", *d)
			}
			if d := findDiagnostic(DefaultConfig(), c.code); d != nil {
				t.Fatalf("unset %s, unexpected diagnostic %+v", c.key, *d)
			}
		})
	}
}

func TestReviewMaxRework(t *testing.T) {
	if got := DefaultConfig().Review.MaxReworkRounds(); got != 2 {
		t.Fatalf("default max_rework = %d, want 2", got)
	}
	cfg, err := Parse([]byte("[review]\nmax_rework = 0\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if got := cfg.Review.MaxReworkRounds(); got != 0 {
		t.Fatalf("max_rework = %d, want 0", got)
	}
}

func TestValidateWarnsOnNegativeMaxRework(t *testing.T) {
	cfg := DefaultConfig()
	n := -1
	cfg.Review.MaxRework = &n
	found := false
	for _, d := range Validate(cfg).Diagnostics {
		if d.Code == DiagnosticCodeReviewMaxReworkInvalid && d.FieldPath == "review.max_rework" && d.Fix != "" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a review.max_rework diagnostic")
	}
	if cfg.Review.MaxReworkRounds() != 0 {
		t.Fatal("a negative max_rework should act as 0")
	}
}

func TestParseReviewPublishAction(t *testing.T) {
	if got := DefaultConfig().Review.PublishAction; got != "" {
		t.Fatalf("default publish_action = %q, want empty", got)
	}
	cfg, err := Parse([]byte("[review]\npublish_action = \"draft-pr\"\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Review.PublishAction != "draft-pr" {
		t.Fatalf("publish_action = %q, want draft-pr", cfg.Review.PublishAction)
	}
}

func TestValidateWarnsOnUnknownPublishActionSkill(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Skills.Paths = []string{t.TempDir()}
	cfg.Review.PublishAction = "ship-it"
	found := false
	for _, d := range Validate(cfg).Diagnostics {
		if d.Code == DiagnosticCodeReviewPublishActionUnknown && d.FieldPath == "review.publish_action" {
			found = true
		}
	}
	if !found {
		t.Fatal("expected a review.publish_action diagnostic")
	}
	for _, action := range []string{"", "draft-pr"} {
		cfg.Review.PublishAction = action
		for _, d := range Validate(cfg).Diagnostics {
			if d.Code == DiagnosticCodeReviewPublishActionUnknown {
				t.Fatalf("publish_action %q warned: %+v", action, d)
			}
		}
	}
}

func TestParseReviewAutoVerifyQuality(t *testing.T) {
	if DefaultConfig().Review.AutoVerifyQuality {
		t.Fatal("auto_verify_quality should default to false")
	}
	cfg, err := Parse([]byte("[review]\nauto_verify_quality = true\nexternal_stages = [\"adversarial-review\"]\n"))
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.Review.AutoVerifyQuality {
		t.Fatal("auto_verify_quality not parsed")
	}
	if d := findDiagnostic(cfg, DiagnosticCodeReviewAutoVerifyQualityUngated); d != nil {
		t.Fatalf("gated config warned: %+v", *d)
	}
	ungated := DefaultConfig()
	ungated.Review.AutoVerifyQuality = true
	if d := findDiagnostic(ungated, DiagnosticCodeReviewAutoVerifyQualityUngated); d == nil || d.FieldPath != "review.auto_verify_quality" {
		t.Fatalf("ungated diagnostic = %+v", d)
	}
}

func TestParseWorktreeDiffAgainstLocalOnly(t *testing.T) {
	cfg, diags, err := parseWithDiagnostics([]byte(`
[worktree]
diff_against_local_only = true
`))
	if err != nil {
		t.Fatalf("Parse config: %v", err)
	}
	if !cfg.Worktree.DiffAgainstLocalOnly {
		t.Fatal("worktree.diff_against_local_only = false, want true")
	}
	for _, d := range diags {
		if strings.Contains(d.FieldPath, "diff_against_local_only") {
			t.Fatalf("unexpected diagnostic: %+v", d)
		}
	}

	cfg, err = Parse([]byte(`
[worktree]
link = ["brain"]
`))
	if err != nil {
		t.Fatalf("Parse config: %v", err)
	}
	if cfg.Worktree.DiffAgainstLocalOnly {
		t.Fatal("worktree.diff_against_local_only must default to false")
	}
}

func TestParseReviewSkipStagesWithoutChanges(t *testing.T) {
	cfg, diags, err := parseWithDiagnostics([]byte(`
[review]
skip_stages_without_changes = true
`))
	if err != nil {
		t.Fatalf("Parse config: %v", err)
	}
	if !cfg.Review.SkipStagesWithoutChanges {
		t.Fatal("review.skip_stages_without_changes = false, want true")
	}
	for _, d := range diags {
		if strings.Contains(d.FieldPath, "skip_stages_without_changes") {
			t.Fatalf("unexpected diagnostic: %+v", d)
		}
	}

	cfg, err = Parse([]byte(`
[review]
external_stages = ["quality"]
`))
	if err != nil {
		t.Fatalf("Parse config: %v", err)
	}
	if cfg.Review.SkipStagesWithoutChanges {
		t.Fatal("review.skip_stages_without_changes must default to false")
	}
}
