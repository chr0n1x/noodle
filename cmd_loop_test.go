package main

import (
	"strings"
	"testing"
)

func TestLoopControlCommandsQueueTheirAction(t *testing.T) {
	for _, action := range []string{"pause", "resume", "drain"} {
		t.Run(action, func(t *testing.T) {
			app, runtimeDir := writeReviewFixture(t)
			root := newLoopControlCmd(app, action)
			if root.Name() != action {
				t.Fatalf("name = %q, want %q", root.Name(), action)
			}
			root.SetArgs([]string{"--no-wait"})
			out := captureStdout(t, func() {
				if err := root.Execute(); err != nil {
					t.Fatalf("execute: %v", err)
				}
			})
			if !strings.Contains(out, "queued") {
				t.Fatalf("output = %q, want queued notice", out)
			}
			cmd := readControlCommand(t, runtimeDir)
			if cmd.ID == "" || cmd.Action != action || cmd.At.IsZero() {
				t.Fatalf("command = %+v", cmd)
			}
		})
	}
}

func TestLoopControlCommandRejectsArgs(t *testing.T) {
	root := newLoopControlCmd(&App{projectDir: t.TempDir()}, "pause")
	root.SetArgs([]string{"extra"})
	root.SilenceErrors, root.SilenceUsage = true, true
	if err := root.Execute(); err == nil {
		t.Fatal("expected an error for an extra argument")
	}
}

func TestRootHasLoopControlCommands(t *testing.T) {
	root := NewRootCmd()
	for _, name := range []string{"pause", "resume", "drain"} {
		if c, _, err := root.Find([]string{name}); err != nil || c.Name() != name {
			t.Fatalf("root is missing %q: %v", name, err)
		}
	}
}
