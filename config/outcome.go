package config

import (
	"fmt"
	"slices"
	"sort"
	"strings"
)

// StageState is the verdict on a finished stage attempt.
type StageState string

const (
	StageStateAccept StageState = "accept"
	StageStateReject StageState = "reject"
)

// StageAction is what the loop does after a stage attempt gets a state.
type StageAction string

const (
	StageActionPromote       StageAction = "promote"
	StageActionExit          StageAction = "exit"
	StageActionPark          StageAction = "park"
	StageActionRetry         StageAction = "retry"
	StageActionRetryPrevious StageAction = "retry-previous"
)

var stageActions = []StageAction{
	StageActionPromote, StageActionExit, StageActionPark, StageActionRetry, StageActionRetryPrevious,
}

// ParseStageAction reads an action name, ignoring case and surrounding space.
func ParseStageAction(name string) (StageAction, bool) {
	a := StageAction(strings.ToLower(strings.TrimSpace(name)))
	return a, slices.Contains(stageActions, a)
}

// StageOutcomeConfig is one [stages.<task>] table: the action taken for each
// state, and how the task's cooks run. An unset state keeps the loop's
// mode-driven behavior: park for review in supervised mode, promote in auto
// mode, fail the order on a failed cook. Harness (claude or codex) and Model
// replace the scheduler's choice on stages that have not started. Provider is
// codex's model_provider. SystemPrompt follows the skill in the cook's system
// prompt, so it can override the skill's instructions.
type StageOutcomeConfig struct {
	Accept       string `toml:"accept"`
	Reject       string `toml:"reject"`
	Harness      string `toml:"harness"`
	Provider     string `toml:"provider"`
	Model        string `toml:"model"`
	SystemPrompt string `toml:"system_prompt"`
}

// StageConfig returns the [stages.<task>] table for a task key.
func (c Config) StageConfig(taskKey string) StageOutcomeConfig {
	return c.Stages[strings.TrimSpace(taskKey)]
}

// StageAction returns the configured action for a task key and state, and
// false when none is set or the name is unknown.
func (c Config) StageAction(taskKey string, state StageState) (StageAction, bool) {
	s := c.StageConfig(taskKey)
	raw := s.Accept
	if state == StageStateReject {
		raw = s.Reject
	}
	return ParseStageAction(raw)
}

// HooksConfig lists skills the loop runs around work stages. Before and After
// run once per order, BeforeEach and AfterEach around every work-stage
// attempt. A nil After means the default, ["reflect"]; an empty one is off.
type HooksConfig struct {
	Before     []string  `toml:"before"`
	After      *[]string `toml:"after"`
	BeforeEach []string  `toml:"before_each"`
	AfterEach  []string  `toml:"after_each"`
}

// DefaultAfterHook is the order-level after hook used when [hooks] after is
// unset.
const DefaultAfterHook = "reflect"

// AfterOrder returns the order-level after hooks.
func (h HooksConfig) AfterOrder() []string {
	if h.After == nil {
		return []string{DefaultAfterHook}
	}
	return *h.After
}

// Includes reports whether name is one of the configured hooks.
func (h HooksConfig) Includes(name string) bool {
	name = strings.TrimSpace(name)
	if name == "" {
		return false
	}
	for _, list := range [][]string{h.Before, h.AfterOrder(), h.BeforeEach, h.AfterEach} {
		if slices.ContainsFunc(list, func(s string) bool { return strings.TrimSpace(s) == name }) {
			return true
		}
	}
	return false
}

const (
	DiagnosticCodeStageActionUnknown       = "stage_action_unknown"
	DiagnosticCodeStageHarnessUnknown      = "stage_harness_unknown"
	DiagnosticCodeStageProviderUnsupported = "stage_provider_unsupported"
)

func outcomeDiagnostics(config Config) []ConfigDiagnostic {
	tasks := make([]string, 0, len(config.Stages))
	for task := range config.Stages {
		tasks = append(tasks, task)
	}
	sort.Strings(tasks)
	valid := make([]string, len(stageActions))
	for i, a := range stageActions {
		valid[i] = string(a)
	}
	var out []ConfigDiagnostic
	for _, task := range tasks {
		s := config.Stages[task]
		for _, f := range []struct{ key, value string }{{"accept", s.Accept}, {"reject", s.Reject}} {
			if strings.TrimSpace(f.value) == "" {
				continue
			}
			if _, ok := ParseStageAction(f.value); ok {
				continue
			}
			path := "stages." + task + "." + f.key
			out = append(out, ConfigDiagnostic{
				FieldPath: path,
				Message:   fmt.Sprintf("unknown action %q (valid: %s)", f.value, strings.Join(valid, ", ")),
				Severity:  DiagnosticSeverityFatal,
				Fix:       "Set " + path + " to one of " + strings.Join(valid, ", ") + ", or remove it to park.",
				Code:      DiagnosticCodeStageActionUnknown,
			})
		}
		out = append(out, stageRoutingDiagnostics(task, s)...)
	}
	return out
}

func stageRoutingDiagnostics(task string, s StageOutcomeConfig) []ConfigDiagnostic {
	var out []ConfigDiagnostic
	harness := strings.ToLower(strings.TrimSpace(s.Harness))
	if harness != "" && harness != "claude" && harness != "codex" {
		path := "stages." + task + ".harness"
		out = append(out, ConfigDiagnostic{
			FieldPath: path,
			Message:   fmt.Sprintf("unknown harness %q (valid: claude, codex)", s.Harness),
			Severity:  DiagnosticSeverityFatal,
			Fix:       "Set " + path + " to claude or codex, or remove it to keep the scheduler's choice.",
			Code:      DiagnosticCodeStageHarnessUnknown,
		})
	}
	if strings.TrimSpace(s.Provider) != "" && harness != "codex" {
		path := "stages." + task + ".provider"
		out = append(out, ConfigDiagnostic{
			FieldPath: path,
			Message:   "provider is codex's model_provider and needs harness = \"codex\"",
			Severity:  DiagnosticSeverityFatal,
			Fix:       "Set stages." + task + ".harness = \"codex\", or remove " + path + "; the claude CLI takes its backend from the environment.",
			Code:      DiagnosticCodeStageProviderUnsupported,
		})
	}
	return out
}
