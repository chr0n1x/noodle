package schemadoc

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestIsRawMessageTypeRecognizesJSONRawMessage(t *testing.T) {
	typ := reflect.TypeOf(json.RawMessage{})
	if !isRawMessageType(typ) {
		t.Fatalf("isRawMessageType(%v) = false, want true", typ)
	}
	if !isRawMessageType(reflect.PointerTo(typ)) {
		t.Fatalf("isRawMessageType(*%v) = false, want true", typ)
	}
}

func TestGatherLeafPathsTreatsRawMessageMapValueAsLeaf(t *testing.T) {
	type holder struct {
		Extra map[string]json.RawMessage `json:"extra"`
	}
	got, err := gatherLeafPaths(reflect.TypeOf(holder{}), "")
	if err != nil {
		t.Fatalf("gatherLeafPaths: %v", err)
	}
	if _, ok := got["extra{}"]; !ok || len(got) != 1 {
		t.Fatalf("leaf paths = %v, want only \"extra{}\"", got)
	}
}
