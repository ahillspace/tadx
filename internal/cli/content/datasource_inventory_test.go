package content

import (
	"context"
	"reflect"
	"testing"

	datasourceget "github.com/ahillspace/tadx/actions/datasource/get"
	datasourcelist "github.com/ahillspace/tadx/actions/datasource/list"
	"github.com/ahillspace/tadx/internal/identity"
)

type datasourceInventoryActions struct {
	listInputs []datasourcelist.Input
	getInputs  []datasourceget.Input
}

func (a *datasourceInventoryActions) ListDatasources(_ context.Context, input datasourcelist.Input) (datasourcelist.Output, error) {
	a.listInputs = append(a.listInputs, input)
	return datasourcelist.Output{Status: "listed"}, nil
}

func (a *datasourceInventoryActions) GetDatasource(_ context.Context, input datasourceget.Input) (datasourceget.Output, error) {
	a.getInputs = append(a.getInputs, input)
	return datasourceget.Output{Status: "found"}, nil
}

type datasourceInventoryRenderer struct{ values []any }

func (r *datasourceInventoryRenderer) Render(value any) error {
	r.values = append(r.values, value)
	return nil
}

func TestDatasourceListForwardsEveryBoundedFilterAndRendersOutput(t *testing.T) {
	actions := &datasourceInventoryActions{}
	renderer := &datasourceInventoryRenderer{}
	command := newDatasourceInventory(actions, actions, renderer)
	command.SetArgs([]string{
		"list", "--environment", "dev", "--name", "Sales", "--owner", "owner", "--project-name", "Ops",
		"--type", "hyper", "--tag", "daily", "--updated-after", "2026-01-01T00:00:00Z",
		"--updated-before", "2026-09-01T00:00:00Z", "--limit", "10", "--cursor", "opaque",
		"--catalog",
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := datasourcelist.Input{
		Environment: "dev", Name: "Sales", OwnerName: "owner", ProjectName: "Ops", Type: "hyper", Tag: "daily",
		UpdatedAfter: "2026-01-01T00:00:00Z", UpdatedBefore: "2026-09-01T00:00:00Z", Limit: 10, Cursor: "opaque", Catalog: true,
	}
	if !reflect.DeepEqual(actions.listInputs, []datasourcelist.Input{want}) || len(renderer.values) != 1 {
		t.Fatalf("inputs = %#v, rendered = %#v", actions.listInputs, renderer.values)
	}
	if _, ok := renderer.values[0].(datasourcelist.Output); !ok {
		t.Fatalf("rendered type = %T", renderer.values[0])
	}
}

func TestDatasourceGetUsesAuthoritativeOrExactSelectorGrammar(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want datasourceget.Input
	}{
		{name: "LUID", args: []string{"get", "--environment", "dev", "--id", "ds-1", "--catalog"}, want: datasourceget.Input{Environment: "dev", Selector: datasourceSelector("ds-1", "", ""), Catalog: true}},
		{name: "exact labels", args: []string{"get", "--name", "Sales", "--project", "Department/Ops"}, want: datasourceget.Input{Selector: datasourceSelector("", "Sales", "Department/Ops")}},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions := &datasourceInventoryActions{}
			renderer := &datasourceInventoryRenderer{}
			command := newDatasourceInventory(actions, actions, renderer)
			command.SetArgs(test.args)
			if err := command.Execute(); err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(actions.getInputs, []datasourceget.Input{test.want}) || len(renderer.values) != 1 {
				t.Fatalf("inputs = %#v, rendered = %#v", actions.getInputs, renderer.values)
			}
		})
	}
}

func TestDatasourceGetRejectsIncompleteOrConflictingSelectors(t *testing.T) {
	for _, args := range [][]string{
		{"get"},
		{"get", "--name", "Sales"},
		{"get", "--project", "Department/Ops"},
		{"get", "--id", "ds-1", "--name", "Sales", "--project", "Department/Ops"},
	} {
		actions := &datasourceInventoryActions{}
		command := newDatasourceInventory(actions, actions, &datasourceInventoryRenderer{})
		command.SetArgs(args)
		if err := command.Execute(); err == nil || len(actions.getInputs) != 0 {
			t.Fatalf("args %v: error = %v, inputs = %#v", args, err, actions.getInputs)
		}
	}
}

func datasourceSelector(luid, name, projectPath string) identity.Selector {
	var input datasourceget.Input
	input.SetSelector(luid, name, projectPath)
	return input.Selector
}
