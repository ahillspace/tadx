package app

import (
	"slices"
	"testing"

	searchaction "github.com/ahillspace/tadx/actions/search"
	resourcesearch "github.com/ahillspace/tadx/internal/resources/search"
)

func TestCombinedContinuationPreservesSelectedSourceScope(t *testing.T) {
	for _, dedicatedTypes := range [][]string{{"group", "user"}, {"definition", "group", "user"}} {
		native := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "wb-1", Type: "workbook", Name: "Sales"}}}}}
		dedicated := &appSearchFake{pages: []resourcesearch.Page{{Items: []resourcesearch.Item{{LUID: "group-1", Type: "group", Name: "Sales"}}}}}
		source := globalSearchSource{native: native, dedicated: dedicated}
		nativeTypes := []string{"datasource", "flow", "project", "workbook"}
		if slices.Contains(dedicatedTypes, "definition") {
			nativeTypes = append(nativeTypes, "metric")
		}
		types := append(slices.Clone(nativeTypes), dedicatedTypes...)
		input := searchaction.Input{Terms: "Sales", Limit: 1}
		first, err := searchaction.Execute(t.Context(), source, input, types)
		if err != nil || first.Page.NextCursor == "" || len(dedicated.inputs) != 0 {
			t.Fatalf("first=%+v err=%v dedicated=%+v", first, err, dedicated.inputs)
		}
		if len(native.inputs) != 1 || !slices.Equal(native.inputs[0].Types, nativeTypes) {
			t.Fatalf("native scope=%+v want=%v", native.inputs, nativeTypes)
		}
		input.Cursor = first.Page.NextCursor
		second, err := searchaction.Execute(t.Context(), source, input, types)
		if err != nil || len(second.Items) != 1 || second.Items[0].LUID != "group-1" || len(dedicated.inputs) != 1 || !slices.Equal(dedicated.inputs[0].Types, dedicatedTypes) {
			t.Fatalf("second=%+v err=%v dedicated=%+v", second, err, dedicated.inputs)
		}
	}
}
