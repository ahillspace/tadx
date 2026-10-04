package logout_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	logout "github.com/ahillspace/tadx/actions/auth/logout"
	"github.com/ahillspace/tadx/internal/config"
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

type unrelatedInstalledStoreError struct{}

func (unrelatedInstalledStoreError) Error() string { return "unrelated installed-state claim" }

func (unrelatedInstalledStoreError) ConfigurationInstalled() bool { return true }

func (unrelatedInstalledStoreError) ExternalCommitConfirmed() bool { return false }

type unrelatedRestoredStoreError struct{}

func (unrelatedRestoredStoreError) Error() string { return "unrelated restored-state claim" }

func (unrelatedRestoredStoreError) PriorConfigurationReinstalled() bool { return true }

func TestExecuteIgnoresUnrelatedStoreStateClaims(t *testing.T) {
	for _, test := range []struct {
		name string
		err  error
	}{
		{name: "installed", err: unrelatedInstalledStoreError{}},
		{name: "restored", err: unrelatedRestoredStoreError{}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: test.err}).Execute(t.Context(), logout.Input{Environment: "dev"})
			payload := errs.Structure(err).Error
			if payload.Phase != "" || payload.Outcome != "" || strings.Contains(payload.Summary, "reference") || strings.Contains(strings.ToLower(payload.CorrectiveAction), "orphan") {
				t.Fatalf("unrelated store failure was classified as config state: %#v", payload)
			}
		})
	}
}

func TestExecuteReportsRemovalWhenConfigurationIsNotDurable(t *testing.T) {
	_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: &config.InstalledError{Err: errors.New("sync failed")}}).Execute(context.Background(), logout.Input{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome != errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || strings.Contains(payload.Summary, "failed") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteDoesNotConfirmRemovalWhenCredentialDeletionFailedAfterInstall(t *testing.T) {
	cause := &errs.Error{Kind: errs.KindOperation, Summary: "credential deletion unconfirmed", CorrectiveAction: "Inspect the named entry; remove it if present."}
	_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: errors.Join(cause, &config.InstalledError{Err: errors.New("sync failed"), ExternalErr: errors.New("delete denied")})}).Execute(context.Background(), logout.Input{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Outcome == errs.OutcomeConfirmed || payload.Phase != errs.PhasePersistence || !strings.Contains(payload.Summary, "not confirmed") || !strings.Contains(payload.CorrectiveAction, "remove it if present") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("error = %#v", payload)
	}
}

func TestExecuteReportsUncertainRestoredReferenceWithoutOrphanClaim(t *testing.T) {
	restored := &config.PostSaveRestoreError{ExternalErr: errors.New("delete denied"), RestoreErr: errors.New("sync failed"), PriorConfigurationInstalled: true}
	_, err := logout.New(&resolver{target: logout.Target{Environment: "dev"}}, &store{err: restored}).Execute(t.Context(), logout.Input{Environment: "dev"})
	payload := errs.Structure(err).Error
	if payload.ID != "auth.logout.remove" || payload.Phase != errs.PhasePersistence || payload.Outcome != errs.OutcomeUnknown {
		t.Fatalf("failure classification: %#v", payload)
	}
	if !strings.Contains(payload.Summary, "durable") || !strings.Contains(payload.Summary, "not confirmed") || strings.Contains(strings.ToLower(payload.CorrectiveAction), "orphan") || !strings.Contains(payload.CorrectiveAction, "stored reference") || !strings.Contains(payload.CorrectiveAction, "OS credential store entry") || !strings.Contains(payload.CorrectiveAction, "not revoked") {
		t.Fatalf("failure guidance: %#v", payload)
	}
}
