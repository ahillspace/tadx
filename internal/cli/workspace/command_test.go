package workspace_test

import (
	"bytes"
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	workspacecli "github.com/ahillspace/tadx/internal/cli/workspace"
	"github.com/ahillspace/tadx/internal/errs"
)

type actions struct {
	create          []workspaceaction.CreateInput
	clone           []workspaceaction.CloneInput
	list            []workspaceaction.ListInput
	status          []workspaceaction.StatusInput
	move            []workspaceaction.MoveInput
	delete          []workspaceaction.DeleteArtifactInput
	clean           []workspaceaction.CleanInput
	preview         []bool
	setDefault      []workspaceaction.SetDefaultInput
	unregister      []workspaceaction.UnregisterInput
	deleteWorkspace []workspaceaction.DeleteWorkspaceInput
}

func (a *actions) Create(_ context.Context, input workspaceaction.CreateInput) (workspaceaction.CreateOutput, error) {
	a.create = append(a.create, input)
	return workspaceaction.CreateOutput{}, nil
}
func (a *actions) Clone(_ context.Context, input workspaceaction.CloneInput) (workspaceaction.CloneOutput, error) {
	a.clone = append(a.clone, input)
	return workspaceaction.CloneOutput{}, nil
}
func (a *actions) List(_ context.Context, input workspaceaction.ListInput) (workspaceaction.ListOutput, error) {
	a.list = append(a.list, input)
	return workspaceaction.ListOutput{}, nil
}
func (a *actions) Status(_ context.Context, input workspaceaction.StatusInput) (workspaceaction.StatusOutput, error) {
	a.status = append(a.status, input)
	return workspaceaction.StatusOutput{}, nil
}
func (a *actions) Move(_ context.Context, input workspaceaction.MoveInput) (workspaceaction.MoveOutput, error) {
	a.move = append(a.move, input)
	return workspaceaction.MoveOutput{}, nil
}
func (a *actions) DeleteArtifact(_ context.Context, input workspaceaction.DeleteArtifactInput, preview bool) (workspaceaction.DeleteArtifactOutput, error) {
	a.delete = append(a.delete, input)
	a.preview = append(a.preview, preview)
	return workspaceaction.DeleteArtifactOutput{}, nil
}
func (a *actions) Clean(_ context.Context, input workspaceaction.CleanInput) (workspaceaction.CleanOutput, error) {
	a.clean = append(a.clean, input)
	return workspaceaction.CleanOutput{}, nil
}
func (a *actions) SetDefault(_ context.Context, input workspaceaction.SetDefaultInput) (workspaceaction.SetDefaultOutput, error) {
	a.setDefault = append(a.setDefault, input)
	return workspaceaction.SetDefaultOutput{}, nil
}
func (a *actions) Unregister(_ context.Context, input workspaceaction.UnregisterInput) (workspaceaction.UnregisterOutput, error) {
	a.unregister = append(a.unregister, input)
	return workspaceaction.UnregisterOutput{}, nil
}
func (a *actions) DeleteWorkspace(_ context.Context, input workspaceaction.DeleteWorkspaceInput, preview bool) (workspaceaction.DeleteWorkspaceOutput, error) {
	a.deleteWorkspace = append(a.deleteWorkspace, input)
	a.preview = append(a.preview, preview)
	return workspaceaction.DeleteWorkspaceOutput{}, nil
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
	if !reflect.DeepEqual(a.create, []workspaceaction.CreateInput{{Name: "development"}}) {
		t.Fatalf("create = %#v", a.create)
	}
	if !reflect.DeepEqual(a.list, []workspaceaction.ListInput{{Limit: 5, Cursor: "10"}}) {
		t.Fatalf("list = %#v", a.list)
	}
	if !reflect.DeepEqual(a.status, []workspaceaction.StatusInput{{Workspace: "development", Limit: 7, Cursor: "3"}}) {
		t.Fatalf("status = %#v", a.status)
	}
	if !reflect.DeepEqual(a.setDefault, []workspaceaction.SetDefaultInput{{Name: "development"}}) {
		t.Fatalf("set default = %#v", a.setDefault)
	}
	if !reflect.DeepEqual(a.unregister, []workspaceaction.UnregisterInput{{Name: "old"}}) {
		t.Fatalf("unregister = %#v", a.unregister)
	}
	if !reflect.DeepEqual(a.deleteWorkspace, []workspaceaction.DeleteWorkspaceInput{{Name: "throwaway", Force: true}}) {
		t.Fatalf("delete workspace = %#v", a.deleteWorkspace)
	}
	if !reflect.DeepEqual(a.move, []workspaceaction.MoveInput{{SourceWorkspace: "development", DestinationWorkspace: "archive", Kind: "workbook", LUID: "wb-1"}}) {
		t.Fatalf("move = %#v", a.move)
	}
	if !reflect.DeepEqual(a.delete, []workspaceaction.DeleteArtifactInput{{Workspace: "archive", Path: "artifacts/workbook/Finance", Force: true}}) || !reflect.DeepEqual(a.preview, []bool{true, true}) {
		t.Fatalf("delete = %#v preview = %#v", a.delete, a.preview)
	}
	if !reflect.DeepEqual(a.clean, []workspaceaction.CleanInput{{Workspace: "archive", Class: "temporary"}}) {
		t.Fatalf("clean = %#v", a.clean)
	}
	if r.calls != len(commands) {
		t.Fatalf("render calls = %d", r.calls)
	}
}

func TestWorkspaceListAndStatusAllCarryCompleteInventoryMode(t *testing.T) {
	a := &actions{}
	command := workspacecli.New(workspacecli.Dependencies{Lister: a, Statuser: a, Renderer: &renderer{}})
	command.SetArgs([]string{"list", "--all"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	command.SetArgs([]string{"status", "--all", "--workspace", "development"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.list, []workspaceaction.ListInput{{All: true}}) || !reflect.DeepEqual(a.status, []workspaceaction.StatusInput{{All: true, Workspace: "development"}}) {
		t.Fatalf("list=%#v status=%#v", a.list, a.status)
	}
}

func TestWorkspaceAllRejectsLimitAndCursor(t *testing.T) {
	for _, args := range [][]string{{"list", "--all", "--limit", "1"}, {"list", "--all", "--cursor", "0"}, {"status", "--all", "--limit", "1"}, {"status", "--all", "--cursor", "0"}} {
		a := &actions{}
		command := workspacecli.New(workspacecli.Dependencies{Lister: a, Statuser: a, Renderer: &renderer{}})
		command.SetArgs(args)
		if err := command.Execute(); err == nil || len(a.list) != 0 || len(a.status) != 0 {
			t.Fatalf("args=%v error=%v list_calls=%d status_calls=%d", args, err, len(a.list), len(a.status))
		}
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
	if !reflect.DeepEqual(a.create, []workspaceaction.CreateInput{{Name: "development", Path: "relative/workspace"}}) {
		t.Fatalf("create = %#v", a.create)
	}
	command.SetArgs([]string{"clone", "development", "--name", "experiment"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(a.clone, []workspaceaction.CloneInput{{Source: "development", Name: "experiment"}}) {
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

func TestWorkspaceMoveAliasDispatchesTheSameInput(t *testing.T) {
	for _, prefix := range [][]string{{"move"}, {"artifact", "move"}} {
		a := &actions{}
		command := workspacecli.New(workspacecli.Dependencies{Mover: a, Renderer: &renderer{}})
		command.SetArgs(append(prefix, "--source", "source", "--destination", "destination", "--artifact", "artifacts/workbook/Sales", "--preview"))
		if err := command.Execute(); err != nil {
			t.Fatal(err)
		}
		want := []workspaceaction.MoveInput{{SourceWorkspace: "source", DestinationWorkspace: "destination", Path: "artifacts/workbook/Sales", Preview: true}}
		if !reflect.DeepEqual(a.move, want) {
			t.Fatalf("%v input = %#v", prefix, a.move)
		}
	}
}

func TestWorkspaceValidationErrorsPrecedeDispatch(t *testing.T) {
	for _, test := range []struct {
		args []string
		want string
	}{
		{[]string{"register"}, "--path is required"},
		{[]string{"clone", "source"}, "--name is required"},
		{[]string{"artifact", "move", "--artifact", "../escape"}, "--source and --destination are required"},
		{[]string{"move", "--source", "source", "--destination", "destination", "--artifact", "../escape"}, "--artifact must be a workspace-relative managed path"},
		{[]string{"artifact", "delete", "--artifact", "../escape"}, "--workspace is required"},
		{[]string{"artifact", "delete", "--workspace", "source", "--artifact", "artifacts/workbook/Sales", "--kind", "workbook"}, "use either --artifact or both --kind and --id"},
	} {
		a := &actions{}
		command := workspacecli.New(workspacecli.Dependencies{Mover: a, Deleter: a, Renderer: &renderer{}})
		command.SetArgs(test.args)
		err := command.Execute()
		structured, ok := errors.AsType[*errs.Error](err)
		if !ok || structured.Kind != errs.KindUsage || errs.ExitCode(err) != 2 {
			t.Fatalf("%v: usage error = %#v", test.args, err)
		}
		if err == nil || !strings.Contains(err.Error(), test.want) || len(a.move) != 0 || len(a.delete) != 0 {
			t.Fatalf("%v: error=%v move=%v delete=%v", test.args, err, a.move, a.delete)
		}
	}
}
