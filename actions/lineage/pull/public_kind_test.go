package pull_test

import (
	"context"
	"encoding/json"
	lineagepull "github.com/ahillspace/tadx/actions/lineage/pull"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
	"testing"
)

func TestDatasourceKindAcceptsPublicAndLegacyNames(t *testing.T) {
	for _, kind := range []string{"datasource", "published_datasource"} {
		action := lineagepull.New(resolver{resource: lineagepull.Resource{Kind: "published_datasource", LUID: "ds-1", Name: "Sales"}}, reader{graph: lineagepull.Graph{Complete: true}}, &writer{})
		output, err := action.Execute(context.Background(), lineagepull.Input{Workspace: "workspace", Kind: kind, Selector: identity.Selector{LUID: "ds-1"}})
		if err != nil {
			t.Fatal(err)
		}
		for _, projection := range []any{output.CompactOutput(), output.FullOutput()} {
			data, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}
			if strings.Contains(string(data), "published_datasource") || !strings.Contains(string(data), `"kind":"datasource"`) {
				t.Fatalf("output=%s", data)
			}
		}
	}
}
