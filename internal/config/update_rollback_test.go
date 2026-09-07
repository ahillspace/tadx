package config_test

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/lock"
)

func TestUpdateWithRollbackRestoresStagedFilesBeforeUnlockOnSaveFailure(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "config.yaml")
	backup := filepath.Join(directory, "saved-config.yaml")
	root := filepath.Join(directory, "workspace")
	staged := filepath.Join(directory, "staged-workspace")
	if err := config.Save(path, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(root, 0o700); err != nil {
		t.Fatal(err)
	}
	rolledBack := false
	_, err := config.UpdateWithRollback(path, false, func(c config.Config) (config.Config, func() error, error) {
		if err := os.Rename(root, staged); err != nil {
			t.Fatal(err)
		}
		if err := os.Rename(path, backup); err != nil {
			t.Fatal(err)
		}
		// A directory at the destination forces the atomic configuration save to fail.
		if err := os.Mkdir(path, 0o700); err != nil {
			t.Fatal(err)
		}
		return c, func() error {
			rolledBack = true
			handle, lockErr := lock.TryAcquire(path + ".lock")
			if handle != nil {
				_ = handle.Release()
			}
			if !errors.Is(lockErr, lock.ErrLocked) {
				t.Errorf("rollback ran without configuration lock: %v", lockErr)
			}
			if err := os.Remove(path); err != nil {
				return err
			}
			if err := os.Rename(backup, path); err != nil {
				return err
			}
			return os.Rename(staged, root)
		}, nil
	})
	if err == nil || !rolledBack {
		t.Fatalf("err=%v rollback=%t", err, rolledBack)
	}
	if _, err := os.Stat(root); err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatal(err)
	}
	handle, err := lock.TryAcquire(path + ".lock")
	if err != nil {
		t.Fatal(err)
	}
	if err := handle.Release(); err != nil {
		t.Fatal(err)
	}
}

func TestUpdateWithRollbackPreservesBothErrors(t *testing.T) {
	mutationErr := errors.New("mutation failed")
	rollbackErr := errors.New("restore failed")
	_, err := config.UpdateWithRollback(filepath.Join(t.TempDir(), "config.yaml"), true, func(c config.Config) (config.Config, func() error, error) {
		return c, func() error { return rollbackErr }, mutationErr
	})
	if !errors.Is(err, mutationErr) || !errors.Is(err, rollbackErr) {
		t.Fatalf("error = %v", err)
	}
}

func TestUpdateWithRollbackDoesNotRestoreAfterCommit(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	_, err := config.UpdateWithRollback(path, true, func(c config.Config) (config.Config, func() error, error) {
		return c, func() error { t.Error("committed mutation was rolled back"); return nil }, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := config.Load(path); err != nil {
		t.Fatal(err)
	}
}
