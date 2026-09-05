package delete_test

import (
	"context"
	workspacedelete "github.com/ahillspace/tadx/actions/workspace/delete"
	"testing"
)

type store struct {
	deleted bool
	dirty   bool
}

func (s *store) Resolve(context.Context, string) (workspacedelete.Workspace, error) {
	return workspacedelete.Workspace{Name: "dev", ID: "ws_1", Root: "root", Dirty: s.dirty}, nil
}
func (s *store) Delete(context.Context, workspacedelete.DeleteRequest) error {
	s.deleted = true
	return nil
}
func TestPreviewDoesNotDelete(t *testing.T) {
	s := &store{}
	out, err := workspacedelete.New(s).Execute(context.Background(), workspacedelete.Input{Name: "dev"}, true)
	if err != nil {
		t.Fatal(err)
	}
	if s.deleted || out.Result != nil {
		t.Fatalf("output=%#v deleted=%t", out, s.deleted)
	}
}
func TestDirtyDeleteRequiresForce(t *testing.T) {
	s := &store{dirty: true}
	if _, err := workspacedelete.New(s).Execute(context.Background(), workspacedelete.Input{Name: "dev"}, false); err == nil {
		t.Fatal("dirty delete succeeded")
	}
	if _, err := workspacedelete.New(s).Execute(context.Background(), workspacedelete.Input{Name: "dev", Force: true}, false); err != nil {
		t.Fatal(err)
	}
}
