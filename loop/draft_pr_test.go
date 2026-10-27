package loop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/poteto/noodle/config"
)

type fakeDraftPR struct {
	requests []DraftPRRequest
	err      error
}

func (f *fakeDraftPR) Publish(req DraftPRRequest) (DraftPRResult, error) {
	f.requests = append(f.requests, req)
	if f.err != nil {
		return DraftPRResult{}, f.err
	}
	return DraftPRResult{Branch: "noodle/" + req.OrderID, URL: "https://github.com/acme/widgets/pull/7"}, nil
}

func TestLoopStateReportsDraftPR(t *testing.T) {
	l := newControlTestLoop(t, &fakeWorktree{}, newMockRuntime())
	if l.buildLoopStateSnapshot().DraftPR {
		t.Fatal("draft_pr should be false without a publisher")
	}
	l.deps.DraftPR = &fakeDraftPR{}
	if !l.buildLoopStateSnapshot().DraftPR {
		t.Fatal("draft_pr should be true with a publisher")
	}
}

func TestNewWiresDraftPRFromDeliveryConfig(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Delivery = config.DeliveryConfig{PushRemote: "origin", PRRepo: "acme/widgets", Base: "main"}
	l := New(t.TempDir(), "noodle", cfg, Dependencies{})
	if l.deps.DraftPR == nil {
		t.Fatal("New did not wire a draft PR publisher from [delivery]")
	}
	if New(t.TempDir(), "noodle", config.DefaultConfig(), Dependencies{}).deps.DraftPR != nil {
		t.Fatal("draft PR publisher wired without [delivery]")
	}
}

func runGitT(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", append([]string{"-C", dir}, args...)...)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return strings.TrimSpace(string(out))
}

func TestGitDraftPRPublisherPushesAndOpensDraftOnce(t *testing.T) {
	root := t.TempDir()
	project := filepath.Join(root, "project")
	remote := filepath.Join(root, "acme", "widgets.git")
	if err := os.MkdirAll(project, 0o755); err != nil {
		t.Fatal(err)
	}
	runGitT(t, root, "init", "-q", "--bare", remote)
	runGitT(t, project, "init", "-q", "-b", "main")
	runGitT(t, project, "config", "user.email", "t@example.com")
	runGitT(t, project, "config", "user.name", "t")
	runGitT(t, project, "commit", "-q", "--allow-empty", "-m", "init")
	runGitT(t, project, "remote", "add", "fork", "https://github.com/acme-fork/widgets.git")
	runGitT(t, project, "config", "remote.fork.pushurl", remote)
	runGitT(t, project, "branch", "42-0-execute")
	runGitT(t, project, "checkout", "-q", "42-0-execute")
	runGitT(t, project, "commit", "-q", "--allow-empty", "-m", "stage 0")
	runGitT(t, project, "checkout", "-q", "main")

	bin := filepath.Join(root, "bin")
	if err := os.MkdirAll(bin, 0o755); err != nil {
		t.Fatal(err)
	}
	ghLog := filepath.Join(root, "gh.log")
	script := "#!/bin/sh\nprintf '%s\\n' \"$*\" >> " + ghLog + "\necho https://github.com/acme/widgets/pull/7\n"
	if err := os.WriteFile(filepath.Join(bin, "gh"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin+string(os.PathListSeparator)+os.Getenv("PATH"))

	pub := newGitDraftPRPublisher(project, filepath.Join(project, ".noodle"), config.DeliveryConfig{
		PushRemote: "fork", PRRepo: "acme/widgets", Base: "main",
	})

	res, err := pub.Publish(DraftPRRequest{OrderID: "42", Branch: "42-0-execute", Title: "Fix the widget", Body: "why"})
	if err != nil {
		t.Fatalf("Publish: %v", err)
	}
	if res.Branch != "noodle/42" || res.URL != "https://github.com/acme/widgets/pull/7" {
		t.Fatalf("result = %+v", res)
	}
	if got := runGitT(t, root, "--git-dir", remote, "log", "-1", "--format=%s", "noodle/42"); got != "stage 0" {
		t.Fatalf("remote branch head = %q", got)
	}
	logged, _ := os.ReadFile(ghLog)
	want := "pr create --draft --repo acme/widgets --base main --head acme-fork:noodle/42 --title Fix the widget --body why"
	if strings.TrimSpace(string(logged)) != want {
		t.Fatalf("gh args:\n%s\nwant:\n%s", logged, want)
	}

	runGitT(t, project, "branch", "42-1-quality", "fork/noodle/42")
	runGitT(t, project, "checkout", "-q", "42-1-quality")
	runGitT(t, project, "commit", "-q", "--allow-empty", "-m", "stage 1")
	runGitT(t, project, "checkout", "-q", "main")
	again := newGitDraftPRPublisher(project, filepath.Join(project, ".noodle"), config.DeliveryConfig{
		PushRemote: "fork", PRRepo: "acme/widgets", Base: "main",
	})
	res, err = again.Publish(DraftPRRequest{OrderID: "42", Branch: "42-1-quality", Title: "Fix the widget"})
	if err != nil {
		t.Fatalf("second Publish: %v", err)
	}
	if res.URL != "https://github.com/acme/widgets/pull/7" {
		t.Fatalf("second result URL = %q", res.URL)
	}
	if got := runGitT(t, root, "--git-dir", remote, "log", "-1", "--format=%s", "noodle/42"); got != "stage 1" {
		t.Fatalf("remote branch head after stage 1 = %q", got)
	}
	logged, _ = os.ReadFile(ghLog)
	if n := strings.Count(string(logged), "pr create"); n != 1 {
		t.Fatalf("gh pr create ran %d times, want 1", n)
	}
}

func TestOtherControlAcksHaveNoMessage(t *testing.T) {
	l := newControlTestLoop(t, &fakeWorktree{}, newMockRuntime())
	l.applyControlCommand(ControlCommand{ID: "c1", Action: "approve", OrderID: "42"})
	ack := l.applyControlCommand(ControlCommand{ID: "c2", Action: "pause"})
	if ack.Message != "" {
		t.Fatalf("pause ack message = %q, want empty", ack.Message)
	}
}
