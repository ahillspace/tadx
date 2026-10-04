package app

import (
	"encoding/base64"
	"encoding/json"
)

// Legacy process-boundary cursors can still target a previously stored snapshot.
func legacyInventorySnapshot(value string) bool {
	data, err := base64.RawURLEncoding.DecodeString(value)
	if err != nil {
		return false
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(data, &fields) != nil {
		return false
	}
	for _, key := range []string{"c", "Snapshot"} {
		var token string
		if json.Unmarshal(fields[key], &token) == nil && token != "" {
			return true
		}
	}
	return false
}
