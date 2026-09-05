package content

import (
	"context"
	"testing"

	workbookdelete "github.com/ahillspace/tadx/actions/workbook/delete"
)

type workbookDeleteCommands struct {
	input   workbookdelete.Input
	preview bool
}

func (c *workbookDeleteCommands) DeleteWorkbook(_ context.Context, input workbookdelete.Input, preview bool) (workbookdelete.Output, error) {
	c.input, c.preview = input, preview
	return workbookdelete.Output{}, nil
}

type workbookDeleteRenderer struct{ calls int }

func (r *workbookDeleteRenderer) Render(any) error {
	r.calls++
	return nil
}

func TestWorkbookDeleteParsesExactSelectorAndPreview(t *testing.T) {
	actions := &workbookDeleteCommands{}
	renderer := &workbookDeleteRenderer{}
	command := newWorkbookDelete(actions, renderer, true)
	command.SetArgs([]string{"--environment", "dev", "--name", "Finance", "--project", "Department/Ops", "--preview"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.input.Environment != "dev" || actions.input.Selector.Name != "Finance" || actions.input.Selector.ProjectPath != "Department/Ops" || !actions.preview || renderer.calls != 1 {
		t.Fatalf("input=%#v preview=%t renders=%d", actions.input, actions.preview, renderer.calls)
	}
}

func TestWorkbookDeleteRequiresExplicitEnvironment(t *testing.T) {
	command := newWorkbookDelete(&workbookDeleteCommands{}, &workbookDeleteRenderer{}, true)
	command.SetArgs([]string{"--id", "wb-1"})
	if err := command.ExecuteContext(context.Background()); err == nil {
		t.Fatal("expected explicit environment error")
	}
}

func TestWorkbookDeleteStaysDiscoverableWhenMutationsAreDisabled(t *testing.T) {
	command := newWorkbookDelete(&workbookDeleteCommands{}, &workbookDeleteRenderer{}, false)
	if command.Hidden {
		t.Fatal("workbook delete is hidden")
	}
}
