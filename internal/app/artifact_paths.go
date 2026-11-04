package app

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

// artifactWorkspaceRelativePath projects storage results through physical path
// identity before returning a portable path to an action.
func artifactWorkspaceRelativePath(workspace, target, kind, label string) (_ string, err error) {
	root, err := workspacecore.CanonicalRoot(workspace)
	defer func() { err = errs.WithWorkspaceRelativePaths(err, workspace, root) }()
	if err != nil {
		return "", err
	}
	resolved, err := workspacecore.CanonicalRoot(target)
	if err != nil {
		return "", err
	}
	relative, err := filepath.Rel(root, resolved)
	if err != nil {
		return "", err
	}
	relative = filepath.ToSlash(relative)
	if relative == ".." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("%s artifact %s escapes the resolved workspace", kind, label)
	}
	return relative, nil
}
