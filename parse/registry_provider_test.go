package parse

import "testing"

type fakeAdapter struct{}

func (fakeAdapter) Parse([]byte) ([]CanonicalEvent, error) {
	return []CanonicalEvent{{Type: EventComplete, Message: "fake"}}, nil
}

func TestParseLineForUsesTheNamedProvider(t *testing.T) {
	r := NewRegistry()
	r.Register("fake", fakeAdapter{})
	provider, events, err := r.ParseLineFor("fake", []byte(`{"type":"result","exitCode":0}`))
	if err != nil {
		t.Fatal(err)
	}
	if provider != "fake" || len(events) != 1 || events[0].Message != "fake" {
		t.Fatalf("got provider %q events %+v", provider, events)
	}
}

func TestParseLineForDetectsWhenProviderEmpty(t *testing.T) {
	provider, _, err := NewRegistry().ParseLineFor("", []byte(`{"type":"thread.started","thread_id":"t"}`))
	if err != nil || provider != "codex" {
		t.Fatalf("provider %q err %v", provider, err)
	}
}

func TestParseLineForUnknownProvider(t *testing.T) {
	if _, _, err := NewRegistry().ParseLineFor("gemini", []byte(`{"type":"x"}`)); err == nil {
		t.Fatal("want error for provider without adapter")
	}
}
