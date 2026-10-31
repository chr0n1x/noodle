package stamp

import (
	"testing"

	"github.com/poteto/noodle/parse"
)

type fakeAdapter struct{}

func (fakeAdapter) Parse([]byte) ([]parse.CanonicalEvent, error) {
	return []parse.CanonicalEvent{{Type: parse.EventComplete}}, nil
}

func TestProcessLineRoutesToConfiguredProvider(t *testing.T) {
	p := NewProcessor()
	p.Registry.Register("fake", fakeAdapter{})
	p.Provider = "fake"
	_, events, err := p.ProcessLine([]byte(`{"type":"result"}`))
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 1 || events[0].Provider != "fake" || events[0].Type != parse.EventComplete {
		t.Fatalf("events = %+v", events)
	}
}
