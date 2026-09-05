package unregister_test

import (
	"context"
	unregister "github.com/ahillspace/tadx/actions/workspace/unregister"
	"testing"
)

type registry struct{}

func (registry) Unregister(context.Context, string) (unregister.Workspace, error) {
	return unregister.Workspace{Name: "dev", ID: "ws_1", Root: "root"}, nil
}
func TestExecutePreservesFiles(t *testing.T) {
	out, err := unregister.New(registry{}).Execute(context.Background(), unregister.Input{Name: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if !out.FilesPreserved || out.Status != "unregistered" {
		t.Fatalf("output = %#v", out)
	}
}
