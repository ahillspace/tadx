package catalog

import (
	"context"
	"database/sql"
	"errors"
)

type projectFilterUnavailableError struct{}

func (projectFilterUnavailableError) Error() string {
	return "project-name filtering requires complete cached project and datasource scopes with canonical content project identities; refresh projects and datasources"
}
func (projectFilterUnavailableError) CatalogProjectRefreshRequired() bool { return true }

// The projection's project_luid is an identity, never a project display path or
// its last segment. Guard JSON extraction so legacy empty payloads fail closed.
const cachedProjectIdentity = `CASE WHEN json_valid(CAST(payload AS TEXT)) THEN CASE WHEN json_type(CAST(payload AS TEXT),'$.project_luid')='text' THEN json_extract(CAST(payload AS TEXT),'$.project_luid') END END`

func cachedProjectNameFilter(ctx context.Context, tx *sql.Tx, query *ResourceQuery, meta generationMeta) (string, []any, error) {
	snapshot, err := currentResourceScopeSnapshot(ctx, tx, query.Environment, query.Site, "project")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return "", nil, err
	}
	projects := `SELECT luid FROM resource_entries WHERE environment=? AND site=? AND kind='project'`
	projectArgs := []any{query.Environment, query.Site}
	if err != nil {
		var complete bool
		err = tx.QueryRowContext(ctx, `SELECT complete FROM generation_scopes WHERE generation_key=? AND scope='projects'`, meta.key).Scan(&complete)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return "", nil, err
		}
		if !complete {
			return "", nil, projectFilterUnavailableError{}
		}
		projects = `SELECT luid FROM catalog_records WHERE generation_key=? AND kind='project'`
		projectArgs = []any{meta.key}
		query.projectSnapshot = meta.id
	} else if !snapshot.complete {
		return "", nil, projectFilterUnavailableError{}
	} else {
		query.projectSnapshot = snapshot.id
	}
	// Missing, malformed, or orphaned identities cannot be classified as a
	// non-match. Check the selected content population before applying the name.
	where, args := resourceWhere(*query)
	var missing bool
	err = tx.QueryRowContext(ctx, `SELECT EXISTS(SELECT 1 FROM resource_entries `+where+` AND COALESCE((`+cachedProjectIdentity+`) IN (`+projects+`),0)=0)`, append(args, projectArgs...)...).Scan(&missing)
	if err != nil {
		return "", nil, err
	}
	if missing {
		return "", nil, projectFilterUnavailableError{}
	}
	return `(` + cachedProjectIdentity + `) IN (` + projects + ` AND name=?)`, append(projectArgs, query.ProjectName), nil
}
