package parse

import (
	"encoding/json"
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// CopilotAdapter parses `copilot --output-format json` output.
type CopilotAdapter struct{}

type copilotLine struct {
	Type     string          `json:"type"`
	Injected json.RawMessage `json:"_ts"`
	Data     copilotData     `json:"data"`
	ExitCode int             `json:"exitCode"`
}

type copilotData struct {
	Content      string               `json:"content"`
	ToolRequests []copilotToolRequest `json:"toolRequests"`
	Success      *bool                `json:"success"`
	Result       struct {
		Content string `json:"content"`
	} `json:"result"`
	Error json.RawMessage `json:"error"`
}

type copilotToolRequest struct {
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

// copilotShellExit matches the trailer copilot appends to shell tool output,
// e.g. "<shellId: 0 completed with exit code 1>".
var copilotShellExit = regexp.MustCompile(`completed with exit code (\d+)>\s*$`)

func (CopilotAdapter) Parse(line []byte) ([]CanonicalEvent, error) {
	var payload copilotLine
	if err := json.Unmarshal(line, &payload); err != nil {
		return nil, fmt.Errorf("parse copilot line: %w", err)
	}
	ts := parseTimestamp(parseTimestampField(payload.Injected), "")

	switch payload.Type {
	case "user.message":
		return []CanonicalEvent{{Type: EventInit, Message: "session started", Timestamp: ts}}, nil

	case "assistant.message":
		var events []CanonicalEvent
		for _, req := range payload.Data.ToolRequests {
			events = append(events, CanonicalEvent{Type: EventAction, Message: summarizeClaudeToolUse(req.Name, req.Arguments), Timestamp: ts})
		}
		if text := strings.TrimSpace(payload.Data.Content); text != "" {
			events = append(events, CanonicalEvent{Type: EventAction, Message: "text:" + text, Timestamp: ts})
		}
		return events, nil

	case "tool.execution_complete":
		output := strings.TrimSpace(payload.Data.Result.Content)
		failed := payload.Data.Success != nil && !*payload.Data.Success
		if m := copilotShellExit.FindStringSubmatch(output); m != nil {
			if code, _ := strconv.Atoi(m[1]); code != 0 {
				failed = true
			}
		}
		if !failed {
			return nil, nil
		}
		msg := extractFlexibleMessage(payload.Data.Error)
		if msg == "" {
			msg = output
		}
		return []CanonicalEvent{{Type: EventError, Message: "tool failed: " + msg, Timestamp: ts}}, nil

	case "result":
		if payload.ExitCode != 0 {
			return []CanonicalEvent{{Type: EventError, Message: fmt.Sprintf("copilot exited with code %d", payload.ExitCode), Timestamp: ts}}, nil
		}
		return []CanonicalEvent{{Type: EventResult, Message: "turn complete", Timestamp: ts}}, nil
	}
	return nil, nil
}
