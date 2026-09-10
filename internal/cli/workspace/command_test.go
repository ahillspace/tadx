package workspace_test

import (
	"bytes"
	"context"
	"reflect"
	"strings"
	"testing"

	artifactdelete "github.com/ahillspace/tadx/actions/workspace/artifact/delete"
	workspaceclean "github.com/ahillspace/tadx/actions/workspace/clean"
	workspaceclone "github.com/ahillspace/tadx/actions/workspace/clone"
	workspacecreate "github.com/ahillspace/tadx/actions/workspace/create"
	workspacedelete "github.com/ahillspace/tadx/actions/workspace/delete"
	workspacelist "github.com/ahillspace/tadx/actions/workspace/list"
	workspacemove "github.com/ahillspace/tadx/actions/workspace/move"
	workspacesetdefault "github.com/ahillspace/tadx/actions/workspace/setdefault"
	workspacestatus "github.com/ahillspace/tadx/actions/workspace/status"
	workspaceunregister "github.com/ahillspace/tadx/actions/workspace/unregister"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
)

type actions struct {
	create          []workspacecreate.Input
	clone           []workspaceclone.Input
	list            []workspacelist.Input
	status          []workspacestatus.Input
	move            []workspacemove.Input
	delete          []artifactdelete.Input
	clean           []workspaceclean.Input
	preview         []bool
	setDefault      []workspacesetdefault.Input
	unregister      []workspaceunregister.Input
	deleteWorkspace []workspacedelete.Input
}

func (a *actions) Create(_ context.Context, input workspacecreate.Input) (workspacecreate.Output, error) {
	a.create = append(a.create, input)
	return workspacecreate.Output{}, nil
}
func (a *actions) Clone(_ context.Context, input workspaceclone.Input) (workspaceclone.Output, error) {
	a.clone = append(a.clone, input)
	return workspaceclone.Output{}, nil
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
func (a *actions) Delete(_ context.Context, input artifactdelete.Input, preview bool) (artifactdelete.Output, error) {
	a.delete = append(a.delete, input)
	a.preview = append(a.preview, preview)
	return artifactdelete.Output{}, nil
}
func (a *actions) Clean(_ context.Context, input workspaceclean.Input) (workspaceclean.Output, error) {
	a.clean = append(a.clean, input)
	return workspaceclean.Output{}, nil
}
func (a *actions) SetDefault(_ context.Context, input workspacesetdefault.Input) (workspacesetdefault.Output, error) {
	a.setDefault = append(a.setDefault, input)
	return workspacesetdefault.Output{}, nil
}
func (a *actions) Unregister(_ context.Context, input workspaceunregister.Input) (workspaceunregister.Output, error) {
	a.unregister = append(a.unregister, input)
	return workspaceunregister.Output{}, nil
}
func (a *actions) DeleteWorkspace(_ context.Context, input workspacedelete.Input, preview bool) (workspacedelete.Output, error) {
	a.deleteWorkspace = append(a.deleteWorkspace, input)
	a.preview = append(a.preview, preview)
	return workspacedelete.Output{}, nil
}

type renderer struct{ calls int }

func (r *renderer) Render(any) error { r.calls++; return nil }

func TestWorkspaceCommandsMapExactInputs(t *testing.T) {
	a := &actions{}
	r := &renderer{}
	command := workspacecli.New(workspacecli.Dependencies{Creator: a, Lister: a, Statuser: a, DefaultSetter: a, Unregistrar: a, WorkspaceDeleter: a, Mover: a, Deleter: a, Cleaner: a, Renderer: r})
	commands := [][]string{
		{"create", "development"},
		{"list", "--limit", "5", "--cursor", "10"},
		{"status", "--workspace", "development", "--limit", "7", "--cursor", "3"},
		{"set-default", "development"},
		{"unregister", "old"},
		{"delete", "throwaway", "--force", "--preview"},
		{"artifact", "move", "--source", "development", "--destination", "archive", "--kind", "workbook", "--id", "wb-1"},
		{"artifact", "delete", "--workspace", "archive", "--artifact", "artifacts/workbook/Finance", "--force", "--preview"},
		{"clean", "--workspace", "archive", "--class", "temporary"},
	}
	for _, args := range commands {
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
	}
	if !reflect.DeepEqual(a.create, []workspacecreate.Input{{Name: "development"}}) {
		t.Fatalf("create = %#v", a.create)
	}
	if !reflect.DeepEqual(a.list, []workspacelist.Input{{Limit: 5, Cursor: "10"}}) {
		t.Fatalf("list = %#v", a.list)
	}
	if !reflect.DeepEqual(a.status, []workspacestatus.Input{{Workspace: "development", Limit: 7, Cursor: "3"}}) {
		t.Fatalf("status = %#v", a.status)
	}
	if !reflect.DeepEqual(a.setDefault, []workspacesetdefault.Input{{Name: "development"}}) {
		t.Fatalf("set default = %#v", a.setDefault)
	}
	if !reflect.DeepEqual(a.unregister, []workspaceunregister.Input{{Name: "old"}}) {
		t.Fatalf("unregister = %#v", a.unregister)
	}
	if !reflect.DeepEqual(a.deleteWorkspace, []workspacedelete.Input{{Name: "throwaway", Force: true}}) {
		t.Fatalf("delete workspace = %#v", a.deleteWorkspace)
	}
	if !reflect.DeepEqual(a.move, []workspacemove.Input{{SourceWorkspace: "development", DestinationWorkspace: "archive", Kind: "workbook", LUID: "wb-1"}}) {
		t.Fatalf("move = %#v", a.move)
	}
	if !reflect.DeepEqual(a.delete, []artifactdelete.Input{{Workspace: "archive", Path: "artifacts/workbook/Finance", Force: true}}) || !reflect.DeepEqual(a.preview, []bool{true, true}) {
		t.Fatalf("delete = %#v preview = %#v", a.delete, a.preview)
	}
	if !reflect.DeepEqual(a.clean, []workspaceclean.Input{{Workspace: "archive", Class: "temporary"}}) {
		t.Fatalf("clean = %#v", a.clean)
	}
	if r.calls != len(commands) {
		t.Fatalf("render calls = %d", r.calls)
	}
}

func TestWorkspaceRegistryUsePreservesPositionalArguments(t *testing.T) {
	command := workspacecli.New(workspacecli.Dependencies{Uses: map[string]string{
		"workspace.create":      "create",
		"workspace.set-default": "set-default",
		"workspace.unregister":  "unregister",
		"workspace.delete":      "delete",
	}})
	for _, test := range []struct {
		path string
		want string
	}{
		{path: "create", want: "create <name>"},
		{path: "set-default", want: "set-default <name>"},
		{path: "unregister", want: "unregister <name>"},
		{path: "delete", want: "delete <name>"},
	} {
		found, _, err := command.Find([]string{test.path})
		if err != nil || found.Use != test.want {
			t.Fatalf("Find(%s) use = %q, error = %v, want %q", test.path, found.Use, err, test.want)
		}
	}
}

func TestWorkspaceCreateAndClonePreserveExplicitPaths(t *testing.T) {
	a := &actions{}
	r := &renderer{}
	command := workspacecli.New(workspacecli.Dependencies{Creator: a, Cloner: a, Lister: a, Statuser: a, Mover: a, Deleter: a, Cleaner: a, Renderer: r})
	command.SetArgs([]string{"create", "development", "--path", "relative/workspace"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.create, []workspacecreate.Input{{Name: "development", Path: "relative/workspace"}}) {
		t.Fatalf("create = %#v", a.create)
	}
	command.SetArgs([]string{"clone", "development", "--name", "experiment"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.clone, []workspaceclone.Input{{Source: "development", Name: "experiment"}}) {
		t.Fatalf("clone = %#v", a.clone)
	}
}

func TestWorkspaceCreateHelpDefinesPathAsNewOrEmptyRoot(t *testing.T) {
	var stdout bytes.Buffer
	command := workspacecli.New(workspacecli.Dependencies{Creator: &actions{}, Renderer: &renderer{}})
	command.SetOut(&stdout)
	command.SetArgs([]string{"create", "--help"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(stdout.String(), "new workspace root or existing empty real directory") {
		t.Fatalf("help = %q", stdout.String())
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

func TestWorkspaceArtifactMoveIsCanonicalAndLegacyAliasIsHidden(t *testing.T) {
	command := workspacecli.New(workspacecli.Dependencies{Mover: &actions{}, Deleter: &actions{}, Renderer: &renderer{}})
	artifactMove, _, err := command.Find([]string{"artifact", "move"})
	if err != nil {
		t.Fatal(err)
	}
	if artifactMove.Hidden {
		t.Fatal("canonical artifact move command is hidden")
	}
	if got := artifactMove.Annotations["tadx.capability"]; got != "workspace.move" {
		t.Fatalf("capability annotation = %q", got)
	}
	legacyMove, _, err := command.Find([]string{"move"})
	if err != nil {
		t.Fatal(err)
	}
	if !legacyMove.Hidden {
		t.Fatal("legacy workspace move alias is visible")
	}
}
