package dispatcher

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestProcessSessionParsesWithItsProvider(t *testing.T) {
	cmd := exec.Command("printf", `{"type":"result","subtype":"success"}\n`)
	process, err := StartProcess(cmd)
	if err != nil {
		t.Fatalf("StartProcess: %v", err)
	}
	dir := t.TempDir()
	canonicalPath := filepath.Join(dir, "canonical.ndjson")
	session := newProcessSession(processSessionConfig{
		id:            "session-p",
		provider:      "gemini",
		process:       process,
		canonicalPath: canonicalPath,
		stampedPath:   filepath.Join(dir, "raw.ndjson"),
	})
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	session.start(ctx)
	select {
	case <-session.Done():
	case <-time.After(5 * time.Second):
		t.Fatal("session did not signal done")
	}
	data, err := os.ReadFile(canonicalPath)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `provider \"gemini\"`) {
		t.Fatalf("line was not routed to the session provider:\n%s", data)
	}
}
