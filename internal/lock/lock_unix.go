//go:build unix

package lock

import (
	"errors"
	"os"

	"golang.org/x/sys/unix"
)

func lockFile(file *os.File, block bool) error {
	how := unix.LOCK_EX
	if !block {
		how |= unix.LOCK_NB
	}
	for {
		err := unix.Flock(int(file.Fd()), how)
		if err == nil {
			return nil
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if errors.Is(err, unix.EWOULDBLOCK) {
			return ErrLocked
		}
		return err
	}
}

func unlockFile(file *os.File) error {
	return unix.Flock(int(file.Fd()), unix.LOCK_UN)
}
