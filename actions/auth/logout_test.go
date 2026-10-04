package auth_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	logout "github.com/ahillspace/tadx/actions/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

func newLogout(resolver logout.LogoutResolver, store logout.LogoutStore) *logout.Service {
	return logout.New(logout.Ports{LogoutResolver: resolver, LogoutStore: store})
}

type logoutResolver struct {
	target logout.LogoutTarget
	err    error
	alias  string
}

func (r *logoutResolver) Resolve(_ context.Context, alias string) (logout.LogoutTarget, error) {
	r.alias = alias
	return r.target, r.err
}

type logoutStoreFake struct {
	result logout.LogoutRemoveResult
	err    error
	target logout.LogoutTarget
	calls  int
}

func (s *logoutStoreFake) Remove(_ context.Context, target logout.LogoutTarget) (logout.LogoutRemoveResult, error) {
	s.calls++
	s.target = target
	return s.result, s.err
}

func TestLogoutExecuteRemovesOnlyStoredCredential(t *testing.T) {
	logoutResolver := &logoutResolver{target: logout.LogoutTarget{Environment: "dev"}}
	logoutStoreFake := &logoutStoreFake{result: logout.LogoutRemoveResult{Removed: true}}
	got, err := newLogout(logoutResolver, logoutStoreFake).Logout(context.Background(), logout.LogoutInput{Environment: "dev"})
	if err != nil {
		t.Fatal(err)
	}
	if logoutResolver.alias != "dev" || logoutStoreFake.calls != 1 || logoutStoreFake.target.Environment != "dev" {
		t.Fatalf("handoff: alias=%q calls=%d target=%#v", logoutResolver.alias, logoutStoreFake.calls, logoutStoreFake.target)
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

func TestLogoutExecuteMissingStoredCredentialIsNoOp(t *testing.T) {
	got, err := newLogout(&logoutResolver{target: logout.LogoutTarget{Environment: "dev"}}, &logoutStoreFake{}).Logout(context.Background(), logout.LogoutInput{Environment: "dev"})
	if err != nil || got.Status != "unchanged" {
		t.Fatalf("output = %#v, error = %v", got, err)
	}
}

func TestLogoutExecuteStopsOnFreshResolutionFailure(t *testing.T) {
	storage := &logoutStoreFake{}
	_, err := newLogout(&logoutResolver{err: errors.New("configuration changed")}, storage).Logout(context.Background(), logout.LogoutInput{Environment: "dev"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.ID != "auth.logout.resolve" || storage.calls != 0 {
		t.Fatalf("error = %#v, store calls = %d", err, storage.calls)
	}
}

func TestLogoutExecuteWrapsRemovalFailure(t *testing.T) {
	_, err := newLogout(&logoutResolver{target: logout.LogoutTarget{Environment: "dev"}}, &logoutStoreFake{err: errors.New("unavailable")}).Logout(context.Background(), logout.LogoutInput{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Environment != "dev" || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestLogoutExecuteReportsRemovalWhenConfigurationIsNotDurable(t *testing.T) {
	_, err := newLogout(&logoutResolver{target: logout.LogoutTarget{Environment: "dev"}}, &logoutStoreFake{err: &config.InstalledError{Err: errors.New("sync failed")}}).Logout(context.Background(), logout.LogoutInput{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome != errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary, "failed") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestLogoutExecuteDoesNotConfirmRemovalWhenCredentialDeletionFailedAfterInstall(t *testing.T) {
	cause := &errs.Error{Kind: errs.KindOperation, Summary: "credential deletion unconfirmed", CorrectiveAction: "Inspect the named entry; remove it if present."}
	_, err := newLogout(&logoutResolver{target: logout.LogoutTarget{Environment: "dev"}}, &logoutStoreFake{err: errors.Join(cause, &config.InstalledError{Err: errors.New("sync failed"), ExternalErr: errors.New("delete denied")})}).Logout(context.Background(), logout.LogoutInput{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome == errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || !strings.Contains(payload.Summary, "not confirmed") || !strings.Contains(payload.CorrectiveAction, "remove it if present") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestLogoutReportsUncertainRestoredReferenceWithoutOrphanClaim(t *testing.T) {
	restored := &config.PostSaveRestoreError{ExternalErr: errors.New("delete denied"), RestoreErr: errors.New("sync failed"), PriorConfigurationInstalled: true}
	_, err := newLogout(&logoutResolver{target: logout.LogoutTarget{Environment: "dev"}}, &logoutStoreFake{err: restored}).Logout(t.Context(), logout.LogoutInput{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Phase != errs.PhasePersistence || payload.Outcome != errs.OutcomeUnknown {
		t.Fatalf("failure classification: %#v", payload)
	}
	if !strings.Contains(payload.Summary, "durable") || !strings.Contains(payload.Summary, "not confirmed") || strings.Contains(strings.ToLower(payload.CorrectiveAction), "orphan") || !strings.Contains(payload.CorrectiveAction, "stored reference") || !strings.Contains(payload.CorrectiveAction, "OS credential store entry") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("failure guidance: %#v", payload)
	}
}

type unrelatedInstalledStoreError struct{}

func (unrelatedInstalledStoreError) Error() string { return "unrelated installed-state claim" }

func (unrelatedInstalledStoreError) ConfigurationInstalled() bool { return true }

func (unrelatedInstalledStoreError) ExternalCommitConfirmed() bool { return false }

type unrelatedRestoredStoreError struct{}

func (unrelatedRestoredStoreError) Error() string { return "unrelated restored-state claim" }

func (unrelatedRestoredStoreError) PriorConfigurationReinstalled() bool { return true }

func TestLogoutIgnoresUnrelatedStoreStateClaims(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "installed", err: unrelatedInstalledStoreError{}},
		{name: "restored", err: unrelatedRestoredStoreError{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := newLogout(&logoutResolver{target: logout.LogoutTarget{Environment: "dev"}}, &logoutStoreFake{err: test.err}).Logout(t.Context(), logout.LogoutInput{Environment: "dev"})
			payload := errs.Structure(err).Error
			if payload.Phase != "" || payload.Outcome != "" || strings.Contains(payload.Summary, "reference") || strings.Contains(strings.ToLower(payload.CorrectiveAction), "orphan") {
				t.Fatalf("unrelated store failure was classified as config state: %#v", payload)
			}
		})
	}
}
