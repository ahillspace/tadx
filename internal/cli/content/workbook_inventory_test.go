package content

import (
	"context"
	"testing"

	workbookget "github.com/ahillspace/tadx/actions/workbook/get"
	workbooklist "github.com/ahillspace/tadx/actions/workbook/list"
)

type workbookInventoryCommands struct {
	listInput workbooklist.Input
	getInput  workbookget.Input
}

func (c *workbookInventoryCommands) ListWorkbooks(_ context.Context, input workbooklist.Input) (workbooklist.Output, error) {
	c.listInput = input
	return workbooklist.Output{Status: "listed"}, nil
}

func (c *workbookInventoryCommands) GetWorkbook(_ context.Context, input workbookget.Input) (workbookget.Output, error) {
	c.getInput = input
	return workbookget.Output{Status: "found"}, nil
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
	command.SetArgs([]string{"--environment", "dev", "--name", "Finance", "--owner", "Analyst", "--project-name", "Ops", "--tag", "quarterly", "--limit", "20", "--cursor", "next", "--catalog"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	input := actions.listInput
	if input.Environment != "dev" || input.Name != "Finance" || input.OwnerName != "Analyst" || input.ProjectName != "Ops" || input.Tag != "quarterly" || input.Limit != 20 || input.Cursor != "next" || !input.Catalog {
		t.Fatalf("input = %#v", input)
	}
	if _, ok := renderer.value.(workbooklist.Output); !ok {
		t.Fatalf("rendered value = %T", renderer.value)
	}
}

func TestWorkbookListRejectsUnsupportedProjectLUIDFilter(t *testing.T) {
	command := newWorkbookList(&workbookInventoryCommands{}, &workbookInventoryRenderer{})
	command.SetArgs([]string{"--project-id", "project-1"})
	if err := command.Execute(); err == nil {
		t.Fatal("expected unknown project LUID filter flag")
	}
}

func TestWorkbookGetAcceptsOnlyExactSelectorGrammar(t *testing.T) {
	tests := []struct {
		name        string
		args        []string
		wantError   bool
		wantLUID    string
		wantName    string
		wantProject string
	}{
		{name: "LUID", args: []string{"--environment", "dev", "--id", "wb-1", "--catalog"}, wantLUID: "wb-1"},
		{name: "name and project", args: []string{"--name", "Finance", "--project", "Department/Ops"}, wantName: "Finance", wantProject: "Department/Ops"},
		{name: "mixed selectors", args: []string{"--id", "wb-1", "--name", "Finance", "--project", "Ops"}, wantError: true},
		{name: "name only", args: []string{"--name", "Finance"}, wantError: true},
		{name: "project only", args: []string{"--project", "Ops"}, wantError: true},
		{name: "positional argument", args: []string{"unexpected", "--id", "wb-1"}, wantError: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			actions := &workbookInventoryCommands{}
			renderer := &workbookInventoryRenderer{}
			command := newWorkbookGet(actions, renderer)
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
			selector := actions.getInput.Selector
			if string(selector.LUID) != test.wantLUID || selector.Name != test.wantName || selector.ProjectPath != test.wantProject {
				t.Fatalf("selector = %#v", selector)
			}
			if test.name == "LUID" && !actions.getInput.Catalog {
				t.Fatal("--catalog was not forwarded")
			}
			if _, ok := renderer.value.(workbookget.Output); !ok {
				t.Fatalf("rendered value = %T", renderer.value)
			}
		})
	}
}
