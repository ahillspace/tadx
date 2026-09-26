package workspace_test

import (
	"context"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
)

type unregisterRegistry struct{}

func (unregisterRegistry) Unregister(context.Context, workspaceaction.UnregisterInput) (workspaceaction.Workspace, error) {
	return workspaceaction.Workspace{Name: "dev", ID: "ws_1", Root: "root"}, nil
}
func TestUnregisterExecutePreservesFiles(t *testing.T) {
	out, err := (&workspaceaction.Service{Registry: unregisterRegistry{}}).Unregister(t.Context(), workspaceaction.UnregisterInput{Name: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.FilesPreserved || out.Status != "unregistered" {
		t.Fatalf("output = %#v", out)
	}
}
