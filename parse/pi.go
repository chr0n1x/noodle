package parse

import (
	"encoding/json"
	"fmt"
	"strings"
)

// PiAdapter parses `pi --mode json` output. pig emits the same events.
type PiAdapter struct{}

type piLine struct {
	Type     string          `json:"type"`
	Injected json.RawMessage `json:"_ts"`
	Message  piMessage       `json:"message"`
	Messages []piMessage     `json:"messages"`
	ToolName string          `json:"toolName"`
	IsError  bool            `json:"isError"`
	Result   piToolResult    `json:"result"`
}

type piMessage struct {
	Role         string      `json:"role"`
	Content      []piContent `json:"content"`
	Usage        piUsage     `json:"usage"`
	StopReason   string      `json:"stopReason"`
	ErrorMessage string      `json:"errorMessage"`
}

type piContent struct {
	Type      string          `json:"type"`
	Text      string          `json:"text"`
	Name      string          `json:"name"`
	Arguments json.RawMessage `json:"arguments"`
}

type piUsage struct {
	Input  int `json:"input"`
	Output int `json:"output"`
	Cost   struct {
		Total float64 `json:"total"`
	} `json:"cost"`
}

type piToolResult struct {
	Content []piContent `json:"content"`
}

func (PiAdapter) Parse(line []byte) ([]CanonicalEvent, error) {
	var payload piLine
	if err := json.Unmarshal(line, &payload); err != nil {
		return nil, fmt.Errorf("parse pi line: %w", err)
	}
	ts := parseTimestamp(parseTimestampField(payload.Injected), "")

	switch payload.Type {
	case "session":
		return []CanonicalEvent{{Type: EventInit, Message: "session started", Timestamp: ts}}, nil

	case "message_end":
		if payload.Message.Role != "assistant" {
			return nil, nil
		}
		var events []CanonicalEvent
		for _, c := range payload.Message.Content {
			switch c.Type {
			case "toolCall":
				events = append(events, CanonicalEvent{Type: EventAction, Message: summarizeClaudeToolUse(c.Name, c.Arguments), Timestamp: ts})
			case "text":
				if text := strings.TrimSpace(c.Text); text != "" {
					events = append(events, CanonicalEvent{Type: EventAction, Message: "text:" + text, Timestamp: ts})
				}
			}
		}
		switch payload.Message.StopReason {
		case "error":
			msg := strings.TrimSpace(payload.Message.ErrorMessage)
			if msg == "" {
				msg = "turn failed"
			}
			events = append(events, CanonicalEvent{Type: EventError, Message: msg, Timestamp: ts})
		case "aborted":
			events = append(events, CanonicalEvent{Type: EventError, Message: "turn aborted", Timestamp: ts})
		}
		return events, nil

	case "tool_execution_end":
		if !payload.IsError {
			return nil, nil
		}
		var parts []string
		for _, c := range payload.Result.Content {
			if text := strings.TrimSpace(c.Text); text != "" {
				parts = append(parts, text)
			}
		}
		msg := strings.TrimSpace(payload.ToolName + ": " + strings.Join(parts, "\n"))
		return []CanonicalEvent{{Type: EventError, Message: msg, Timestamp: ts}}, nil

	case "agent_end":
		event := CanonicalEvent{Type: EventResult, Message: "turn complete", Timestamp: ts}
		for _, m := range payload.Messages {
			if m.Role != "assistant" {
				continue
			}
			event.CostUSD += m.Usage.Cost.Total
			event.TokensIn += m.Usage.Input
			event.TokensOut += m.Usage.Output
		}
		return []CanonicalEvent{event}, nil
	}
	return nil, nil
}
