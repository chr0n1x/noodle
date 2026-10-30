package loop

import "strings"

// applyExtraStages inserts each [review] extra_stages name as a stage right
// after an order's last quality stage, when that quality stage is still pending
// and the order lacks a stage with that name. An order with no quality stage
// gets them at its end instead, while none of its stages has started; the
// schedule and schedule-bootstrap orders never get them. The added stage reuses
// the provider, model and runtime of the stage it follows. Names for which
// known returns false are skipped and returned, since order normalization
// drops stages without a registered skill.
func applyExtraStages(orders OrdersFile, names []string, known func(string) bool) (OrdersFile, []string) {
	var unknown []string
	var usable []string
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if known(name) {
			usable = append(usable, name)
		} else {
			unknown = append(unknown, name)
		}
	}
	out := orders
	out.Orders = make([]Order, len(orders.Orders))
	for i, order := range orders.Orders {
		out.Orders[i] = order
		q := -1
		for j, s := range order.Stages {
			if s.TaskKey == "quality" {
				q = j
			}
		}
		if q < 0 && appendsExtraStages(order) {
			q = len(order.Stages) - 1
		} else if q < 0 || order.Stages[q].Status != StageStatusPending {
			continue
		}
		after := order.Stages[q]
		var added []Stage
		for _, name := range usable {
			if orderHasStage(order, name) || stagesHave(added, name) {
				continue
			}
			added = append(added, Stage{TaskKey: name, Skill: name, Provider: after.Provider, Model: after.Model, Runtime: after.Runtime, Status: StageStatusPending})
		}
		if len(added) == 0 {
			continue
		}
		stages := make([]Stage, 0, len(order.Stages)+len(added))
		stages = append(stages, order.Stages[:q+1]...)
		stages = append(stages, added...)
		stages = append(stages, order.Stages[q+1:]...)
		out.Orders[i].Stages = stages
	}
	return out, unknown
}

// appendsExtraStages reports whether an order without a quality stage gets the
// extra stages at its end: it is a work order with stages, none of them started.
func appendsExtraStages(order Order) bool {
	if len(order.Stages) == 0 || isScheduleOrder(order) || isScheduleBootstrapOrder(order) {
		return false
	}
	for _, s := range order.Stages {
		if s.Status != StageStatusPending {
			return false
		}
	}
	return true
}

func orderHasStage(order Order, name string) bool {
	return stagesHave(order.Stages, name)
}

func stagesHave(stages []Stage, name string) bool {
	for _, s := range stages {
		if s.TaskKey == name || s.Skill == name {
			return true
		}
	}
	return false
}
