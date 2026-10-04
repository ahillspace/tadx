package auth

import (
	"errors"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
)

func TestAuthCredentialStoreNamesOrphanedEntryWhenDeletionFailsAfterInstall(t *testing.T) {
	reference := coreauth.CredentialReference("cred_88888888888888888888888888888888")
	failed := orphanedCredentialError(&config.InstalledError{Err: errors.New("sync failed"), ExternalErr: errors.New("delete denied")}, reference)
	_, advice := errs.RetryAdvice(failed)
	if !strings.Contains(advice, coreauth.CredentialStoreEntry(reference)) || !strings.Contains(advice, "remove it if present") {
		t.Fatalf("corrective action = %q, want the orphaned entry", advice)
	}
	for _, err := range []error{errors.New("plain failure"), &config.InstalledError{Err: errors.New("sync failed")}, unrelatedInstalledCredentialError{}} {
		if got := orphanedCredentialError(err, reference); got != err {
			t.Fatalf("orphanedCredentialError(%v) = %v, want unchanged", err, got)
		}
	}
}

type unrelatedInstalledCredentialError struct{}

func (unrelatedInstalledCredentialError) Error() string { return "unrelated credential-store failure" }

func (unrelatedInstalledCredentialError) ConfigurationInstalled() bool { return true }

func (unrelatedInstalledCredentialError) ExternalCommitConfirmed() bool { return false }
