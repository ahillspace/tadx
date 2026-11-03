//go:build !windows

package workspace

import "path/filepath"

func resolveExistingPath(path string) (string, error) {
	return filepath.EvalSymlinks(path)
}
