package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poteto/noodle/loop"
)

func writeReviewFixture(t *testing.T) (*App, string) {
	t.Helper()
	projectDir := t.TempDir()
	runtimeDir := filepath.Join(projectDir, ".noodle")
	if err := os.MkdirAll(filepath.Join(runtimeDir, "reviews"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	pending := `{"items": [
  {"order_id": "5", "stage_index": 1, "task_key": "quality", "title": "Fix it", "worktree_name": "5-1-quality", "worktree_path": "/wt/5-1-quality", "reason": "external review: run /noodle-review"},
  {"order_id": "6", "stage_index": 0, "task_key": "execute", "title": "Other", "worktree_name": "6-0-execute", "worktree_path": "/wt/6", "reason": "merge requires review"}
]}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "pending-review.json"), []byte(pending), 0o644); err != nil {
		t.Fatalf("write pending: %v", err)
	}
	if err := os.WriteFile(filepath.Join(runtimeDir, "reviews", "5-1.prompt.md"), []byte("review prompt body"), 0o644); err != nil {
		t.Fatalf("write prompt: %v", err)
	}
	return &App{projectDir: projectDir}, runtimeDir
}

func TestReviewListJSONFiltersExternalReviews(t *testing.T) {
	app, runtimeDir := writeReviewFixture(t)
	out := captureStdout(t, func() {
		if err := runReviewList(app, true); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	var got struct {
		Items []map[string]any `json:"items"`
	}
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("parse %q: %v", out, err)
	}
	if len(got.Items) != 1 {
		t.Fatalf("items = %v, want only the external review", got.Items)
	}
	item := got.Items[0]
	want := map[string]any{
		"order_id":      "5",
		"stage_index":   float64(1),
		"task_key":      "quality",
		"title":         "Fix it",
		"worktree_path": "/wt/5-1-quality",
		"prompt_file":   filepath.Join(runtimeDir, "reviews", "5-1.prompt.md"),
	}
	if len(item) != len(want) {
		t.Fatalf("item keys = %v", item)
	}
	for k, v := range want {
		if item[k] != v {
			t.Fatalf("%s = %v, want %v", k, item[k], v)
		}
	}
}

func TestReviewListJSONEmpty(t *testing.T) {
	app := &App{projectDir: t.TempDir()}
	out := captureStdout(t, func() {
		if err := runReviewList(app, true); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	if strings.TrimSpace(out) != `{"items": []}` {
		t.Fatalf("output = %q", out)
	}
}

func TestReviewListTable(t *testing.T) {
	app, _ := writeReviewFixture(t)
	out := captureStdout(t, func() {
		if err := runReviewList(app, false); err != nil {
			t.Fatalf("list: %v", err)
		}
	})
	if !strings.Contains(out, "5") || !strings.Contains(out, "quality") || strings.Contains(out, "Other") {
		t.Fatalf("table = %q", out)
	}
}

func TestReviewPromptPrintsFile(t *testing.T) {
	app, _ := writeReviewFixture(t)
	out := captureStdout(t, func() {
		if err := runReviewPrompt(app, "5"); err != nil {
			t.Fatalf("prompt: %v", err)
		}
	})
	if !strings.Contains(out, "review prompt body") {
		t.Fatalf("output = %q", out)
	}
	if err := runReviewPrompt(app, "6"); err == nil {
		t.Fatal("expected error for order without external review prompt")
	}
}

func TestReviewShowPrintsVerdict(t *testing.T) {
	app, runtimeDir := writeReviewFixture(t)
	verdict := `{"order_id":"5","stage_index":1,"task_key":"quality","verdict":"rejected","review_file":"/p/brain/reviews/5-quality.md"}`
	if err := os.WriteFile(filepath.Join(runtimeDir, "reviews", "5.verdict.json"), []byte(verdict), 0o644); err != nil {
		t.Fatal(err)
	}
	out := captureStdout(t, func() {
		if err := runReviewShow(app, "5", true); err != nil {
			t.Fatalf("show: %v", err)
		}
	})
	if !strings.Contains(out, `"verdict": "rejected"`) || !strings.Contains(out, `"review_file": "/p/brain/reviews/5-quality.md"`) {
		t.Fatalf("output = %q", out)
	}
	text := captureStdout(t, func() {
		if err := runReviewShow(app, "5", false); err != nil {
			t.Fatalf("show: %v", err)
		}
	})
	if !strings.Contains(text, "rejected") {
		t.Fatalf("text output = %q", text)
	}
	if err := runReviewShow(app, "6", true); err == nil {
		t.Fatal("expected error for order without a verdict")
	}
}

func writeReviewFile(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "review.md")
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatalf("write review: %v", err)
	}
	return path
}

func readControlCommand(t *testing.T, runtimeDir string) loop.ControlCommand {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(runtimeDir, "control.ndjson"))
	if err != nil {
		t.Fatalf("read control: %v", err)
	}
	var cmd loop.ControlCommand
	if err := json.Unmarshal(data, &cmd); err != nil {
		t.Fatalf("parse %q: %v", data, err)
	}
	return cmd
}

func TestOrderCompleteWritesStageCompleteCommand(t *testing.T) {
	app, runtimeDir := writeReviewFixture(t)
	reviewFile := writeReviewFile(t, "needs tests")
	root := newOrderCmd(app)
	root.SetArgs([]string{"5", "complete", "quality", "--file", reviewFile, "--reject", "--no-wait"})
	out := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(out, "queued") {
		t.Fatalf("output = %q, want queued notice", out)
	}
	cmd := readControlCommand(t, runtimeDir)
	if cmd.ID == "" || cmd.Action != "stage-complete" || cmd.OrderID != "5" || cmd.TaskKey != "quality" ||
		cmd.Prompt != "needs tests" || cmd.Value != "reject" || cmd.At.IsZero() {
		t.Fatalf("command = %+v", cmd)
	}
}

func TestOrderCompleteAcceptsByDefault(t *testing.T) {
	app, runtimeDir := writeReviewFixture(t)
	root := newOrderCmd(app)
	root.SetArgs([]string{"5", "complete", "adversarial-review", "--file", writeReviewFile(t, "ok"), "--no-wait"})
	captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if cmd := readControlCommand(t, runtimeDir); cmd.Value != "accept" || cmd.TaskKey != "adversarial-review" {
		t.Fatalf("command = %+v", cmd)
	}
}

func TestOrderCompleteHasNoPublishFlags(t *testing.T) {
	sub := newOrderCmd(&App{projectDir: t.TempDir()})
	for _, f := range []string{"draft-pr", "merge", "publish"} {
		if sub.Flags().Lookup(f) != nil {
			t.Fatalf("order complete has --%s", f)
		}
	}
	for _, f := range []string{"file", "reject", "no-wait"} {
		if sub.Flags().Lookup(f) == nil {
			t.Fatalf("order complete missing --%s", f)
		}
	}
}

func TestReviewHasNoCompleteSubcommands(t *testing.T) {
	root := newReviewCmd(&App{projectDir: t.TempDir()})
	for _, c := range root.Commands() {
		switch c.Name() {
		case "list", "show", "prompt", "help", "completion":
		default:
			t.Fatalf("unexpected review subcommand %q", c.Name())
		}
	}
}

func TestOrderRequiresCompleteVerb(t *testing.T) {
	app, _ := writeReviewFixture(t)
	root := newOrderCmd(app)
	root.SetArgs([]string{"5", "finish", "quality", "--file", writeReviewFile(t, "ok"), "--no-wait"})
	root.SilenceUsage, root.SilenceErrors = true, true
	if err := root.Execute(); err == nil {
		t.Fatal("want an error for an unknown order verb")
	}
}

func TestOrderCompleteErrorAckFails(t *testing.T) {
	app, runtimeDir := writeReviewFixture(t)
	reviewFile := writeReviewFile(t, "ok")
	ackPath := filepath.Join(runtimeDir, "control-ack.ndjson")
	controlAckHook = func(id string) {
		line, _ := json.Marshal(loop.ControlAck{ID: id, Action: "stage-complete", Status: "error", Message: "boom"})
		_ = os.WriteFile(ackPath, append(line, '\n'), 0o644)
	}
	t.Cleanup(func() { controlAckHook = nil })
	err := runStageComplete(app, "5", "quality", reviewFile, false, true)
	if err == nil || !strings.Contains(err.Error(), "stage-complete failed: boom") {
		t.Fatalf("err = %v", err)
	}
}

func TestOrderResolveWritesResolveCommand(t *testing.T) {
	app, runtimeDir := writeReviewFixture(t)
	root := newOrderCmd(app)
	root.SetArgs([]string{"14", "resolve", "adversarial-review", "reject", "retry-previous", "--feedback-file", writeReviewFile(t, "you hallucinated"), "--no-wait"})
	out := captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if !strings.Contains(out, "queued") {
		t.Fatalf("output = %q, want queued notice", out)
	}
	cmd := readControlCommand(t, runtimeDir)
	if cmd.Action != "resolve" || cmd.OrderID != "14" || cmd.TaskKey != "adversarial-review" || cmd.State != "reject" ||
		cmd.Do != "retry-previous" || cmd.Prompt != "you hallucinated" || cmd.At.IsZero() {
		t.Fatalf("command = %+v", cmd)
	}
}

func TestOrderResolveFeedbackIsOptional(t *testing.T) {
	app, runtimeDir := writeReviewFixture(t)
	root := newOrderCmd(app)
	root.SetArgs([]string{"14", "resolve", "execute", "accept", "promote", "--no-wait"})
	captureStdout(t, func() {
		if err := root.Execute(); err != nil {
			t.Fatalf("execute: %v", err)
		}
	})
	if cmd := readControlCommand(t, runtimeDir); cmd.State != "accept" || cmd.Do != "promote" || cmd.Prompt != "" {
		t.Fatalf("command = %+v", cmd)
	}
}

func TestOrderResolveRejectsBadArgs(t *testing.T) {
	for _, args := range [][]string{
		{"14", "resolve", "execute", "maybe", "promote"},
		{"14", "resolve", "execute", "accept", "retry-last"},
		{"14", "resolve", "execute", "accept"},
	} {
		app, _ := writeReviewFixture(t)
		root := newOrderCmd(app)
		root.SetArgs(append(args, "--no-wait"))
		root.SilenceUsage, root.SilenceErrors = true, true
		if err := root.Execute(); err == nil {
			t.Fatalf("%v: want an error", args)
		}
	}
}

func TestOrderCompleteStillRequiresFile(t *testing.T) {
	app, _ := writeReviewFixture(t)
	root := newOrderCmd(app)
	root.SetArgs([]string{"5", "complete", "quality", "--no-wait"})
	root.SilenceUsage, root.SilenceErrors = true, true
	if err := root.Execute(); err == nil {
		t.Fatal("complete without --file should fail")
	}
}
