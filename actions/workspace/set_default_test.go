package workspace_test

import (
	"context"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
)

type setDefaultSetter struct{ name string }

func (s *setDefaultSetter) SetDefault(_ context.Context, input workspaceaction.SetDefaultInput) (workspaceaction.Workspace, error) {
	s.name = input.Name
	return workspaceaction.Workspace{Name: "development", ID: "ws_1", Root: "root"}, nil
}

func TestSetDefaultExecuteSelectsExactWorkspace(t *testing.T) {
	s := &setDefaultSetter{}
	out, err := (&workspaceaction.Service{Setter: s}).SetDefault(t.Context(), workspaceaction.SetDefaultInput{Name: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if s.name != "development" || out.Status != "default-set" || out.Workspace.ID != "ws_1" {
		t.Fatalf("output = %#v name = %q", out, s.name)
	}
}
