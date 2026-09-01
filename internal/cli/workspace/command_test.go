package workspace_test

import (
	"context"
	"reflect"
	"testing"

	artifactdelete "github.com/ahillspace/tadx/actions/workspace/artifact/delete"
	workspacecreate "github.com/ahillspace/tadx/actions/workspace/create"
	workspacelist "github.com/ahillspace/tadx/actions/workspace/list"
	workspacemove "github.com/ahillspace/tadx/actions/workspace/move"
	workspacestatus "github.com/ahillspace/tadx/actions/workspace/status"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
)

type actions struct {
	create []workspacecreate.Input
	list   []workspacelist.Input
	status []workspacestatus.Input
	move   []workspacemove.Input
	delete []artifactdelete.Input
	apply  []bool
}

func (a *actions) Create(_ context.Context, input workspacecreate.Input) (workspacecreate.Output, error) {
	a.create = append(a.create, input)
	return workspacecreate.Output{}, nil
}
func (a *actions) List(_ context.Context, input workspacelist.Input) (workspacelist.Output, error) {
	a.list = append(a.list, input)
	return workspacelist.Output{}, nil
}
func (a *actions) Status(_ context.Context, input workspacestatus.Input) (workspacestatus.Output, error) {
	a.status = append(a.status, input)
	return workspacestatus.Output{}, nil
}
func (a *actions) Move(_ context.Context, input workspacemove.Input) (workspacemove.Output, error) {
	a.move = append(a.move, input)
	return workspacemove.Output{}, nil
}
func (a *actions) Delete(_ context.Context, input artifactdelete.Input, apply bool) (artifactdelete.Output, error) {
	a.delete = append(a.delete, input)
	a.apply = append(a.apply, apply)
	return artifactdelete.Output{}, nil
}

type renderer struct{ calls int }

func (r *renderer) Render(any) error { r.calls++; return nil }

func TestWorkspaceCommandsMapExactInputs(t *testing.T) {
	a := &actions{}
	r := &renderer{}
	command := workspacecli.New(workspacecli.Dependencies{Creator: a, Lister: a, Statuser: a, Mover: a, Deleter: a, Renderer: r})
	commands := [][]string{
		{"create", "development", "--path", "relative/workspace"},
		{"list", "--limit", "5", "--cursor", "10"},
		{"status", "--workspace", "development", "--limit", "7", "--cursor", "3"},
		{"move", "--source", "development", "--destination", "archive", "--kind", "workbook", "--id", "wb-1"},
		{"artifact", "delete", "--workspace", "archive", "--artifact", "artifacts/workbook/Finance", "--force", "--apply"},
	}
	for _, args := range commands {
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
	}
	if !reflect.DeepEqual(a.create, []workspacecreate.Input{{Name: "development", Path: "relative/workspace"}}) {
		t.Fatalf("create = %#v", a.create)
	}
	if !reflect.DeepEqual(a.list, []workspacelist.Input{{Limit: 5, Cursor: "10"}}) {
		t.Fatalf("list = %#v", a.list)
	}
	if !reflect.DeepEqual(a.status, []workspacestatus.Input{{Workspace: "development", Limit: 7, Cursor: "3"}}) {
		t.Fatalf("status = %#v", a.status)
	}
	if !reflect.DeepEqual(a.move, []workspacemove.Input{{SourceWorkspace: "development", DestinationWorkspace: "archive", Kind: "workbook", LUID: "wb-1"}}) {
		t.Fatalf("move = %#v", a.move)
	}
	if !reflect.DeepEqual(a.delete, []artifactdelete.Input{{Workspace: "archive", Path: "artifacts/workbook/Finance", Force: true}}) || !reflect.DeepEqual(a.apply, []bool{true}) {
		t.Fatalf("delete = %#v apply = %#v", a.delete, a.apply)
	}
	if r.calls != len(commands) {
		t.Fatalf("render calls = %d", r.calls)
	}
}

func TestWorkspaceArtifactSelectorsRejectAbsolutePathsAndPartialIdentity(t *testing.T) {
	for _, args := range [][]string{
		{"move", "--source", "development", "--destination", "archive", "--artifact", `C:\artifacts\Finance`},
		{"artifact", "delete", "--workspace", "archive", "--kind", "workbook"},
	} {
		command := workspacecli.New(workspacecli.Dependencies{Creator: &actions{}, Lister: &actions{}, Statuser: &actions{}, Mover: &actions{}, Deleter: &actions{}, Renderer: &renderer{}})
		command.SetArgs(args)
		if err := command.Execute(); err == nil {
			t.Fatalf("Execute(%v) error = nil", args)
		}
	}
}

func TestWorkspaceArtifactDeleteIsVisibleWithoutRemoteMutationDiscovery(t *testing.T) {
	command := workspacecli.New(workspacecli.Dependencies{Creator: &actions{}, Lister: &actions{}, Statuser: &actions{}, Mover: &actions{}, Deleter: &actions{}, Renderer: &renderer{}})
	artifactCommand, _, err := command.Find([]string{"artifact", "delete"})
	if err != nil {
		t.Fatal(err)
	}
	if artifactCommand.Hidden {
		t.Fatal("local artifact delete was hidden by remote mutation discovery")
	}
}
