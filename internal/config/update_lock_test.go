package config

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/lock"
)

func TestUpdateStopsWaitingForAHeldConfigurationLock(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	if err := Save(path, Config{Version: CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	holder, err := lock.Acquire(path + configLockSuffix)
	if err != nil {
		t.Fatal(err)
	}
	defer holder.Release()
	shortenLockWait(t, 100*time.Millisecond)

	started := time.Now()
	mutated := false
	_, err = Update(path, false, func(current Config) (Config, error) {
		mutated = true
		return current, nil
	})
	var timeout *LockTimeoutError
	if !errors.As(err, &timeout) {
		t.Fatalf("Update() error = %v, want LockTimeoutError", err)
	}
	if elapsed := time.Since(started); elapsed > 5*time.Second {
		t.Fatalf("Update() waited %s for the held lock", elapsed)
	}
	if mutated {
		t.Fatal("Update() ran the mutator without the configuration lock")
	}
	if !timeout.Retryable() || timeout.CorrectiveAction() == "" {
		t.Fatalf("LockTimeoutError retry advice = %v/%q", timeout.Retryable(), timeout.CorrectiveAction())
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatalf("configuration after lock timeout = %q, want %q", after, before)
	}
}

func TestUpdateProceedsWhenTheConfigurationLockIsReleasedInTime(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	holder, err := lock.Acquire(path + configLockSuffix)
	if err != nil {
		t.Fatal(err)
	}
	shortenLockWait(t, 10*time.Second)
	go func() {
		time.Sleep(100 * time.Millisecond)
		_ = holder.Release()
	}()

	mutated := false
	if _, err := Update(path, true, func(current Config) (Config, error) {
		mutated = true
		return current, nil
	}); err != nil {
		t.Fatalf("Update() error = %v", err)
	}
	if !mutated {
		t.Fatal("Update() did not run the mutator after the lock was released")
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("saved configuration: %v", err)
	}
}

func shortenLockWait(t *testing.T, wait time.Duration) {
	t.Helper()
	previous := lockWait
	lockWait = wait
	t.Cleanup(func() { lockWait = previous })
}
