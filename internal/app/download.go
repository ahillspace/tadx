package app

import (
	"fmt"
	"io"
	"os"

	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

// downloadStaging holds one command's content downloads under the workspace's
// .tadx/tmp. The directory is created by the first download, so resolution
// and previews leave the workspace untouched, and Close removes it with any
// payload the artifact layer did not install.
type downloadStaging struct {
	workspace string
	directory string
}

func newDownloadStaging(workspace string) *downloadStaging {
	return &downloadStaging{workspace: workspace}
}

// file streams one content download into a new staged file and flushes it to
// stable storage, so the artifact layer can install it by rename. A failed
// download removes its partial file.
func (s *downloadStaging) file(download func(io.Writer) error) (string, error) {
	if s.directory == "" {
		directory, err := workspacecore.DownloadDir(s.workspace)
		if err != nil {
			return "", err
		}
		s.directory = directory
	}
	file, err := os.CreateTemp(s.directory, "payload-")
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

func (s *downloadStaging) Close() {
	if s.directory != "" {
		_ = os.RemoveAll(s.directory)
	}
}
