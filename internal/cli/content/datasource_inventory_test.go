package content

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"reflect"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
)

type datasourceInventoryActions struct {
	listInputs    []datasourceops.ListInput
	inspectInputs []datasourceops.InspectInput
}

func (a *datasourceInventoryActions) ListDatasources(_ context.Context, input datasourceops.ListInput) (datasourceops.ListOutput, error) {
	a.listInputs = append(a.listInputs, input)
	return datasourceops.ListOutput{Status: "listed"}, nil
}

func (a *datasourceInventoryActions) InspectDatasource(_ context.Context, input datasourceops.InspectInput) (datasourceops.InspectOutput, error) {
	a.inspectInputs = append(a.inspectInputs, input)
	return datasourceops.InspectOutput{Status: "found"}, nil
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
		"list", "--environment", "dev", "--name", "Sales", "--owner", "owner", "--project-id", "project-1", "--project-name", "Ops",
		"--type", "hyper", "--tag", "daily", "--updated-after", "2026-01-01T00:00:00Z",
		"--updated-before", "2026-09-01T00:00:00Z", "--limit", "10", "--cursor", "opaque",
		"--cache",
	})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	want := datasourceops.ListInput{
		Environment: "dev", Name: "Sales", OwnerName: "owner", ProjectLUID: "project-1", ProjectName: "Ops", Type: "hyper", Tag: "daily",
		UpdatedAfter: "2026-01-01T00:00:00Z", UpdatedBefore: "2026-09-01T00:00:00Z", Limit: 10, Cursor: "opaque", Cache: true,
	}
	if !reflect.DeepEqual(actions.listInputs, []datasourceops.ListInput{want}) || len(renderer.values) != 1 {
		t.Fatalf("inputs = %#v, rendered = %#v", actions.listInputs, renderer.values)
	}
	if _, ok := renderer.values[0].(datasourceops.ListOutput); !ok {
		t.Fatalf("rendered type = %T", renderer.values[0])
	}
}

func TestDatasourceInspectUsesAuthoritativeOrExactSelectorGrammar(t *testing.T) {
	tests := []struct {
		name string
		args []string
		want datasourceops.InspectInput
	}{
		{name: "LUID", args: []string{"inspect", "--environment", "dev", "--id", "ds-1", "--cache"}, want: datasourceops.InspectInput{Environment: "dev", Selector: datasourceSelector("ds-1", "", ""), Cache: true}},
		{name: "exact labels", args: []string{"inspect", "--name", "Sales", "--project", "Department/Ops"}, want: datasourceops.InspectInput{Selector: datasourceSelector("", "Sales", "Department/Ops")}},
		{name: "exact project ID", args: []string{"inspect", "--name", "Sales", "--project-id", "project-1"}, want: datasourceops.InspectInput{Selector: datasourceSelectorWithProjectLUID("", "Sales", "", "project-1")}},
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
			if !reflect.DeepEqual(actions.inspectInputs, []datasourceops.InspectInput{test.want}) || len(renderer.values) != 1 {
				t.Fatalf("inputs = %#v, rendered = %#v", actions.inspectInputs, renderer.values)
			}
		})
	}
}

func TestDatasourceInspectRejectsIncompleteOrConflictingSelectors(t *testing.T) {
	for _, args := range [][]string{
		{"inspect"},
		{"inspect", "--name", "Sales"},
		{"inspect", "--project", "Department/Ops"},
		{"inspect", "--id", "ds-1", "--name", "Sales", "--project", "Department/Ops"},
		{"inspect", "--name", "Sales", "--project", "Department/Ops", "--project-id", "project-1"},
	} {
		actions := &datasourceInventoryActions{}
		command := newDatasourceInventory(actions, actions, &datasourceInventoryRenderer{})
		command.SetArgs(args)
		if err := command.Execute(); err == nil || len(actions.inspectInputs) != 0 {
			t.Fatalf("args %v: error = %v, inputs = %#v", args, err, actions.inspectInputs)
		}
	}
}

func datasourceSelector(luid, name, projectPath string) identity.Selector {
	var input datasourceops.InspectInput
	input.SetSelector(luid, name, projectPath)
	return input.Selector
}

func datasourceSelectorWithProjectLUID(luid, name, projectPath, projectLUID string) identity.Selector {
	var input datasourceops.InspectInput
	input.SetSelectorWithProjectLUID(luid, name, projectPath, projectLUID)
	return input.Selector
}
