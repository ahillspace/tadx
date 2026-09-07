package auth_test

import (
	"context"
	"errors"
	"strings"
	"testing"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	authlogin "github.com/ahillspace/tadx/actions/auth/login"
	authlogout "github.com/ahillspace/tadx/actions/auth/logout"
	authstatus "github.com/ahillspace/tadx/actions/auth/status"
	authcli "github.com/ahillspace/tadx/internal/cli/auth"
	"github.com/ahillspace/tadx/internal/errs"
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

type login struct{ inputs []authlogin.Input }

func (l *login) Execute(_ context.Context, input authlogin.Input) (authlogin.Output, error) {
	l.inputs = append(l.inputs, input)
	return authlogin.Output{}, nil
}

type logout struct{ inputs []authlogout.Input }

func (l *logout) Execute(_ context.Context, input authlogout.Input) (authlogout.Output, error) {
	l.inputs = append(l.inputs, input)
	return authlogout.Output{}, nil
}

type prompter struct {
	terminal    bool
	name        string
	secret      string
	nameErr     error
	secretErr   error
	nameReads   int
	secretReads int
}

func (p *prompter) IsTerminal() bool { return p.terminal }
func (p *prompter) ReadPATName(context.Context) (string, error) {
	p.nameReads++
	return p.name, p.nameErr
}
func (p *prompter) ReadPATSecret(context.Context) (string, error) {
	p.secretReads++
	return p.secret, p.secretErr
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

func TestLoginRequiresTerminalAndMapsPromptedCredentials(t *testing.T) {
	login := &login{}
	prompt := &prompter{terminal: true, name: "pat-name", secret: "pat-secret"}
	renderer := &renderer{}
	command := authcli.New(authcli.Dependencies{Login: login, Prompter: prompt, Renderer: renderer})
	command.SetArgs([]string{"login", "--environment", "Prod-West"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(login.inputs) != 1 || login.inputs[0].Environment != "Prod-West" || login.inputs[0].PATName != "pat-name" || login.inputs[0].PATSecret != "pat-secret" {
		t.Fatalf("inputs = %#v", login.inputs)
	}
	if prompt.nameReads != 1 || prompt.secretReads != 1 || renderer.calls != 1 {
		t.Fatalf("prompt reads = %d/%d, renders = %d", prompt.nameReads, prompt.secretReads, renderer.calls)
	}
}

func TestLoginRejectsMissingEnvironmentBeforePrompt(t *testing.T) {
	prompt := &prompter{terminal: true}
	command := authcli.New(authcli.Dependencies{Login: &login{}, Prompter: prompt, Renderer: &renderer{}})
	command.SetArgs([]string{"login"})
	err := command.Execute()
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || prompt.nameReads != 0 || prompt.secretReads != 0 {
		t.Fatalf("error = %#v, prompt = %#v", err, prompt)
	}
}

func TestLoginRejectsNonTerminalInputWithoutReading(t *testing.T) {
	prompt := &prompter{}
	login := &login{}
	command := authcli.New(authcli.Dependencies{Login: login, Prompter: prompt, Renderer: &renderer{}})
	command.SetArgs([]string{"login", "--environment", "dev"})
	err := command.Execute()
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || !strings.Contains(err.Error(), "interactive terminal") {
		t.Fatalf("error = %#v", err)
	}
	if prompt.nameReads != 0 || prompt.secretReads != 0 || len(login.inputs) != 0 {
		t.Fatalf("input was read or action called: prompt=%#v inputs=%#v", prompt, login.inputs)
	}
}

func TestLoginDoesNotExposeCredentialFlags(t *testing.T) {
	login := &login{}
	command := authcli.New(authcli.Dependencies{Login: login, Prompter: &prompter{terminal: true}, Renderer: &renderer{}})
	command.SetArgs([]string{"login", "--environment", "dev", "--pat-secret", "private"})
	err := command.Execute()
	if err == nil || !strings.Contains(err.Error(), "unknown flag") || len(login.inputs) != 0 {
		t.Fatalf("error = %v, inputs = %#v", err, login.inputs)
	}
}

func TestLoginPromptFailureDoesNotCallAction(t *testing.T) {
	login := &login{}
	prompt := &prompter{terminal: true, nameErr: errors.New("terminal closed")}
	command := authcli.New(authcli.Dependencies{Login: login, Prompter: prompt, Renderer: &renderer{}})
	command.SetArgs([]string{"login", "--environment", "dev"})
	err := command.Execute()
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "auth.login.input" || len(login.inputs) != 0 {
		t.Fatalf("error = %#v, inputs = %#v", err, login.inputs)
	}
}

func TestLogoutIsNoninteractiveAndMapsEnvironment(t *testing.T) {
	logout := &logout{}
	renderer := &renderer{}
	command := authcli.New(authcli.Dependencies{Logout: logout, Prompter: &prompter{}, Renderer: renderer})
	command.SetArgs([]string{"logout", "--environment", "dev"})
	if err := command.Execute(); err != nil {
		t.Fatal(err)
	}
	if len(logout.inputs) != 1 || logout.inputs[0].Environment != "dev" || renderer.calls != 1 {
		t.Fatalf("inputs = %#v, renders = %d", logout.inputs, renderer.calls)
	}
}

func TestLogoutRequiresExplicitEnvironment(t *testing.T) {
	logout := &logout{}
	command := authcli.New(authcli.Dependencies{Logout: logout, Renderer: &renderer{}})
	command.SetArgs([]string{"logout"})
	err := command.Execute()
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage || len(logout.inputs) != 0 {
		t.Fatalf("error = %#v, inputs = %#v", err, logout.inputs)
	}
}
