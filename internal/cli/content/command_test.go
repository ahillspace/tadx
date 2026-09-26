package content_test

import (
	"context"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"testing"

	contentcli "github.com/ahillspace/tadx/internal/cli/content"
)

type contentActions struct {
	publishInputs []workbookops.PublishInput
}

func (a *contentActions) Execute(_ context.Context, input workbookops.PullInput) (workbookops.PullOutput, error) {
	return workbookops.PullOutput{}, nil
}

type publisher struct{ actions *contentActions }

func (p publisher) Execute(_ context.Context, input workbookops.PublishInput, _ bool) (workbookops.PublishOutput, error) {
	p.actions.publishInputs = append(p.actions.publishInputs, input)
	return workbookops.PublishOutput{}, nil
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
