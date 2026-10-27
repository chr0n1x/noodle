package projection

import (
	"testing"
	"time"

	"github.com/poteto/noodle/internal/mode"
	"github.com/poteto/noodle/internal/state"
	"github.com/poteto/noodle/internal/statever"
)

func TestOrderProjectionCarriesURLFromMetadata(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, 10, 27, 6, 0, 0, 0, time.UTC)
	st := state.State{
		Orders: map[string]state.OrderNode{
			"9": {
				OrderID:   "9",
				Title:     "PROJ-123 fix login bug",
				Status:    state.OrderActive,
				CreatedAt: now,
				UpdatedAt: now,
				Metadata:  map[string]string{"url": "https://jira.example.com/browse/PROJ-123"},
				Stages:    []state.StageNode{{StageIndex: 0, Skill: "execute", Status: state.StagePending}},
			},
			"10": {
				OrderID:   "10",
				Title:     "no link",
				Status:    state.OrderActive,
				CreatedAt: now,
				UpdatedAt: now,
				Stages:    []state.StageNode{{StageIndex: 0, Skill: "execute", Status: state.StagePending}},
			},
		},
		Mode:          state.RunModeAuto,
		SchemaVersion: statever.Current,
		LastEventID:   "1",
	}

	bundle := mustProject(t, st, mode.ModeState{EffectiveMode: state.RunModeAuto})

	urls := map[string]string{}
	for _, order := range bundle.OrdersProjection {
		urls[order.ID] = order.URL
	}
	if urls["9"] != "https://jira.example.com/browse/PROJ-123" {
		t.Fatalf("order 9 URL = %q", urls["9"])
	}
	if urls["10"] != "" {
		t.Fatalf("order 10 URL = %q, want empty", urls["10"])
	}
	legacyURLs := map[string]string{}
	for _, order := range legacyOrdersFileProjection(bundle).Orders {
		legacyURLs[order.ID] = order.URL
	}
	if legacyURLs["9"] != "https://jira.example.com/browse/PROJ-123" {
		t.Fatalf("orders.json order 9 URL = %q", legacyURLs["9"])
	}
}
