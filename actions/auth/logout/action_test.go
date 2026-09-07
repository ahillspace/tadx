package logout_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	logout "github.com/ahillspace/tadx/actions/auth/logout"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type resolver struct {
	target logout.Target
	err    error
	alias  string
}

func (r *resolver) Resolve(_ context.Context, alias string) (logout.Target, error) {
	r.alias = alias
	return r.target, r.err
}

type store struct {
	result logout.RemoveResult
	err    error
	target logout.Target
	calls  int
}

func (s *store) Remove(_ context.Context, target logout.Target) (logout.RemoveResult, error) {
	s.calls++
	s.target = target
	return s.result, s.err
}

func TestExecuteRemovesOnlyStoredCredential(t *testing.T) {
	resolver := &resolver{target: logout.Target{Environment: "dev"}}
	store := &store{result: logout.RemoveResult{Removed: true}}
	got, err := logout.New(resolver, store).Execute(context.Background(), logout.Input{Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if resolver.alias != "dev" || store.calls != 1 || store.target.Environment != "dev" {
		t.Fatalf("handoff: alias=%q calls=%d target=%#v", resolver.alias, store.calls, store.target)
	}
	if got.Status != "removed" || got.TableauPATRevoked || !strings.Contains(strings.Join(got.Help, " "), "remains valid") {
		t.Fatalf("output = %#v", got)
	}
	var rendered bytes.Buffer
	if err := output.Render(&rendered, got); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(rendered.String(), "tableau_pat_revoked: false") {
		t.Fatalf("remote PAT state is unclear: %s", rendered.String())
	}
}

func TestExecuteMissingStoredCredentialIsNoOp(t *testing.T) {
	got, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{}).Execute(context.Background(), logout.Input{Environment: "dev"})
	if err != nil || got.Status != "unchanged" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecuteRequiresExplicitEnvironment(t *testing.T) {
	_, err := logout.New(&resolver{}, &store{}).Execute(context.Background(), logout.Input{})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindUsage {
		t.Fatalf("error = %#v", err)
	}
}

func TestExecuteWrapsRemovalFailure(t *testing.T) {
	_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: errors.New("unavailable")}).Execute(context.Background(), logout.Input{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Environment != "dev" || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}
