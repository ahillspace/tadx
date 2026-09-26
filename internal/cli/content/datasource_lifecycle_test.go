package content

import (
	"context"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"testing"

	"github.com/spf13/cobra"
)

type datasourceLifecycleCommands struct {
	pullInput      datasourceops.PullInput
	publishInput   datasourceops.PublishInput
	publishPreview bool
	deleteInput    datasourceops.DeleteInput
	deletePreview  bool
}

func (c *datasourceLifecycleCommands) PullDatasource(_ context.Context, input datasourceops.PullInput) (datasourceops.PullOutput, error) {
	c.pullInput = input
	return datasourceops.PullOutput{}, nil
}
func (c *datasourceLifecycleCommands) PublishDatasource(_ context.Context, input datasourceops.PublishInput, preview bool) (datasourceops.PublishOutput, error) {
	c.publishInput, c.publishPreview = input, preview
	return datasourceops.PublishOutput{}, nil
}
func (c *datasourceLifecycleCommands) DeleteDatasource(_ context.Context, input datasourceops.DeleteInput, preview bool) (datasourceops.DeleteOutput, error) {
	c.deleteInput, c.deletePreview = input, preview
	return datasourceops.DeleteOutput{}, nil
}

type datasourceLifecycleRenderer struct{ calls int }

func (r *datasourceLifecycleRenderer) Render(any) error { r.calls++; return nil }

func datasourceLifecycleRoot(actions *datasourceLifecycleCommands, renderer Renderer, mutations bool) *cobra.Command {
	root := &cobra.Command{Use: "datasource"}
	addDatasourceLifecycle(root, datasourceLifecycleDependencies{puller: actions, publisher: actions, deleter: actions, renderer: renderer, mutationsEnabled: mutations})
	return root
}

func TestDatasourcePullParsesLogicalWorkspaceAndExactSelector(t *testing.T) {
	actions := &datasourceLifecycleCommands{}
	renderer := &datasourceLifecycleRenderer{}
	command := datasourceLifecycleRoot(actions, renderer, true)
	command.SetArgs([]string{"pull", "--environment", "dev", "--workspace", "analytics", "--name", "Sales", "--project", "Department/Ops", "--overwrite"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.pullInput.Environment != "dev" || actions.pullInput.Workspace != "analytics" || actions.pullInput.Selector.Name != "Sales" || actions.pullInput.Selector.ProjectPath != "Department/Ops" || !actions.pullInput.Overwrite || renderer.calls != 1 {
		t.Fatalf("input = %#v, renders = %d", actions.pullInput, renderer.calls)
	}
}

func TestDatasourcePublishKeepsSourceIndependentOfDestination(t *testing.T) {
	actions := &datasourceLifecycleCommands{}
	renderer := &datasourceLifecycleRenderer{}
	command := datasourceLifecycleRoot(actions, renderer, true)
	command.SetArgs([]string{"publish", "--workspace", "analytics", "--artifact", "artifacts/datasource/Sales", "--project-id", "project-1", "--overwrite"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.publishInput.SourceDefaulted || actions.publishInput.Mode != datasourceops.ModeOverwrite || actions.publishInput.Workspace != "analytics" || actions.publishInput.ArtifactPath != "artifacts/datasource/Sales" || actions.publishPreview || renderer.calls != 1 {
		t.Fatalf("input = %#v, preview = %t, renders = %d", actions.publishInput, actions.publishPreview, renderer.calls)
	}
}

func TestDatasourcePublishParsesExplicitDestinationModeAndPreview(t *testing.T) {
	actions := &datasourceLifecycleCommands{}
	command := datasourceLifecycleRoot(actions, &datasourceLifecycleRenderer{}, true)
	command.SetArgs([]string{"publish", "--artifact", "artifacts/datasource/Sales", "--environment", "prod", "--project-id", "project-1", "--create", "--preview"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.publishInput.SourceDefaulted || actions.publishInput.Environment != "prod" || actions.publishInput.ProjectSelector.LUID != "project-1" || actions.publishInput.Mode != datasourceops.ModeCreate || !actions.publishPreview {
		t.Fatalf("input = %#v, preview = %t", actions.publishInput, actions.publishPreview)
	}
}

func TestDatasourcePublishMapsEveryExplicitModeWithoutInference(t *testing.T) {
	tests := []struct {
		flag string
		mode datasourceops.Mode
	}{{"--create", datasourceops.ModeCreate}, {"--overwrite", datasourceops.ModeOverwrite}, {"--append", datasourceops.ModeAppend}, {"--replace", datasourceops.ModeReplace}}
	for _, test := range tests {
		actions := &datasourceLifecycleCommands{}
		command := datasourceLifecycleRoot(actions, &datasourceLifecycleRenderer{}, true)
		command.SetArgs([]string{"publish", "--artifact", "artifacts/datasource/Sales", "--project-id", "project-1", test.flag})
		if err := command.ExecuteContext(context.Background()); err != nil {
			t.Fatalf("%s: %v", test.flag, err)
		}
		if actions.publishInput.Mode != test.mode {
			t.Fatalf("%s mode = %q", test.flag, actions.publishInput.Mode)
		}
	}
}

func TestDatasourcePublishRejectsUnsafeTargetAndModeCombinations(t *testing.T) {
	tests := [][]string{
		{"publish", "--artifact", "artifacts/datasource/Sales"},
		{"publish", "--artifact", "artifacts/datasource/Sales", "--create", "--overwrite"},
		{"publish", "--artifact", "artifacts/datasource/Sales", "--environment", "prod", "--create"},
		{"publish", "--artifact", "artifacts/datasource/Sales", "--project", "Analytics", "--project-id", "project-1", "--create"},
		{"publish", "--artifact", `C:\workspace\artifacts\datasource\Sales`, "--create"},
	}
	for _, args := range tests {
		command := datasourceLifecycleRoot(&datasourceLifecycleCommands{}, &datasourceLifecycleRenderer{}, true)
		command.SetArgs(args)
		if err := command.ExecuteContext(context.Background()); err == nil {
			t.Fatalf("args %v succeeded", args)
		}
	}
}

func TestDatasourceDeleteRequiresExplicitEnvironmentAndParsesPreview(t *testing.T) {
	actions := &datasourceLifecycleCommands{}
	missing := datasourceLifecycleRoot(actions, &datasourceLifecycleRenderer{}, true)
	missing.SetArgs([]string{"delete", "--id", "ds-1"})
	if err := missing.ExecuteContext(context.Background()); err == nil {
		t.Fatal("delete without environment succeeded")
	}
	command := datasourceLifecycleRoot(actions, &datasourceLifecycleRenderer{}, true)
	command.SetArgs([]string{"delete", "--environment", "prod", "--id", "ds-1", "--preview"})
	if err := command.ExecuteContext(context.Background()); err != nil {
		t.Fatal(err)
	}
	if actions.deleteInput.Environment != "prod" || actions.deleteInput.Selector.LUID != "ds-1" || !actions.deletePreview {
		t.Fatalf("input = %#v, preview = %t", actions.deleteInput, actions.deletePreview)
	}
}

func TestDatasourceMutationDiscoveryStaysVisibleWhenDisabled(t *testing.T) {
	command := datasourceLifecycleRoot(&datasourceLifecycleCommands{}, &datasourceLifecycleRenderer{}, false)
	publish, _, err := command.Find([]string{"publish"})
	if err != nil {
		t.Fatal(err)
	}
	deleteCommand, _, err := command.Find([]string{"delete"})
	if err != nil {
		t.Fatal(err)
	}
	pull, _, err := command.Find([]string{"pull"})
	if err != nil {
		t.Fatal(err)
	}
	if publish.Hidden || deleteCommand.Hidden || pull.Hidden {
		t.Fatalf("hidden states: pull=%t publish=%t delete=%t", pull.Hidden, publish.Hidden, deleteCommand.Hidden)
	}
}
