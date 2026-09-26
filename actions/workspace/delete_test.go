package workspace_test

import (
	"context"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
)

type deleteWorkspaceStore struct {
	deleted bool
	dirty   bool
}

func (s *deleteWorkspaceStore) ResolveWorkspace(context.Context, string) (workspaceaction.DeletionTarget, error) {
	return workspaceaction.DeletionTarget{Workspace: workspaceaction.Workspace{Name: "dev", ID: "ws_1", Root: "root"}, Dirty: s.dirty}, nil
}
func (s *deleteWorkspaceStore) DeleteWorkspace(context.Context, workspaceaction.WorkspaceDeleteRequest) error {
	s.deleted = true
	return nil
}
func TestDeleteWorkspacePreviewDoesNotDelete(t *testing.T) {
	s := &deleteWorkspaceStore{}
	out, err := (&workspaceaction.Service{WorkspaceStore: s}).DeleteWorkspace(t.Context(), workspaceaction.DeleteWorkspaceInput{Name: "dev"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if s.deleted || out.Result != nil {
		t.Fatalf("output=%#v deleted=%t", out, s.deleted)
	}
}
func TestDeleteWorkspaceDirtyDeleteRequiresForce(t *testing.T) {
	s := &deleteWorkspaceStore{dirty: true}
	if _, err := (&workspaceaction.Service{WorkspaceStore: s}).DeleteWorkspace(t.Context(), workspaceaction.DeleteWorkspaceInput{Name: "dev"}, false); err == nil {
		t.Fatal("dirty delete succeeded")
	}
	if _, err := (&workspaceaction.Service{WorkspaceStore: s}).DeleteWorkspace(t.Context(), workspaceaction.DeleteWorkspaceInput{Name: "dev", Force: true}, false); err != nil {
		t.Fatal(err)
	}
}
