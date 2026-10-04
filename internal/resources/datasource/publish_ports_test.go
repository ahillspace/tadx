package datasource

import (
	"testing"

	datasource "github.com/ahillspace/tadx/actions/datasource"
)

func TestDatasourcePublishModeMappingIsExhaustive(t *testing.T) {
	for _, mode := range []datasource.Mode{datasource.ModeCreate, datasource.ModeOverwrite, datasource.ModeAppend, datasource.ModeReplace} {
		if _, err := datasourcePublishMode(mode); err != nil {
			t.Fatalf("mode %q: %v", mode, err)
		}
	}
	if _, err := datasourcePublishMode("unexpected"); err == nil {
		t.Fatal("unexpected mode succeeded")
	}
}
