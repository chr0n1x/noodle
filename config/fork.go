package config

import (
	"fmt"
	"strings"

	"github.com/poteto/noodle/skill"
)

// DeliveryConfig enables the "Approve & Draft PR" review action: approved
// stages are pushed to PushRemote and collected in one draft PR per order
// against Base on PRRepo.
type DeliveryConfig struct {
	PushRemote string `toml:"push_remote"`
	PRRepo     string `toml:"pr_repo"`
	Base       string `toml:"base"`
	Branch     string `toml:"branch"`
}

func (d DeliveryConfig) Enabled() bool {
	return strings.TrimSpace(d.PushRemote) != "" && strings.TrimSpace(d.PRRepo) != "" && strings.TrimSpace(d.Base) != ""
}

// BranchTemplate returns the PR branch name template; "{order_id}" is
// replaced with the order ID.
func (d DeliveryConfig) BranchTemplate() string {
	if b := strings.TrimSpace(d.Branch); b != "" {
		return b
	}
	return "noodle/{order_id}"
}

// WorktreeConfig lists root paths symlinked into each new cook worktree.
// DiffAgainstLocalOnly makes review diffs fork from the local default branch
// instead of origin/<default>, for checkouts whose origin lags behind.
type WorktreeConfig struct {
	Link                 []string `toml:"link"`
	DiffAgainstLocalOnly bool     `toml:"diff_against_local_only"`
}

// ReviewConfig lists stages that are held for an external reviewer instead
// of being dispatched to an agent session.
type ReviewConfig struct {
	ExternalStages []string `toml:"external_stages"`
	// LegacyOnAccept, LegacyOnQualityAccept and LegacyOnAdversarialReviewAccept
	// hold retired publish-on-accept keys so they can be reported as ignored.
	LegacyOnAccept                  string `toml:"on_accept"`
	LegacyOnQualityAccept           string `toml:"on_quality_accept"`
	LegacyOnAdversarialReviewAccept string `toml:"on_adversarial_review_accept"`
	// MaxRework bounds how many times a rejected external review sends its
	// feedback back to an execute stage for one order. Unset means 2.
	MaxRework *int `toml:"max_rework"`
	// ExtraStages are skills added after each pending quality stage when
	// scheduled orders are promoted, or at the end of an unstarted order
	// with no quality stage, e.g. ["adversarial-review", "reflect"].
	ExtraStages []string `toml:"extra_stages"`
	// PublishAction is how approve-and-publish delivers an order's kept
	// work: empty merges it locally, "draft-pr" opens a draft PR from
	// [delivery], any other value names a skill run as a final stage.
	PublishAction string `toml:"publish_action"`
	// AutoVerifyQuality makes the loop insert a quality stage, run as its
	// own cook, after each execute stage that completes, in gated configs.
	// A rejecting quality stage then queues execute rework.
	AutoVerifyQuality bool `toml:"auto_verify_quality"`
	// SkipStagesWithoutChanges makes approving a stage of an order with no
	// commits and a clean worktree skip the order's remaining stages and
	// complete it with nothing to publish.
	SkipStagesWithoutChanges bool `toml:"skip_stages_without_changes"`
}

// Gated reports whether orders keep their work until approve-and-publish.
func (r ReviewConfig) Gated() bool {
	return len(r.ExternalStages) > 0 || strings.TrimSpace(r.PublishAction) != ""
}

// MaxReworkRounds returns max_rework, 2 when unset and 0 when negative.
func (r ReviewConfig) MaxReworkRounds() int {
	if r.MaxRework == nil {
		return 2
	}
	return max(*r.MaxRework, 0)
}

const (
	DiagnosticCodeReviewOnAcceptIgnored                  = "review_on_accept_ignored"
	DiagnosticCodeReviewOnQualityAcceptIgnored           = "review_on_quality_accept_ignored"
	DiagnosticCodeReviewOnAdversarialReviewAcceptIgnored = "review_on_adversarial_review_accept_ignored"
	DiagnosticCodeReviewMaxReworkInvalid                 = "review_max_rework_invalid"
	DiagnosticCodeReviewPublishActionUnknown             = "review_publish_action_unknown"
	DiagnosticCodeReviewAutoVerifyQualityUngated         = "review_auto_verify_quality_ungated"
)

// forkDiagnostics validates config fields added by this fork.
func forkDiagnostics(config Config) []ConfigDiagnostic {
	var out []ConfigDiagnostic
	for _, c := range []struct{ key, value, code string }{
		{"on_accept", config.Review.LegacyOnAccept, DiagnosticCodeReviewOnAcceptIgnored},
		{"on_quality_accept", config.Review.LegacyOnQualityAccept, DiagnosticCodeReviewOnQualityAcceptIgnored},
		{"on_adversarial_review_accept", config.Review.LegacyOnAdversarialReviewAccept, DiagnosticCodeReviewOnAdversarialReviewAcceptIgnored},
	} {
		if strings.TrimSpace(c.value) != "" {
			out = append(out, ConfigDiagnostic{
				FieldPath: "review." + c.key,
				Message:   c.key + " is ignored; accepting a stage never publishes",
				Severity:  DiagnosticSeverityWarning,
				Fix:       "Remove review." + c.key + " from .noodle.toml.",
				Code:      c.code,
			})
		}
	}
	if r := config.Review.MaxRework; r != nil && *r < 0 {
		out = append(out, ConfigDiagnostic{
			FieldPath: "review.max_rework",
			Message:   fmt.Sprintf("max_rework %d is negative; using 0 (no rework)", *r),
			Severity:  DiagnosticSeverityWarning,
			Fix:       "Set review.max_rework to 0 or a positive number in .noodle.toml.",
			Code:      DiagnosticCodeReviewMaxReworkInvalid,
		})
	}
	if name := strings.TrimSpace(config.Review.PublishAction); name != "" && name != PublishActionDraftPR {
		if _, err := (skill.Resolver{SearchPaths: config.Skills.Paths}).Resolve(name); err != nil {
			out = append(out, ConfigDiagnostic{
				FieldPath: "review.publish_action",
				Message:   fmt.Sprintf("publish_action skill %s was not found in skills.paths; approve-and-publish will refuse it", name),
				Severity:  DiagnosticSeverityWarning,
				Fix:       "Add the skill, set publish_action to \"draft-pr\", or remove it to merge locally.",
				Code:      DiagnosticCodeReviewPublishActionUnknown,
			})
		}
	}
	if config.Review.AutoVerifyQuality && !config.Review.Gated() {
		out = append(out, ConfigDiagnostic{
			FieldPath: "review.auto_verify_quality",
			Message:   "auto_verify_quality only applies when external_stages or publish_action is set; it is ignored",
			Severity:  DiagnosticSeverityWarning,
			Fix:       "Set review.external_stages or review.publish_action, or remove auto_verify_quality.",
			Code:      DiagnosticCodeReviewAutoVerifyQualityUngated,
		})
	}
	return append(out, outcomeDiagnostics(config)...)
}

// PublishActionDraftPR is the publish_action value that opens a draft PR.
const PublishActionDraftPR = "draft-pr"

// IsExternal reports whether a stage, matched by task key or skill name, is
// reviewed externally.
func (r ReviewConfig) IsExternal(taskKey, skill string) bool {
	taskKey, skill = strings.TrimSpace(taskKey), strings.TrimSpace(skill)
	for _, name := range r.ExternalStages {
		name = strings.TrimSpace(name)
		if name != "" && (name == taskKey || name == skill) {
			return true
		}
	}
	return false
}
