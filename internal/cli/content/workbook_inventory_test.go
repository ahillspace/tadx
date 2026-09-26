package content

import (
	"context"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"testing"
)

type workbookInventoryCommands struct {
	listInput    workbookops.ListInput
	inspectInput workbookops.InspectInput
}

func (c *workbookInventoryCommands) ListWorkbooks(_ context.Context, input workbookops.ListInput) (workbookops.ListOutput, error) {
	c.listInput = input
	return workbookops.ListOutput{Status: "listed"}, nil
}

func (c *workbookInventoryCommands) InspectWorkbook(_ context.Context, input workbookops.InspectInput) (workbookops.InspectOutput, error) {
	c.inspectInput = input
	return workbookops.InspectOutput{Status: "found"}, nil
}

type workbookInventoryRenderer struct{ value any }

func (r *workbookInventoryRenderer) Render(value any) error {
	r.value = value
	return nil
}

func TestWorkbookListParsesEveryBoundedFilter(t *testing.T) {
	actions := &workbookInventoryCommands{}
	renderer := &workbookInventoryRenderer{}
	command := newWorkbookList(actions, renderer)
	command.SetArgs([]string{"--environment", "dev", "--name", "Finance", "--owner", "Analyst", "--project-id", "project-1", "--project-name", "Ops", "--tag", "quarterly", "--limit", "20", "--cursor", "next", "--cache"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	input := actions.listInput
	if input.Environment != "dev" || input.Name != "Finance" || input.OwnerName != "Analyst" || input.ProjectLUID != "project-1" || input.ProjectName != "Ops" || input.Tag != "quarterly" || input.Limit != 20 || input.Cursor != "next" || !input.Cache {
		t.Fatalf("input = %#v", input)
	}
	if _, ok := renderer.value.(workbookops.ListOutput); !ok {
		t.Fatalf("rendered value = %T", renderer.value)
	}
}

func TestWorkbookListAcceptsAuthoritativeProjectLUIDFilter(t *testing.T) {
	command := newWorkbookList(&workbookInventoryCommands{}, &workbookInventoryRenderer{})
	command.SetArgs([]string{"--project-id", "project-1"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
}

func TestWorkbookInspectAcceptsOnlyExactSelectorGrammar(t *testing.T) {
	tests := []struct {
		name          string
		args          []string
		wantError     bool
		wantLUID      string
		wantName      string
		wantProject   string
		wantProjectID string
	}{
		{name: "LUID", args: []string{"--environment", "dev", "--id", "wb-1", "--cache"}, wantLUID: "wb-1"},
		{name: "name and project", args: []string{"--name", "Finance", "--project", "Department/Ops"}, wantName: "Finance", wantProject: "Department/Ops"},
		{name: "name and project ID", args: []string{"--name", "Finance", "--project-id", "project-1"}, wantName: "Finance", wantProjectID: "project-1"},
		{name: "mixed selectors", args: []string{"--id", "wb-1", "--name", "Finance", "--project", "Ops"}, wantError: true},
		{name: "name only", args: []string{"--name", "Finance"}, wantError: true},
		{name: "project only", args: []string{"--project", "Ops"}, wantError: true},
		{name: "positional argument", args: []string{"unexpected", "--id", "wb-1"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions := &workbookInventoryCommands{}
			renderer := &workbookInventoryRenderer{}
			command := newWorkbookInspect(actions, renderer)
			command.SetArgs(test.args)
			err := command.Execute()
			if test.wantError {
				if err == nil {
					t.Fatal("expected selector error")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			selector := actions.inspectInput.Selector
			if string(selector.LUID) != test.wantLUID || selector.Name != test.wantName || selector.ProjectPath != test.wantProject || string(selector.ProjectLUID) != test.wantProjectID {
				t.Fatalf("selector = %#v", selector)
			}
			if test.name == "LUID" && !actions.inspectInput.Cache {
				t.Fatal("--cache was not forwarded")
			}
			if _, ok := renderer.value.(workbookops.InspectOutput); !ok {
				t.Fatalf("rendered value = %T", renderer.value)
			}
		})
	}
}
