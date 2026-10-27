package snapshot

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestReadLoopEventsShowsReviewOutcomes(t *testing.T) {
	dir := t.TempDir()
	ndjson := `{"seq":1,"type":"review.decided","at":"2026-10-26T10:00:00Z","payload":{"order_id":"42","task_key":"quality","verdict":"accepted","outcome":"draft PR https://github.com/acme/widgets/pull/7","summary":"Looks good.\nTests pass.","review_file":"/p/brain/reviews/42-quality.md"}}
{"seq":2,"type":"review.decided","at":"2026-10-26T10:01:00Z","payload":{"order_id":"43","task_key":"quality","verdict":"rejected","outcome":"sent back","summary":"Missing tests.","review_file":"/p/brain/reviews/43-quality.md"}}
{"seq":3,"type":"order.work_kept","at":"2026-10-26T10:02:00Z","payload":{"order_id":"44","branches":["44-0-execute","44-1-quality"]}}
`
	if err := os.WriteFile(filepath.Join(dir, "loop-events.ndjson"), []byte(ndjson), 0o644); err != nil {
		t.Fatal(err)
	}

	events := readLoopEvents(dir)
	if len(events) != 3 {
		t.Fatalf("event count = %d, want 3", len(events))
	}
	accepted := events[0]
	if accepted.Label != "Review accepted" || accepted.TaskType != "quality" {
		t.Fatalf("accepted = %+v", accepted)
	}
	for _, want := range []string{"Order 42", "draft PR https://github.com/acme/widgets/pull/7", "Looks good.", "/p/brain/reviews/42-quality.md"} {
		if !strings.Contains(accepted.Body, want) {
			t.Fatalf("accepted body %q missing %q", accepted.Body, want)
		}
	}
	if events[1].Label != "Review rejected" || !strings.Contains(events[1].Body, "Missing tests.") {
		t.Fatalf("rejected = %+v", events[1])
	}
	kept := events[2]
	if kept.Label != "Work kept" || !strings.Contains(kept.Body, "44-0-execute, 44-1-quality") {
		t.Fatalf("work kept = %+v", kept)
	}
}
