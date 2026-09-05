package setdefault_test

import (
	"context"
	"testing"

	setdefault "github.com/ahillspace/tadx/actions/workspace/setdefault"
)

type setter struct{ name string }

func (s *setter) SetDefault(_ context.Context, name string) (setdefault.Workspace, error) {
	s.name = name
	return setdefault.Workspace{Name: "development", ID: "ws_1", Root: "root"}, nil
}

func TestExecuteSelectsExactWorkspace(t *testing.T) {
	s := &setter{}
	out, err := setdefault.New(s).Execute(context.Background(), setdefault.Input{Name: "development"})
	if err != nil {
		t.Fatal(err)
	}
	if s.name != "development" || out.Status != "default-set" || out.Workspace.ID != "ws_1" {
		t.Fatalf("output = %#v name = %q", out, s.name)
	}
}
