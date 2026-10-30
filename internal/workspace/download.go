package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/lock"
)

// downloadLockName guards .tadx/tmp. Commands staging downloads hold it shared,
// and temporary cleanup holds it exclusively, so cleanup never removes a
// download in progress. It sits beside tmp so removing tmp keeps the lock.
const downloadLockName = "tmp.lock"

// Downloads stages one command's content downloads in a private directory
// under the workspace's disposable .tadx/tmp state. Content streams there and
// is renamed into its artifact, so it shares the workspace filesystem. The
// directory is created by the first download, so resolution and previews leave
// the workspace untouched, and Close removes it with any payload the artifact
// layer did not install. `tadx workspace clean --class temporary` removes one
// left by an interrupted command.
type Downloads struct {
	root      string
	held      *lock.Handle
	directory string
}

// NewDownloads returns staging for downloads into the workspace at root.
func NewDownloads(root string) *Downloads {
	return &Downloads{root: root}
}

// File streams one content download into a new staged file and flushes it to
// stable storage, so the artifact layer can install it by rename. A failed
// download removes its partial file.
func (d *Downloads) File(ctx context.Context, download func(io.Writer) error) (string, error) {
	if d.directory == "" {
		if err := d.open(ctx); err != nil {
			return "", err
		}
	}
	file, err := os.CreateTemp(d.directory, "payload-")
	if err != nil {
		return "", fmt.Errorf("create download file: %w", err)
	}
	if err := download(file); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", err
	}
	if err := file.Sync(); err != nil {
		_ = file.Close()
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("flush download file: %w", err)
	}
	if err := file.Close(); err != nil {
		_ = os.Remove(file.Name())
		return "", fmt.Errorf("close download file: %w", err)
	}
	return file.Name(), nil
}

// Close removes the staged downloads and releases the staging lock.
func (d *Downloads) Close() {
	if d.directory != "" {
		_ = os.RemoveAll(d.directory)
		d.directory = ""
	}
	_ = d.held.Release()
	d.held = nil
}

func (d *Downloads) open(ctx context.Context) error {
	control, err := controlDirectory(d.root, true)
	if err != nil {
		return err
	}
	held, err := lock.AcquireSharedContext(ctx, filepath.Join(control, downloadLockName))
	if err != nil {
		return fmt.Errorf("lock workspace download state: %w", err)
	}
	parent := filepath.Join(control, "tmp")
	if err := realDirectory(parent, ".tadx/tmp", true); err != nil {
		_ = held.Release()
		return err
	}
	directory, err := os.MkdirTemp(parent, "download-")
	if err != nil {
		_ = held.Release()
		return fmt.Errorf("create workspace download directory: %w", err)
	}
	d.held, d.directory = held, directory
	return nil
}

// controlDirectory returns the workspace's .tadx directory, creating it when
// create is set, and rejects a control path that is not a real directory.
func controlDirectory(root string, create bool) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	control := filepath.Join(absoluteRoot, ".tadx")
	return control, realDirectory(control, ".tadx", create)
}

func realDirectory(path, relative string, create bool) error {
	if create {
		if err := os.Mkdir(path, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return fmt.Errorf("create workspace control directory %q: %w", relative, err)
		}
	}
	info, err := os.Lstat(path)
	if err != nil {
		return fmt.Errorf("inspect workspace control directory %q: %w", relative, err)
	}
	if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
		return fmt.Errorf("workspace control path %q must be a real directory", relative)
	}
	return nil
}
