package config

import (
	"errors"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/fsreplace"
)

// replaceFileSequence installs the real replacement for each scripted call and
// then reports that call's failure. A nil installed entry fails before renaming.
func replaceFileSequence(t *testing.T, failures ...func(from, to string) error) {
	t.Helper()
	previous := replaceFile
	call := 0
	replaceFile = func(from, to string) error {
		if call >= len(failures) {
			return previous(from, to)
		}
		failure := failures[call]
		call++
		return failure(from, to)
	}
	t.Cleanup(func() { replaceFile = previous })
}

func installedWithoutSync(from, to string) error {
	if err := fsreplace.Rename(from, to); err != nil {
		return err
	}
	return &fsreplace.DurabilityError{Dir: filepath.Dir(to), Err: errors.New("sync failed")}
}

func durabilityFixture(t *testing.T) (string, Config) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.yaml")
	original := Config{Version: CurrentVersion, Environments: map[string]Environment{
		"dev": {URL: "https://example.test", Auth: Auth{Type: AuthTypePAT, CredentialRef: "cred_11111111111111111111111111111111"}},
	}}
	if err := Save(path, original); err != nil {
		t.Fatal(err)
	}
	return path, original
}

func clearedReference(current Config) Config {
	environment := current.Environments["dev"]
	environment.Auth.CredentialRef = ""
	current.Environments["dev"] = environment
	return current
}

func TestUpdateReinstallsPriorConfigurationBeforeRollbackWhenSyncFails(t *testing.T) {
	path, _ := durabilityFixture(t)
	replaceFileSequence(t, installedWithoutSync)
	sawPrior := false

	_, err := UpdateWithRollback(path, false, func(current Config) (Config, func() error, error) {
		return clearedReference(current), func() error {
			loaded, loadErr := Load(path)
			sawPrior = loadErr == nil && loaded.Environments["dev"].Auth.CredentialRef != ""
			return nil
		}, nil
	})
	var durability *fsreplace.DurabilityError
	var installed *InstalledError
	if !errors.As(err, &durability) || errors.As(err, &installed) {
		t.Fatalf("UpdateWithRollback() error = %v, want a durability failure with the prior configuration reinstalled", err)
	}
	if !sawPrior {
		t.Fatal("rollback ran before the prior configuration was reinstalled")
	}
}

func TestUpdateWithPostSaveSkipsExternalCommitWhenSyncFails(t *testing.T) {
	path, _ := durabilityFixture(t)
	replaceFileSequence(t, installedWithoutSync)
	committed := false

	if _, err := UpdateWithPostSave(path, false, func(current Config) (Config, func() error, error) {
		return clearedReference(current), func() error { committed = true; return nil }, nil
	}); err == nil {
		t.Fatal("UpdateWithPostSave() error = nil")
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	if committed || loaded.Environments["dev"].Auth.CredentialRef == "" {
		t.Fatalf("external commit ran = %t, CredentialRef = %q; want the prior configuration only", committed, loaded.Environments["dev"].Auth.CredentialRef)
	}
}

func TestUpdateKeepsExternalStateWithInstalledConfigurationWhenRestoreFails(t *testing.T) {
	path, _ := durabilityFixture(t)
	replaceFileSequence(t, installedWithoutSync, func(string, string) error { return errors.New("rename failed") })
	rolledBack, committed := false, false

	if _, err := UpdateWithRollback(path, false, func(current Config) (Config, func() error, error) {
		return clearedReference(current), func() error { rolledBack = true; return nil }, nil
	}); !isInstalled(err) || rolledBack {
		t.Fatalf("UpdateWithRollback() error = %v, rolled back = %t; want InstalledError without rollback", err, rolledBack)
	}

	path, _ = durabilityFixture(t)
	replaceFileSequence(t, installedWithoutSync, func(string, string) error { return errors.New("rename failed") })
	next, err := UpdateWithPostSave(path, false, func(current Config) (Config, func() error, error) {
		return clearedReference(current), func() error { committed = true; return nil }, nil
	})
	if !isInstalled(err) || !committed || next.Environments["dev"].Auth.CredentialRef != "" {
		t.Fatalf("UpdateWithPostSave() error = %v, committed = %t, returned %+v; want InstalledError with the external commit completed", err, committed, next.Environments["dev"].Auth)
	}
	loaded, loadErr := Load(path)
	if loadErr != nil || loaded.Environments["dev"].Auth.CredentialRef != "" {
		t.Fatalf("installed configuration = %+v, %v", loaded.Environments["dev"].Auth, loadErr)
	}
}

func isInstalled(err error) bool {
	var installed *InstalledError
	return errors.As(err, &installed) && installed.ConfigurationInstalled()
}
