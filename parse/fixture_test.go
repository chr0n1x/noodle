package parse

import (
	"encoding/json"
	"reflect"
	"strings"
	"testing"

	"github.com/poteto/noodle/internal/testutil/fixturedir"
)

func TestDirectoryFixtures(t *testing.T) {
	t.Helper()

	fixturedir.AssertValidFixtureRoot(t, "testdata")
	inventory := fixturedir.LoadInventory(t, "testdata")

	for _, fixtureCase := range inventory.Cases {
		fixtureCase := fixtureCase
		t.Run(fixtureCase.Name, func(t *testing.T) {
			adapter := adapterForFixture(t, fixtureCase.Name)
			state := fixtureCase.States[0]
			inputLines := fixturedir.NonEmptyLines(t, state.MustReadFile(t, "input.ndjson"), "input.ndjson")
			errorExpectation := fixtureCase.ExpectedError

			expectedEvents := make([]CanonicalEvent, 0)
			if errorExpectation == nil {
				rawExpected := fixturedir.MustSection(t, fixtureCase, "Expected Events")
				expectedEvents = parseCanonicalEvents(
					t,
					fixturedir.NonEmptyLines(t, []byte(rawExpected), "Expected Events"),
				)
			}

			actualEvents := make([]CanonicalEvent, 0)
			var parseErr error
			for _, line := range inputLines {
				events, err := adapter.Parse([]byte(line))
				if err != nil {
					parseErr = err
					break
				}
				actualEvents = append(actualEvents, events...)
			}

			fixturedir.AssertError(t, "parse fixture", parseErr, errorExpectation)
			if errorExpectation != nil {
				return
			}
			if !reflect.DeepEqual(actualEvents, expectedEvents) {
				t.Fatalf("fixture mismatch\nactual:   %#v\nexpected: %#v", actualEvents, expectedEvents)
			}
		})
	}
}

// adapterForFixture looks up the registry adapter named by the fixture's
// leading segment, e.g. "codex-agent-message" uses the codex adapter.
func adapterForFixture(t *testing.T, fixtureName string) LogAdapter {
	t.Helper()

	provider, _, _ := strings.Cut(strings.ToLower(strings.TrimSpace(fixtureName)), "-")
	adapter, ok := NewRegistry().AdapterForProvider(provider)
	if !ok {
		t.Fatalf("fixture %s names no registered provider", fixtureName)
	}
	return adapter
}

func parseCanonicalEvents(t *testing.T, lines []string) []CanonicalEvent {
	t.Helper()

	events := make([]CanonicalEvent, 0, len(lines))
	for _, line := range lines {
		var event CanonicalEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			t.Fatalf("parse expected event %q: %v", line, err)
		}
		events = append(events, event)
	}
	return events
}
