package loop

import (
	"strings"

	"github.com/poteto/noodle/config"
)

// routeStage applies the [stages.<task>] harness and model to a stage that
// has not started.
func routeStage(s Stage, stages map[string]config.StageOutcomeConfig) Stage {
	if s.Status != StageStatusPending || isScheduleStage(s) {
		return s
	}
	c := stages[strings.TrimSpace(s.TaskKey)]
	if h := strings.ToLower(strings.TrimSpace(c.Harness)); h != "" {
		s.Provider = h
	}
	if m := strings.TrimSpace(c.Model); m != "" {
		s.Model = m
	}
	return s
}

// applyStageRouting routes every pending stage of every order.
func applyStageRouting(orders OrdersFile, stages map[string]config.StageOutcomeConfig) OrdersFile {
	if len(stages) == 0 {
		return orders
	}
	for i := range orders.Orders {
		for j := range orders.Orders[i].Stages {
			orders.Orders[i].Stages[j] = routeStage(orders.Orders[i].Stages[j], stages)
		}
	}
	return orders
}
