package workspace

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

// DownloadDir creates a private directory under the workspace's disposable
// .tadx/tmp state for one command's downloads. Content streams there and is
// renamed into its artifact, so it shares the workspace filesystem. The caller
// removes the directory when the command ends; `tadx workspace clean --class
// temporary` removes any left by an interrupted command.
func DownloadDir(root string) (string, error) {
	absoluteRoot, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolve workspace root: %w", err)
	}
	parent := absoluteRoot
	for _, relative := range []string{".tadx", ".tadx/tmp"} {
		parent = filepath.Join(absoluteRoot, filepath.FromSlash(relative))
		if err := os.Mkdir(parent, 0o700); err != nil && !errors.Is(err, os.ErrExist) {
			return "", fmt.Errorf("create workspace download directory: %w", err)
		}
		info, err := os.Lstat(parent)
		if err != nil {
			return "", fmt.Errorf("inspect workspace download directory: %w", err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.IsDir() {
			return "", fmt.Errorf("workspace control path %q must be a real directory", relative)
		}
	}
	directory, err := os.MkdirTemp(parent, "download-")
	if err != nil {
		return "", fmt.Errorf("create workspace download directory: %w", err)
	}
	return directory, nil
}
