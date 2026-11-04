package artifact

import (
	"github.com/ahillspace/tadx/internal/errs"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
)

func withWorkspaceRelativePaths(err error, roots ...string) error {
	if err == nil {
		return nil
	}
	forms := append([]string(nil), roots...)
	for _, root := range roots {
		if canonical, resolveErr := workspacecore.CanonicalRoot(root); resolveErr == nil {
			forms = append(forms, canonical)
		}
	}
	return errs.WithWorkspaceRelativePaths(err, forms...)
}

func bundleWorkspaces(input WorkbookBundlePull) []string {
	roots := []string{input.Workbook.Workspace}
	for _, datasource := range input.Datasources {
		roots = append(roots, datasource.Workspace)
	}
	return roots
}
