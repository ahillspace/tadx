package artifact

import "github.com/ahillspace/tadx/internal/errs"

func withWorkspaceRelativePaths(err error, roots ...string) error {
	return errs.WithWorkspaceRelativePaths(err, roots...)
}

func bundleWorkspaces(input WorkbookBundlePull) []string {
	roots := []string{input.Workbook.Workspace}
	for _, datasource := range input.Datasources {
		roots = append(roots, datasource.Workspace)
	}
	return roots
}
