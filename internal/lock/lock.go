// Package lock provides a cross-platform, workspace-scoped advisory file lock
// used to serialize mutating tadx operations across concurrent processes.
package lock

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"
)

// ErrLocked is returned by TryAcquire when another process already holds the lock.
var ErrLocked = errors.New("workspace lock is held by another process")

// Handle is an acquired advisory lock. Release it exactly once with Release.
type Handle struct {
	file *os.File
}

// Acquire opens (creating if necessary) the lock file at path and blocks until
// it can take an exclusive advisory lock.
func Acquire(path string) (*Handle, error) {
	return acquire(path, true, false)
}

// TryAcquire attempts to take the exclusive advisory lock without blocking. It
// returns ErrLocked if another process currently holds it.
func TryAcquire(path string) (*Handle, error) {
	return acquire(path, false, false)
}

// TryAcquireShared attempts to take a shared advisory lock without blocking.
// It returns ErrLocked if an exclusive lock is currently held.
func TryAcquireShared(path string) (*Handle, error) {
	return acquire(path, false, true)
}

// AcquireContext waits for an advisory lock without blocking cancellation.
func AcquireContext(ctx context.Context, path string) (*Handle, error) {
	return acquireContext(ctx, path, false)
}

// AcquireSharedContext waits for a shared advisory lock without blocking
// cancellation.
func AcquireSharedContext(ctx context.Context, path string) (*Handle, error) {
	return acquireContext(ctx, path, true)
}

func acquireContext(ctx context.Context, path string, shared bool) (*Handle, error) {
	for {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		handle, err := acquire(path, false, shared)
		if !errors.Is(err, ErrLocked) {
			return handle, err
		}
		timer := time.NewTimer(100 * time.Millisecond)
		select {
		case <-ctx.Done():
			timer.Stop()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func acquire(path string, block, shared bool) (*Handle, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("open workspace lock %q: %w", path, err)
	}
	if err := lockFile(file, block, shared); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("acquire workspace lock %q: %w", path, err)
	}
	return &Handle{file: file}, nil
}

// Release drops the advisory lock and closes the underlying file. It is safe to
// call on a nil handle and idempotent.
func (h *Handle) Release() error {
	if h == nil || h.file == nil {
		return nil
	}
	unlockErr := unlockFile(h.file)
	closeErr := h.file.Close()
	h.file = nil
	if unlockErr != nil {
		return fmt.Errorf("release workspace lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("close workspace lock: %w", closeErr)
	}
	return nil
}
