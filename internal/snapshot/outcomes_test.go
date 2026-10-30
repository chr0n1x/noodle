package snapshot

import (
	"encoding/json"
	"testing"
)

func TestStageOutcomeFields(t *testing.T) {
	s := Stage{TaskKey: "execute", Status: "completed", Extra: map[string]json.RawMessage{
		"outcomes": json.RawMessage(`[{"state":"reject","action":"promote","feedback":"no tests"}]`),
	}}
	setOutcomeFields(&s)
	if s.State != "reject" {
		t.Fatalf("state = %q, want reject", s.State)
	}
	if len(s.Outcomes) != 1 || s.Outcomes[0].Action != "promote" || s.Outcomes[0].Feedback != "no tests" {
		t.Fatalf("outcomes = %+v", s.Outcomes)
	}

	plain := Stage{TaskKey: "quality", Status: "active"}
	setOutcomeFields(&plain)
	if plain.State != "in-progress" || plain.Outcomes != nil {
		t.Fatalf("plain stage = %q, %+v", plain.State, plain.Outcomes)
	}
	data, _ := json.Marshal(plain)
	var raw map[string]json.RawMessage
	_ = json.Unmarshal(data, &raw)
	if _, ok := raw["outcomes"]; ok {
		t.Fatal("a stage with no outcomes should omit the field")
	}
	if string(raw["state"]) != `"in-progress"` {
		t.Fatalf("state json = %s", raw["state"])
	}
}
