package lock

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// rootAttempts bounds how often TryAcquireIn reopens a lock name that another
// holder removed between the open and the lock.
const rootAttempts = 3

var errReplaced = errors.New("lock file was replaced")

// TryAcquireIn takes an exclusive advisory lock on the regular file name
// inside root without blocking, creating the file if needed. It returns
// ErrLocked while another process holds the lock.
//
// Release removes the lock file, so nothing is left behind after normal use.
// Because the operating system drops the lock when its holder exits, a file
// left by a holder that was killed is simply locked again by the next caller.
func TryAcquireIn(root *os.Root, name string) (*Handle, error) {
	for attempt := 0; attempt < rootAttempts; attempt++ {
		if info, err := root.Lstat(name); err == nil && !info.Mode().IsRegular() {
			return nil, fmt.Errorf("lock %q is not a regular file", name)
		} else if err != nil && !errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("inspect lock %q: %w", name, err)
		}
		file, err := root.OpenFile(name, os.O_CREATE|os.O_RDWR, 0o600)
		if err != nil {
			return nil, fmt.Errorf("open lock %q: %w", name, err)
		}
		handle, err := lockNamed(root, name, file)
		if !errors.Is(err, errReplaced) {
			return handle, err
		}
	}
	return nil, ErrLocked
}

// lockNamed locks file and confirms that name still refers to it. A holder
// removes the name before it releases the lock, so a caller that opened the
// old file first would otherwise hold a lock nobody else can see.
func lockNamed(root *os.Root, name string, file *os.File) (*Handle, error) {
	if err := lockFile(file, false, false); err != nil {
		_ = file.Close()
		if errors.Is(err, ErrLocked) {
			return nil, err
		}
		return nil, fmt.Errorf("acquire lock %q: %w", name, err)
	}
	held, heldErr := file.Stat()
	current, currentErr := root.Lstat(name)
	if heldErr != nil || currentErr != nil || !current.Mode().IsRegular() || !os.SameFile(held, current) {
		_ = unlockFile(file)
		_ = file.Close()
		return nil, errReplaced
	}
	return &Handle{file: file, root: root, name: name}, nil
}
