package workspace

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/lock"
)

// downloadLockName guards .tadx/tmp. Commands staging downloads hold it shared,
// and temporary cleanup holds it exclusively, so cleanup never removes a
// download in progress. It sits beside tmp so removing tmp keeps the lock.
const downloadLockName = "tmp.lock"

// Downloads owns one command's private download directory. Installable content
// shares the workspace filesystem so the artifact layer can rename its payload;
// temporary-only inspection uses private OS temporary storage. The first file
// creates the directory, and Close removes any payload not installed elsewhere.
type Downloads struct {
	root      string
	temporary bool
	held      *lock.Handle
	directory string
}

// NewDownloads returns staging for downloads into the workspace at root.
func NewDownloads(root string) *Downloads {
	return &Downloads{root: root}
}

// NewTemporaryDownloads returns command-private staging for inspection that
// must not create workspace state. Its files are not installed into artifacts.
func NewTemporaryDownloads() *Downloads {
	return &Downloads{temporary: true}
}

// File streams one content download into a new staged file and flushes it to
// stable storage, so the artifact layer can install it by rename. A failed
// download removes its partial file.
func (d *Downloads) File(ctx context.Context, download func(io.Writer) error) (_ string, err error) {
	defer func() {
		root := d.root
		if d.temporary {
			root = d.directory
			if root == "" {
				root = os.TempDir()
			}
		}
		absolute, _ := filepath.Abs(root)
		err = errs.WithWorkspaceRelativePaths(err, root, absolute)
	}()
	if err := ctx.Err(); err != nil {
		return "", err
	}
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
	if d.temporary {
		directory, err := os.MkdirTemp("", "tadx-preview-download-")
		if err != nil {
			return fmt.Errorf("create temporary download directory: %w", err)
		}
		d.directory = directory
		return nil
	}
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
