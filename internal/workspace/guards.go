package workspace

import (
	"context"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/config"
)

// SameRoot compares exact workspace roots, including aliases of one filesystem entry.
func SameRoot(left, right string) bool {
	leftPath, leftErr := filepath.Abs(left)
	rightPath, rightErr := filepath.Abs(right)
	if leftErr != nil || rightErr != nil {
		return false
	}
	leftPath, rightPath = filepath.Clean(leftPath), filepath.Clean(rightPath)
	if leftPath == rightPath {
		return true
	}
	leftInfo, leftErr := os.Stat(leftPath)
	rightInfo, rightErr := os.Stat(rightPath)
	return leftErr == nil && rightErr == nil && os.SameFile(leftInfo, rightInfo)
}

// HasUnmanagedEntries bounds deletion inspection and treats unknown files as dirty.
func HasUnmanagedEntries(ctx context.Context, root string, managedPaths []string) (bool, error) {
	const maxEntries = 10000
	entries := 0
	managedKinds := map[string]bool{"workbook": true, "datasource": true, "flow": true, "pulse-definition": true, "lineage": true}
	managedFiles := map[string]bool{}
	managedDirectories := map[string]bool{}
	for _, relative := range managedPaths {
		managedFiles[relative] = true
		for directory := filepath.ToSlash(filepath.Dir(filepath.FromSlash(relative))); directory != "."; directory = filepath.ToSlash(filepath.Dir(filepath.FromSlash(directory))) {
			managedDirectories[directory] = true
		}
	}
	dirty := false
	err := filepath.WalkDir(root, func(path string, entry fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		entries++
		if entries > maxEntries {
			return errors.New("workspace deletion inspection exceeds its bounded entry limit")
		}
		if path == root {
			return nil
		}
		relative, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		relative = filepath.ToSlash(relative)
		parts := strings.Split(relative, "/")
		if entry.Type()&os.ModeSymlink != 0 {
			dirty = true
			if entry.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		switch parts[0] {
		case config.WorkspaceConfigName, ".tadx.lock":
			if len(parts) != 1 || !entry.Type().IsRegular() {
				dirty = true
			}
		case ".tadx":
			if len(parts) == 1 {
				return nil
			}
		case "artifacts":
			if len(parts) == 1 && entry.IsDir() {
				return nil
			}
			if len(parts) >= 2 && managedKinds[parts[1]] {
				if len(parts) == 2 && entry.IsDir() {
					return nil
				}
				if parts[1] == "lineage" && len(parts) == 3 && entry.IsDir() && (parts[2] == "workbook" || parts[2] == "published_datasource" || parts[2] == "flow") {
					return nil
				}
				if managedDirectories[relative] && entry.IsDir() {
					return nil
				}
				if managedFiles[relative] && entry.Type().IsRegular() {
					return nil
				}
			}
			dirty = true
			if entry.IsDir() {
				return filepath.SkipDir
			}
		default:
			dirty = true
			if entry.IsDir() && len(parts) == 1 {
				return filepath.SkipDir
			}
		}
		return nil
	})
	return dirty, err
}
