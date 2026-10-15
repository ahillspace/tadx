package workspace_test

import (
	"context"
	"testing"

	workspaceaction "github.com/ahillspace/tadx/actions/workspace"
	"github.com/ahillspace/tadx/internal/commandhint"
)

type unregisterRegistry struct{}

func (unregisterRegistry) Unregister(context.Context, workspaceaction.UnregisterInput) (workspaceaction.Workspace, error) {
	return workspaceaction.Workspace{Name: "dev", ID: "ws_1", Root: "root"}, nil
}

type invalidNameRegistry struct{ name string }

func (r invalidNameRegistry) Unregister(context.Context, workspaceaction.UnregisterInput) (workspaceaction.Workspace, error) {
	return workspaceaction.Workspace{Name: r.name, Status: "invalid", Violations: []string{"name"}}, nil
}

func TestUnregisterInvalidNameUsesReplacementName(t *testing.T) {
	for _, name := range []string{"", " ", "invalid/name", "CaseCollision"} {
		t.Run(name, func(t *testing.T) {
			service := workspaceaction.Service{Registry: invalidNameRegistry{name: name}}
			out, err := service.Unregister(t.Context(), workspaceaction.UnregisterInput{Name: name, NameSet: true})
			if err != nil || len(out.Help) != 1 || out.Help[0] != commandhint.Command("workspace", "register", "--path", "<path>", "<name>") {
				t.Fatalf("invalid name recovery repeats an unusable name: output=%+v err=%v", out, err)
			}
		})
	}
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
