package loop

import (
	"context"
	"encoding/json"
	"strings"

	"github.com/poteto/noodle/config"
)

// hookExtraKey marks a stage the loop inserted for [hooks]; its value is the
// hook point: "before", "after", "before_each" or "after_each".
const hookExtraKey = "hook"

const (
	hookBefore     = "before"
	hookAfter      = "after"
	hookBeforeEach = "before_each"
	hookAfterEach  = "after_each"
)

func hookExtra(point string) map[string]json.RawMessage {
	raw, _ := json.Marshal(point)
	return map[string]json.RawMessage{hookExtraKey: raw}
}

func hookPoint(s Stage) string {
	var point string
	_ = json.Unmarshal(s.Extra[hookExtraKey], &point)
	return point
}

func isHookStage(s Stage) bool {
	_, ok := s.Extra[hookExtraKey]
	return ok
}

// applyHooks wraps each unstarted work order in its [hooks] stages: before
// hooks first, before_each and after_each around every stage, after hooks
// last. Each list runs in its own order, and a name repeated in a list keeps
// only its first place. An order-level hook is left out when the order
// already has a stage with that name. Orders with a started stage or an
// existing hook stage, and the schedule orders, are left as they are. Each hook reuses the provider,
// model and runtime of the stage next to it. Names for which known returns
// false are skipped and returned.
func applyHooks(orders OrdersFile, hooks config.HooksConfig, known func(string) bool) (OrdersFile, []string) {
	var unknown []string
	usable := func(names []string) []string {
		var out []string
		for _, name := range names {
			name = strings.TrimSpace(name)
			if name == "" {
				continue
			}
			if !known(name) {
				if !stagesHaveName(unknown, name) {
					unknown = append(unknown, name)
				}
				continue
			}
			if !stagesHaveName(out, name) {
				out = append(out, name)
			}
		}
		return out
	}
	before, after := usable(hooks.Before), usable(hooks.AfterOrder())
	beforeEach, afterEach := usable(hooks.BeforeEach), usable(hooks.AfterEach)

	out := orders
	out.Orders = make([]Order, len(orders.Orders))
	for i, order := range orders.Orders {
		out.Orders[i] = order
		if !appendsExtraStages(order) || orderHasHook(order) {
			continue
		}
		wrap := func(names []string, point string, next Stage, skipExisting bool) []Stage {
			var added []Stage
			for _, name := range names {
				if skipExisting && orderHasStage(order, name) {
					continue
				}
				added = append(added, Stage{
					TaskKey: name, Skill: name, Provider: next.Provider, Model: next.Model, Runtime: next.Runtime,
					Status: StageStatusPending, Extra: hookExtra(point),
				})
			}
			return added
		}
		first, last := order.Stages[0], order.Stages[len(order.Stages)-1]
		stages := wrap(before, hookBefore, first, true)
		for _, s := range order.Stages {
			stages = append(stages, wrap(beforeEach, hookBeforeEach, s, false)...)
			stages = append(stages, s)
			stages = append(stages, wrap(afterEach, hookAfterEach, s, false)...)
		}
		stages = append(stages, wrap(after, hookAfter, last, true)...)
		out.Orders[i].Stages = stages
	}
	return out, unknown
}

func stagesHaveName(names []string, name string) bool {
	for _, n := range names {
		if n == name {
			return true
		}
	}
	return false
}

func orderHasHook(order Order) bool {
	for _, s := range order.Stages {
		if isHookStage(s) {
			return true
		}
	}
	return false
}

// applyConfiguredHooks runs applyHooks with the loop's config and registry,
// warning about hooks with no registered skill. The default after hook is
// skipped quietly when no reflect skill is registered.
func (l *Loop) applyConfiguredHooks(orders OrdersFile) OrdersFile {
	out, unknown := applyHooks(orders, l.config.Hooks, func(name string) bool {
		_, ok := l.registry.ByKey(name)
		return ok
	})
	for _, name := range unknown {
		if name == config.DefaultAfterHook && l.config.Hooks.After == nil {
			continue
		}
		l.logger.Warn("hook skipped: no skill registered", "hook", name)
	}
	return out
}

// completeHook finishes a hook stage that ended cleanly. Hooks never park
// and record no outcome; the order goes on to its next stage.
func (l *Loop) completeHook(ctx context.Context, cook *cookHandle) error {
	if m := l.readStageMessage(cook.session.ID()); m != nil {
		l.forwardToScheduler(cook, "stage_message", m.Message, nil)
	}
	msg := hookPoint(cook.stage) + " hook " + cook.stage.TaskKey + " completed"
	return l.promoteStage(ctx, cook, msg, false)
}
