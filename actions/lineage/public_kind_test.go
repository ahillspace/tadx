package lineage_test

import (
	"encoding/json"
	lineagepull "github.com/ahillspace/tadx/actions/lineage"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
	"testing"
)

func TestDatasourceKindAcceptsPublicAndLegacyNames(t *testing.T) {
	for _, kind := range []string{"datasource", "published_datasource"} {
		input, err := lineagepull.NormalizeInput(lineagepull.Input{Workspace: "workspace", Kind: kind, Selector: identity.Selector{LUID: "ds-1"}})
		if err != nil || input.Kind != "published_datasource" || input.Direction != "both" || input.Depth != 1 {
			t.Fatalf("normalized input = %#v, err = %v", input, err)
		}
		output, err := testExecute(t.Context(), resolver{resource: lineagepull.Resource{Kind: "published_datasource", LUID: "ds-1", Name: "Sales"}}, reader{graph: lineagepull.Graph{Complete: true}}, &writer{}, input)
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
