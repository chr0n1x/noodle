package snapshot

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/poteto/noodle/internal/projection"
	"github.com/poteto/noodle/loop"
)

type publishOrderJSON struct {
	ID                   string `json:"id"`
	Status               string `json:"status"`
	PublishGated         bool   `json:"publish_gated"`
	PublishBlockedReason string `json:"publish_blocked_reason"`
	PublishError         string `json:"publish_error"`
}

func loadPublishOrders(t *testing.T, runtimeDir string, state loop.LoopState) map[string]publishOrderJSON {
	t.Helper()
	snap, err := LoadSnapshot(runtimeDir, time.Now(), state)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed struct {
		Orders []publishOrderJSON `json:"orders"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	out := map[string]publishOrderJSON{}
	for _, o := range parsed.Orders {
		out[o.ID] = o
	}
	return out
}

func TestSnapshotOrderCarriesPublishState(t *testing.T) {
	runtimeDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(runtimeDir, "publish-errors.json"), []byte(`{"o-1": "draft-pr: push rejected"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	state := loop.LoopState{
		Projection: projection.SnapshotView{Orders: []projection.OrderProjection{
			{ID: "o-1", Status: "awaiting-publish"},
			{ID: "o-2", Status: "active"},
		}},
		PublishGated:  true,
		PublishBlocks: map[string]string{"o-2": "approve-and-publish refused: order o-2 is not awaiting publish (status active)"},
	}
	orders := loadPublishOrders(t, runtimeDir, state)
	if got := orders["o-1"]; got.Status != "awaiting-publish" || !got.PublishGated || got.PublishBlockedReason != "" || got.PublishError != "draft-pr: push rejected" {
		t.Fatalf("o-1 = %+v", got)
	}
	if got := orders["o-2"]; !got.PublishGated || got.PublishBlockedReason != state.PublishBlocks["o-2"] || got.PublishError != "" {
		t.Fatalf("o-2 = %+v", got)
	}
}

func TestSnapshotOrderOmitsPublishStateWhenUngated(t *testing.T) {
	state := loop.LoopState{
		Projection: projection.SnapshotView{Orders: []projection.OrderProjection{{ID: "o-1", Status: "active"}}},
	}
	snap, err := LoadSnapshot(t.TempDir(), time.Now(), state)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	data, _ := json.Marshal(snap.Orders[0])
	var raw map[string]any
	_ = json.Unmarshal(data, &raw)
	for _, key := range []string{"publish_gated", "publish_blocked_reason", "publish_error"} {
		if _, ok := raw[key]; ok {
			t.Fatalf("ungated order has %s: %s", key, data)
		}
	}
}
