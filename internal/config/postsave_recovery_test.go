package config

import (
	"errors"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/fsreplace"
)

func TestPostSaveFailureRetainsInstalledConfigurationWhenRestoreCannotReplace(t *testing.T) {
	path, _ := durabilityFixture(t)
	restoreErr := errors.New("restoration replacement denied")
	replaceFileSequence(t, fsreplace.Replace, func(string, string) error { return restoreErr })
	externalErr := errors.New("credential deletion denied")

	next, err := UpdateWithPostSave(path, false, func(current Config) (Config, func() error, error) {
		return clearedReference(current), func() error { return externalErr }, nil
	})
	loaded, loadErr := Load(path)
	if loadErr != nil || loaded.Environments["dev"].Auth.CredentialRef != "" {
		t.Fatalf("actual installed configuration = %+v, %v", loaded.Environments["dev"].Auth, loadErr)
	}
	installed, ok := errors.AsType[*InstalledError](err)
	if !ok || !installed.ConfigurationInstalled() || installed.ExternalCommitConfirmed() || !installed.DurabilityConfirmed || !errors.Is(err, externalErr) || !errors.Is(err, restoreErr) || strings.Contains(err.Error(), "may not survive power loss") {
		t.Fatalf("installed configuration and failed external deletion lost their recovery state: %v", err)
	}
	if next.Environments["dev"].Auth.CredentialRef != "" || len(next.Environments) != 1 {
		t.Fatalf("returned configuration differs from the installed file: %+v", next)
	}
}

func TestPostSaveFailureReportsRestoredConfigurationWhenRestorationSyncFails(t *testing.T) {
	path, original := durabilityFixture(t)
	replaceFileSequence(t, fsreplace.Replace, installedWithoutSync)
	externalErr := errors.New("credential deletion denied")
	next, err := UpdateWithPostSave(path, false, func(current Config) (Config, func() error, error) {
		return clearedReference(current), func() error { return externalErr }, nil
	})
	if _, ok := errors.AsType[*InstalledError](err); ok || !errors.Is(err, externalErr) {
		t.Fatalf("restored configuration incorrectly reported as the new installed state: %v", err)
	}
	if _, ok := errors.AsType[*fsreplace.DurabilityError](err); !ok {
		t.Fatalf("restoration durability failure was lost: %v", err)
	}
	loaded, loadErr := Load(path)
	if loadErr != nil || loaded.Environments["dev"].Auth.CredentialRef != original.Environments["dev"].Auth.CredentialRef || next.Environments["dev"].Auth.CredentialRef != loaded.Environments["dev"].Auth.CredentialRef {
		t.Fatalf("restored file and result disagree: visible=%+v returned=%+v error=%v", loaded, next, loadErr)
	}
}
