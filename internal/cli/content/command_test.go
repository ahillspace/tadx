package content_test

import (
	"context"
	"testing"

	workbookpublish "github.com/ahillspace/tadx/actions/workbook/publish"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
	contentcli "github.com/ahillspace/tadx/internal/cli/content"
)

type contentActions struct {
	publishInputs []workbookpublish.Input
}

func (a *contentActions) Execute(_ context.Context, input workbookpull.Input) (workbookpull.Output, error) {
	return workbookpull.Output{}, nil
}

type publisher struct{ actions *contentActions }

func (p publisher) Execute(_ context.Context, input workbookpublish.Input, _ bool) (workbookpublish.Output, error) {
	p.actions.publishInputs = append(p.actions.publishInputs, input)
	return workbookpublish.Output{}, nil
}

type renderer struct{}

func (renderer) Render(any) error { return nil }

func TestWorkbookPublishAcceptsLogicalWorkspaceAndRelativeArtifact(t *testing.T) {
	actions := &contentActions{}
	command := contentcli.New(contentcli.Dependencies{Puller: actions, Publisher: publisher{actions}, Renderer: renderer{}, MutationsEnabled: true})
	command.SetArgs([]string{"workbook", "publish", "--workspace", "development", "--artifact", "artifacts/workbook/Finance--identity", "--environment", "production", "--project-id", "project-1"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(actions.publishInputs) != 1 || actions.publishInputs[0].Workspace != "development" || actions.publishInputs[0].ArtifactPath != "artifacts/workbook/Finance--identity" {
		t.Fatalf("publish inputs = %#v", actions.publishInputs)
	}
}

func TestWorkbookPublishRejectsMachineAndEscapingArtifactPaths(t *testing.T) {
	for _, artifactPath := range []string{`C:\workspace\Finance`, "/workspace/Finance", "../Finance", "artifacts/workbook/../Finance", `artifacts\workbook\Finance`} {
		command := contentcli.New(contentcli.Dependencies{Puller: &contentActions{}, Publisher: publisher{&contentActions{}}, Renderer: renderer{}, MutationsEnabled: true})
		command.SetArgs([]string{"workbook", "publish", "--artifact", artifactPath, "--environment", "production", "--project-id", "project-1"})
		if err := command.Execute(); err == nil {
			t.Fatalf("artifact path %q was accepted", artifactPath)
		}
	}
}
