package loop

import (
	"bufio"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func reviewDecisions(t *testing.T, l *Loop) []map[string]any {
	t.Helper()
	file, err := os.Open(filepath.Join(l.runtimeDir, "loop-events.ndjson"))
	if err != nil {
		t.Fatalf("open loop events: %v", err)
	}
	defer file.Close()
	var out []map[string]any
	scanner := bufio.NewScanner(file)
	for scanner.Scan() {
		var raw struct {
			Type    string         `json:"type"`
			Payload map[string]any `json:"payload"`
		}
		if json.Unmarshal(scanner.Bytes(), &raw) == nil && raw.Type == "review.decided" {
			out = append(out, raw.Payload)
		}
	}
	return out
}

func lastDecision(t *testing.T, l *Loop) map[string]any {
	t.Helper()
	d := reviewDecisions(t, l)
	if len(d) == 0 {
		t.Fatal("no review.decided event")
	}
	return d[len(d)-1]
}

func TestAcceptEmitsReviewDecision(t *testing.T) {
	l, _, _ := newHeldReviewForPublish(t)
	if err := acceptHeld(l); err != nil {
		t.Fatalf("accept: %v", err)
	}
	d := lastDecision(t, l)
	if d["verdict"] != "accepted" || d["order_id"] != "42" || d["task_key"] != "review" || d["summary"] != "looks good" {
		t.Fatalf("decision = %v", d)
	}
	if d["review_file"] != filepath.Join(l.projectDir, "brain", "reviews", "42-review.md") {
		t.Fatalf("review_file = %v", d["review_file"])
	}
}

func TestRejectEmitsReviewDecision(t *testing.T) {
	l, _, _ := newHeldReviewForPublish(t)
	if err := l.dispatchControlCommand(stageComplete("review", "reject", "missing tests\nsecond")); err != nil {
		t.Fatalf("reject: %v", err)
	}
	d := lastDecision(t, l)
	if d["verdict"] != "rejected" || d["summary"] != "missing tests\nsecond" {
		t.Fatalf("decision = %v", d)
	}
}

func TestApproveEmitsDecisionWithKeptBranch(t *testing.T) {
	l, _ := newApproveTestLoop(t)
	if err := l.controlApprove("42"); err != nil {
		t.Fatalf("approve: %v", err)
	}
	d := lastDecision(t, l)
	if d["verdict"] != "approved" || !strings.Contains(d["outcome"].(string), "42-0-execute") {
		t.Fatalf("decision = %v", d)
	}
}

func TestReviewSummaryKeepsFirstThreeLines(t *testing.T) {
	if got := reviewSummary("\n# Title\n\none\ntwo\nthree\nfour\n"); got != "# Title\none\ntwo" {
		t.Fatalf("summary = %q", got)
	}
}
