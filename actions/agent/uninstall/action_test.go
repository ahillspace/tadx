package uninstall_test

import (
	"context"
	uninstall "github.com/ahillspace/tadx/actions/agent/uninstall"
	"testing"
)

type service struct{ called bool }

func (s *service) Uninstall(context.Context, uninstall.Input) (uninstall.Result, error) {
	s.called = true
	return uninstall.Result{Status: "uninstalled", Skills: []uninstall.Skill{{Name: "tadx", Status: "removed"}}}, nil
}
func TestExecute(t *testing.T) {
	s := &service{}
	out, err := uninstall.New(s).Execute(context.Background(), uninstall.Input{Target: "codex"})
	if err != nil {
		t.Fatal(err)
	}
	if !s.called || out.Status != "uninstalled" {
		t.Fatalf("out=%#v called=%t", out, s.called)
	}
}
