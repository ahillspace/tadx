package auth_test

import (
	"context"
	"testing"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
)

type checker struct{ inputs []authcheck.Input }

func (c *checker) Execute(_ context.Context, input authcheck.Input) (authcheck.Output, error) {
	c.inputs = append(c.inputs, input)
	return authcheck.Output{}, nil
}

type statuser struct{ inputs []authstatus.Input }

func (s *statuser) Execute(_ context.Context, input authstatus.Input) (authstatus.Output, error) {
	s.inputs = append(s.inputs, input)
	return authstatus.Output{}, nil
}

type renderer struct{ calls int }

func (r *renderer) Render(any) error { r.calls++; return nil }

func TestAuthCheckAndStatusMapEnvironment(t *testing.T) {
	c := &checker{}
	s := &statuser{}
	r := &renderer{}
	command := authcli.New(authcli.Dependencies{Checker: c, Statuser: s, Renderer: r})

	for _, args := range [][]string{{"check", "--environment", "dev"}, {"status", "--environment", "pace"}} {
		command.SetArgs(args)
		if err := command.Execute(); err != nil {
			t.Fatalf("Execute(%v) error = %v", args, err)
		}
	}
	if len(c.inputs) != 1 || c.inputs[0].Environment != "dev" {
		t.Fatalf("check inputs = %#v", c.inputs)
	}
	if len(s.inputs) != 1 || s.inputs[0].Environment != "pace" {
		t.Fatalf("status inputs = %#v", s.inputs)
	}
	if r.calls != 2 {
		t.Fatalf("render calls = %d", r.calls)
	}
}
