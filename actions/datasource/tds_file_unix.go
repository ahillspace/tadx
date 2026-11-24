//go:build !windows

package datasource

import (
	"os"
	"syscall"
)

func editDirectory(info os.FileInfo) bool { return info.IsDir() }

func openEditInput(dir *os.Root, name string, expected os.FileInfo) (*os.File, error) {
	// Opening a FIFO substituted after the directory check must not block.
	f, err := dir.OpenFile(name, os.O_RDONLY|syscall.O_NONBLOCK|syscall.O_NOFOLLOW, 0)
	if err != nil {
		return nil, err
	}
	actual, err := f.Stat()
	if err != nil {
		f.Close()
		return nil, err
	}
	st, ok := actual.Sys().(*syscall.Stat_t)
	if !ok || st.Nlink != 1 || !actual.Mode().IsRegular() || !os.SameFile(expected, actual) {
		f.Close()
		return nil, editFileProblem("must identify one unaliased regular file")
	}
	return f, nil
}
