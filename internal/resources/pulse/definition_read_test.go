package pulse

import (
	"testing"

	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestDefinitionInspectRejectsMalformedConfiguration(t *testing.T) {
	if _, err := definitionGetItem(tableaupulse.Definition{Configuration: []byte(`invalid`)}); err == nil {
		t.Fatal("inspect accepted malformed definition configuration")
	}
}
