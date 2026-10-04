package config

import (
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/fsreplace"
)

func TestUpdateWithPostSaveClassifiesFailedRestoreByInstallPhase(t *testing.T) {
	for _, test := range []struct {
		name                 string
		restore              func(string, string) error
		wantReferencePresent bool
	}{
		{name: "restore replacement failed", restore: func(string, string) error { return errors.New("restore rename denied") }},
		{name: "restore installed but sync failed", restore: installedWithoutSync, wantReferencePresent: true},
	} {
		t.Run(test.name, func(t *testing.T) {
			path, original := durabilityFixture(t)
			replaceFileSequence(t, fsreplace.Replace, test.restore)
			deleteErr := errors.New("credential deletion denied")
			callbackSawCleared := false
			_, err := UpdateWithPostSave(path, false, func(current Config) (Config, func() error, error) {
				return clearedReference(current), func() error {
					loaded, loadErr := Load(path)
					callbackSawCleared = loadErr == nil && loaded.Environments["dev"].Auth.CredentialRef == ""
					return deleteErr
				}, nil
			})
			failure, ok := errors.AsType[*PostSaveRestoreError](err)
			if !ok || !errors.Is(err, deleteErr) || !callbackSawCleared {
				t.Fatalf("error type and operation order: type=%T callback_saw_cleared=%t", err, callbackSawCleared)
			}
			if failure.PriorConfigurationReinstalled() != test.wantReferencePresent || failure.ConfigurationInstalled() == test.wantReferencePresent || failure.ExternalCommitConfirmed() {
				t.Fatalf("restore classification: prior_reinstalled=%t new_installed=%t external_confirmed=%t", failure.PriorConfigurationReinstalled(), failure.ConfigurationInstalled(), failure.ExternalCommitConfirmed())
			}
			loaded, loadErr := Load(path)
			if loadErr != nil {
				t.Fatal(loadErr)
			}
			gotReference := loaded.Environments["dev"].Auth.CredentialRef
			if (gotReference != "") != test.wantReferencePresent {
				t.Fatalf("stored reference present=%t, want %t", gotReference != "", test.wantReferencePresent)
			}
			if test.wantReferencePresent && gotReference != original.Environments["dev"].Auth.CredentialRef {
				t.Fatal("restored reference differs from original")
			}
		})
	}
}
