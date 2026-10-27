package snapshot

import (
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/poteto/noodle/internal/projection"
	"github.com/poteto/noodle/loop"
)

func TestSnapshotPendingReviewCarriesBlockedActions(t *testing.T) {
	state := loop.LoopState{
		Projection: projection.SnapshotView{PendingReviews: []projection.PendingReviewProjection{
			{OrderID: "o-1", TaskKey: "execute"},
			{OrderID: "o-2", TaskKey: "reflect"},
		}},
		ReviewBlocks: map[string]loop.ReviewActionBlock{
			"o-1": {Actions: []string{"approve", "merge"}, Reason: "order o-1 is held at stage quality for external review; complete it with noodle order o-1 complete quality (action stage-complete)"},
		},
	}
	snap, err := LoadSnapshot(t.TempDir(), time.Now(), state)
	if err != nil {
		t.Fatalf("load snapshot: %v", err)
	}
	data, err := json.Marshal(snap)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	var parsed struct {
		PendingReviews []struct {
			OrderID        string   `json:"order_id"`
			BlockedActions []string `json:"blocked_actions"`
			BlockedReason  string   `json:"blocked_reason"`
		} `json:"pending_reviews"`
	}
	if err := json.Unmarshal(data, &parsed); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(parsed.PendingReviews) != 2 {
		t.Fatalf("pending_reviews = %+v", parsed.PendingReviews)
	}
	blocked, free := parsed.PendingReviews[0], parsed.PendingReviews[1]
	if !reflect.DeepEqual(blocked.BlockedActions, []string{"approve", "merge"}) || blocked.BlockedReason != "order o-1 is held at stage quality for external review; complete it with noodle order o-1 complete quality (action stage-complete)" {
		t.Fatalf("o-1 = %+v", blocked)
	}
	if len(free.BlockedActions) != 0 || free.BlockedReason != "" {
		t.Fatalf("o-2 = %+v, want no block", free)
	}
}

func TestSnapshotPendingReviewCarriesPublishGated(t *testing.T) {
	for _, gated := range []bool{true, false} {
		state := loop.LoopState{
			Projection:   projection.SnapshotView{PendingReviews: []projection.PendingReviewProjection{{OrderID: "o-1", TaskKey: "execute"}}},
			PublishGated: gated,
		}
		snap, err := LoadSnapshot(t.TempDir(), time.Now(), state)
		if err != nil {
			t.Fatalf("load snapshot: %v", err)
		}
		if got := snap.PendingReviews[0].PublishGated; got != gated {
			t.Fatalf("publish_gated = %v, want %v", got, gated)
		}
	}
}
