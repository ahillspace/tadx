package logout_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
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

func TestExecuteStopsOnFreshResolutionFailure(t *testing.T) {
	storage := &store{}
	_, err := logout.New(&resolver{err: errors.New("configuration changed")}, storage).Execute(context.Background(), logout.Input{Environment: "dev"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "auth.logout.resolve" || storage.calls != 0 {
		t.Fatalf("error = %#v, store calls = %d", err, storage.calls)
	}
}

func TestExecuteWrapsRemovalFailure(t *testing.T) {
	_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: errors.New("unavailable")}).Execute(context.Background(), logout.Input{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Environment != "dev" || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

type installedConfigurationError struct{ deleteFailed bool }

func (installedConfigurationError) Error() string {
	return "configuration took effect but was not synced"
}

func (installedConfigurationError) ConfigurationInstalled() bool { return true }

func (e installedConfigurationError) ExternalCommitConfirmed() bool { return !e.deleteFailed }

func TestExecuteReportsRemovalWhenConfigurationIsNotDurable(t *testing.T) {
	_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: fmt.Errorf("install configuration: %w", installedConfigurationError{})}).Execute(context.Background(), logout.Input{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome != errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary, "failed") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteDoesNotConfirmRemovalWhenCredentialDeletionFailedAfterInstall(t *testing.T) {
	cause := &errs.Error{Kind: errs.KindOperation, Summary: "credential delete denied", CorrectiveAction: "Delete the orphaned entry by hand."}
	_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: errors.Join(cause, installedConfigurationError{deleteFailed: true})}).Execute(context.Background(), logout.Input{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome == errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary, "was removed") || !strings.Contains(payload.CorrectiveAction, "orphaned entry") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}
