package list

import (
	"encoding/json"
	"testing"
)

func TestExplorationFullProjectionDistinguishesKnownEmptyFromUnknown(t *testing.T) {
	known := Output{Definitions: []Definition{}}.FullOutput().(FullResult)
	if known.Definitions == nil {
		t.Fatal("known-empty definitions became nil")
	}
	knownJSON, err := json.Marshal(known)
	if err != nil {
		t.Fatal(err)
	}
	var knownObject map[string]json.RawMessage
	if err := json.Unmarshal(knownJSON, &knownObject); err != nil || string(knownObject["definitions"]) != "[]" {
		t.Fatalf("known-empty JSON = %s, %v", knownJSON, err)
	}

	unknown := Output{}.FullOutput().(FullResult)
	if unknown.Definitions != nil {
		t.Fatalf("unknown definitions became known-empty: %#v", unknown.Definitions)
	}
	unknownJSON, err := json.Marshal(unknown)
	if err != nil {
		t.Fatal(err)
	}
	var unknownObject map[string]json.RawMessage
	if err := json.Unmarshal(unknownJSON, &unknownObject); err != nil || string(unknownObject["definitions"]) != "null" {
		t.Fatalf("unknown JSON = %s, %v", unknownJSON, err)
	}
}
