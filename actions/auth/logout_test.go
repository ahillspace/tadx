package auth_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	authops "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type logoutResolver struct {
	target authops.LogoutTarget
	err    error
	alias  string
}

func (r *logoutResolver) Resolve(_ context.Context, alias string) (authops.LogoutTarget, error) {
	r.alias = alias
	return r.target, r.err
}

type logoutStore struct {
	result authops.LogoutRemoveResult
	err    error
	target authops.LogoutTarget
	calls  int
}

func (s *logoutStore) Remove(_ context.Context, target authops.LogoutTarget) (authops.LogoutRemoveResult, error) {
	s.calls++
	s.target = target
	return s.result, s.err
}

func TestExecuteRemovesOnlyStoredCredential(t *testing.T) {
	resolver := &logoutResolver{target: authops.LogoutTarget{Environment: "dev"}}
	store := &logoutStore{result: authops.LogoutRemoveResult{Removed: true}}
	got, err := authops.NewLogout(resolver, store).Execute(context.Background(), authops.LogoutInput{Environment: "dev"})
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
	got, err := authops.NewLogout(&logoutResolver{target: authops.LogoutTarget{Environment: "dev"}}, &logoutStore{}).Execute(context.Background(), authops.LogoutInput{Environment: "dev"})
	if err != nil || got.Status != "unchanged" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestExecuteStopsOnFreshResolutionFailure(t *testing.T) {
	storage := &logoutStore{}
	_, err := authops.NewLogout(&logoutResolver{err: errors.New("configuration changed")}, storage).Execute(context.Background(), authops.LogoutInput{Environment: "dev"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "auth.logout.resolve" || storage.calls != 0 {
		t.Fatalf("error = %#v, store calls = %d", err, storage.calls)
	}
}

func TestExecuteWrapsRemovalFailure(t *testing.T) {
	_, err := authops.NewLogout(&logoutResolver{target: authops.LogoutTarget{Environment: "dev"}}, &logoutStore{err: errors.New("unavailable")}).Execute(context.Background(), authops.LogoutInput{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Environment != "dev" || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

type logoutInstalledConfigurationError struct{ deleteFailed bool }

func (logoutInstalledConfigurationError) Error() string {
	return "configuration took effect but was not synced"
}

func (logoutInstalledConfigurationError) ConfigurationInstalled() bool { return true }

func (e logoutInstalledConfigurationError) ExternalCommitConfirmed() bool { return !e.deleteFailed }

func TestExecuteReportsRemovalWhenConfigurationIsNotDurable(t *testing.T) {
	_, err := authops.NewLogout(&logoutResolver{target: authops.LogoutTarget{Environment: "dev"}}, &logoutStore{err: fmt.Errorf("install configuration: %w", logoutInstalledConfigurationError{})}).Execute(context.Background(), authops.LogoutInput{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome != errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary, "failed") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteDoesNotConfirmRemovalWhenCredentialDeletionFailedAfterInstall(t *testing.T) {
	cause := &errs.Error{Kind: errs.KindOperation, Summary: "credential delete denied", CorrectiveAction: "Delete the orphaned entry by hand."}
	_, err := authops.NewLogout(&logoutResolver{target: authops.LogoutTarget{Environment: "dev"}}, &logoutStore{err: errors.Join(cause, logoutInstalledConfigurationError{deleteFailed: true})}).Execute(context.Background(), authops.LogoutInput{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome == errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary, "was removed") || !strings.Contains(payload.CorrectiveAction, "orphaned entry") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}
