package workspace

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/fsreplace"
)

// installedUpdates makes every configuration update take effect and then report
// that it could not be made durable or restored, as config does after a failed
// directory sync followed by a failed restore.
func installedUpdates(m *Manager) {
	m.update = func(path string, createIfMissing bool, mutate func(config.Config) (config.Config, error)) (config.Config, error) {
		next, err := config.Update(path, createIfMissing, mutate)
		if err != nil {
			return next, err
		}
		return next, &config.InstalledError{Err: errors.New("sync failed")}
	}
	m.updateWithRollback = func(path string, createIfMissing bool, mutate func(config.Config) (config.Config, func() error, error)) (config.Config, error) {
		next, err := config.UpdateWithRollback(path, createIfMissing, func(current config.Config) (config.Config, func() error, error) {
			next, _, err := mutate(current)
			return next, nil, err
		})
		if err != nil {
			return next, err
		}
		return next, &config.InstalledError{Err: errors.New("sync failed")}
	}
}

// restoredUpdates reports a durability failure after the prior configuration
// was reinstalled, so nothing was registered.
func restoredUpdates(m *Manager) {
	m.update = func(string, bool, func(config.Config) (config.Config, error)) (config.Config, error) {
		return config.Config{}, errors.Join(&fsreplace.DurabilityError{Dir: "config", Err: errors.New("sync failed")}, errors.New("the prior configuration was reinstalled"))
	}
}

func installedTestManager(t *testing.T) (*Manager, string) {
	t.Helper()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configPath, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	return NewManager(configPath, nil), configPath
}

func assertInstalled(t *testing.T, err error) {
	t.Helper()
	var installed *config.InstalledError
	if !errors.As(err, &installed) {
		t.Fatalf("error = %v, want the installed-configuration failure", err)
	}
}

func assertResolvesToManifest(t *testing.T, m *Manager, name string, record Record) {
	t.Helper()
	m.update, m.updateWithRollback = nil, nil
	resolved, err := m.Resolve(context.Background(), name, "")
	if err != nil || resolved.ID != record.ID || !resolved.ManifestValid {
		t.Fatalf("installed registration resolved to %+v, %v; want preserved workspace %+v", resolved, err, record)
	}
}

func TestCreateKeepsWorkspaceWhenRegistrationStaysInstalled(t *testing.T) {
	m, _ := installedTestManager(t)
	installedUpdates(m)
	root := filepath.Join(t.TempDir(), "created")

	record, err := m.Create(context.Background(), "created", root)
	assertInstalled(t, err)
	if record.ID == "" || record.Name != "created" {
		t.Fatalf("record = %+v, want the installed identity", record)
	}
	assertResolvesToManifest(t, m, "created", record)
}

func TestCreateRemovesWorkspaceWhenPriorConfigurationWasRestored(t *testing.T) {
	m, _ := installedTestManager(t)
	restoredUpdates(m)
	root := filepath.Join(t.TempDir(), "created")

	if _, err := m.Create(context.Background(), "created", root); err == nil {
		t.Fatal("Create() error = nil")
	}
	if _, err := os.Lstat(root); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("created root after restored configuration: %v, want removed", err)
	}
}

func cloneSource(t *testing.T, m *Manager) (string, []byte) {
	t.Helper()
	source := filepath.Join(t.TempDir(), "source")
	if _, err := m.Create(context.Background(), "source", source); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(source, "artifacts", "kept.txt"), []byte("source"), 0o600); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(source, config.WorkspaceConfigName))
	if err != nil {
		t.Fatal(err)
	}
	return source, manifest
}

func assertSourceUnchanged(t *testing.T, source string, manifest []byte) {
	t.Helper()
	current, err := os.ReadFile(filepath.Join(source, config.WorkspaceConfigName))
	if err != nil || string(current) != string(manifest) {
		t.Fatalf("source manifest changed: %v", err)
	}
	if data, err := os.ReadFile(filepath.Join(source, "artifacts", "kept.txt")); err != nil || string(data) != "source" {
		t.Fatalf("source artifact changed: %q, %v", data, err)
	}
}

func TestCloneKeepsDestinationWhenRegistrationStaysInstalled(t *testing.T) {
	m, _ := installedTestManager(t)
	source, manifest := cloneSource(t, m)
	installedUpdates(m)
	destination := filepath.Join(t.TempDir(), "copy")

	record, err := m.Clone(context.Background(), "source", "copy", destination)
	assertInstalled(t, err)
	if record.ID == "" || record.Name != "copy" {
		t.Fatalf("record = %+v, want the installed identity", record)
	}
	assertResolvesToManifest(t, m, "copy", record)
	assertSourceUnchanged(t, source, manifest)
}

func TestCloneRemovesDestinationWhenPriorConfigurationWasRestored(t *testing.T) {
	m, _ := installedTestManager(t)
	source, manifest := cloneSource(t, m)
	restoredUpdates(m)
	destination := filepath.Join(t.TempDir(), "copy")

	if _, err := m.Clone(context.Background(), "source", "copy", destination); err == nil {
		t.Fatal("Clone() error = nil")
	}
	if _, err := os.Lstat(destination); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("clone destination after restored configuration: %v, want removed", err)
	}
	assertSourceUnchanged(t, source, manifest)
}

func TestDeleteFinishesWhenUnregistrationStaysInstalled(t *testing.T) {
	m, _ := installedTestManager(t)
	root := filepath.Join(t.TempDir(), "doomed")
	created, err := m.Create(context.Background(), "doomed", root)
	if err != nil {
		t.Fatal(err)
	}
	resolved, err := m.Resolve(context.Background(), "doomed", "")
	if err != nil {
		t.Fatal(err)
	}
	installedUpdates(m)

	_, err = m.Delete(context.Background(), resolved)
	assertInstalled(t, err)
	entries, readErr := os.ReadDir(filepath.Dir(root))
	if readErr != nil {
		t.Fatal(readErr)
	}
	if len(entries) != 0 {
		t.Fatalf("workspace files remain after installed unregistration: %v", entries)
	}
	m.update, m.updateWithRollback = nil, nil
	if _, err := m.Resolve(context.Background(), "doomed", ""); err == nil {
		t.Fatalf("workspace %s is still registered", created.ID)
	}
}
