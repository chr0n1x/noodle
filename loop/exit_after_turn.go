package loop

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// holdsNoChangeExit reports whether a clean exit without commits must wait
// for review. With exit_after_turn on the stage's provider, an agent that
// ends its turn on a question exits like a finished one; a stage_yield or a non-blocking
// stage_message (the quality verdict) marks it as done.
func (l *Loop) holdsNoChangeExit(cook *cookHandle) bool {
	provider := strings.ToLower(strings.TrimSpace(cook.stage.Provider))
	if provider == "" {
		provider = "claude"
	}
	if !l.config.Agents.For(provider).ExitAfterTurn {
		return false
	}
	if m := l.readStageMessage(cook.session.ID()); m != nil && !m.IsBlocking() {
		return false
	}
	return l.readStageYield(cook.session.ID()) == nil
}

func noChangeExitReason(lastText string) string {
	const prefix = "agent ended its turn without changes"
	lastText = strings.TrimSpace(lastText)
	if lastText == "" {
		return prefix
	}
	const maxLen = 2000
	if len(lastText) > maxLen {
		lastText = lastText[:maxLen] + "..."
	}
	return prefix + ": " + lastText
}

// lastAgentText returns the agent's last text message from the session's
// canonical log, or "" when there is none.
func (l *Loop) lastAgentText(sessionID string) string {
	data, err := os.ReadFile(filepath.Join(l.runtimeDir, "sessions", sessionID, "canonical.ndjson"))
	if err != nil {
		return ""
	}
	last := ""
	for _, line := range strings.Split(string(data), "\n") {
		var ev struct {
			Type    string `json:"type"`
			Message string `json:"message"`
		}
		if json.Unmarshal([]byte(line), &ev) != nil || ev.Type != "action" {
			continue
		}
		if text, ok := strings.CutPrefix(ev.Message, "text:"); ok {
			last = text
		}
	}
	return last
}
