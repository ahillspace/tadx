package catalog

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"sync"
)

type GenerationWriter struct {
	store              *Store
	db                 *sql.DB
	tx                 *sql.Tx
	key                int64
	metadata           GenerationMetadata
	mu                 sync.Mutex
	closed             bool
	partialPermissions bool
	publicationTarget  *Store
	stagingDirectory   string
	stagedRows         int
}

func (w *GenerationWriter) WriteBatch(ctx context.Context, b Batch) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("catalog generation writer is closed")
	}
	expected, ok := batchColumns[b.Scope]
	if !ok {
		return fmt.Errorf("catalog scope %q is unsupported", b.Scope)
	}
	if !equalStrings(b.Columns, expected) {
		return fmt.Errorf("catalog scope %q columns must be %v", b.Scope, expected)
	}
	if len(b.Rows) > maxBatchRows {
		return fmt.Errorf("catalog scope %q batch exceeds %d-row limit", b.Scope, maxBatchRows)
	}
	if w.publicationTarget != nil && w.stagedRows+len(b.Rows) > maximumResourceScopeRows {
		return fmt.Errorf("catalog staging exceeds %d-row limit", maximumResourceScopeRows)
	}
	var admitted int
	if err := w.tx.QueryRowContext(ctx, `SELECT count(*) FROM generation_scopes WHERE generation_key=? AND scope=?`, w.key, b.Scope).Scan(&admitted); err != nil {
		return err
	}
	if admitted != 1 {
		return fmt.Errorf("catalog scope %q was not admitted for this generation", b.Scope)
	}
	statement := fmt.Sprintf("INSERT INTO %s(generation_key,%s) VALUES(%s)", b.Scope, strings.Join(expected, ","), strings.TrimSuffix(strings.Repeat("?,", len(expected)+1), ","))
	prepared, err := w.tx.PrepareContext(ctx, statement)
	if err != nil {
		return err
	}
	defer prepared.Close()
	for index, row := range b.Rows {
		if len(row) != len(expected) {
			return fmt.Errorf("catalog scope %q row %d has %d values, expected %d", b.Scope, index, len(row), len(expected))
		}
		if err := validateBatchRow(b.Scope, index, row); err != nil {
			return err
		}
		if _, err := prepared.ExecContext(ctx, append([]any{w.key}, row...)...); err != nil {
			return fmt.Errorf("insert catalog scope %q row %d: %w", b.Scope, index, err)
		}
		w.stagedRows++
	}
	return nil
}
func (w *GenerationWriter) CompleteScopes(ctx context.Context, scopes []string) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("catalog generation writer is closed")
	}
	for _, scope := range normalizedScopes(scopes, false) {
		if scope == "permissions" && w.partialPermissions {
			return errors.New("catalog permission coverage remains incomplete after denied reads")
		}
		result, err := w.tx.ExecContext(ctx, `UPDATE generation_scopes SET complete=1 WHERE generation_key=? AND scope=?`, w.key, scope)
		if err != nil {
			return err
		}
		count, err := result.RowsAffected()
		if err != nil {
			return err
		}
		if count != 1 {
			return fmt.Errorf("catalog scope %q was not admitted for this generation", scope)
		}
	}
	return nil
}

// MarkPermissionsIncomplete permits publication of useful inventory after
// resource-specific permission denials. The permission scope stays incomplete.
func (w *GenerationWriter) MarkPermissionsIncomplete(ctx context.Context) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return errors.New("catalog generation writer is closed")
	}
	var complete bool
	if err := w.tx.QueryRowContext(ctx, `SELECT complete FROM generation_scopes WHERE generation_key=? AND scope='permissions'`, w.key).Scan(&complete); err != nil {
		return fmt.Errorf("read admitted permission scope: %w", err)
	}
	if complete {
		return errors.New("catalog permission scope is already marked complete")
	}
	w.partialPermissions = true
	return nil
}

func (w *GenerationWriter) Publish(ctx context.Context) (ReplaceResult, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.closed {
		return ReplaceResult{}, errors.New("catalog generation writer is closed")
	}
	var incomplete int
	if err := w.tx.QueryRowContext(ctx, `SELECT count(*) FROM generation_scopes WHERE generation_key=? AND complete=0 AND NOT (scope='permissions' AND ?=1)`, w.key, w.partialPermissions).Scan(&incomplete); err != nil {
		return ReplaceResult{}, err
	}
	if incomplete != 0 {
		return ReplaceResult{}, fmt.Errorf("catalog generation has %d incomplete scopes", incomplete)
	}
	if err := w.buildCatalogRecords(ctx); err != nil {
		return ReplaceResult{}, err
	}
	count, err := w.totalHydratedRows(ctx)
	if err != nil {
		return ReplaceResult{}, err
	}
	fingerprint, err := w.fingerprint(ctx)
	if err != nil {
		return ReplaceResult{}, err
	}
	return w.publishReady(ctx, count, fingerprint)
}

// publishReady promotes already normalized and fingerprinted local rows.
// Refresh runs this work in staging before acquiring the active write lock.
func (w *GenerationWriter) publishReady(ctx context.Context, count int, fingerprint string) (ReplaceResult, error) {
	id := w.metadata.ID
	if id == "" {
		id = "sha256:" + fingerprint
	}
	if err := validateField("generation ID", id); err != nil {
		return ReplaceResult{}, err
	}
	var existingKey int64
	var existingFingerprint string
	err := w.tx.QueryRowContext(ctx, `SELECT generation_key,fingerprint FROM generations WHERE environment=? AND site=? AND id=?`, w.metadata.Environment, w.metadata.Site, id).Scan(&existingKey, &existingFingerprint)
	if err == nil {
		if existingFingerprint != fingerprint {
			return ReplaceResult{}, fmt.Errorf("catalog generation ID %q already identifies different content", id)
		}
		if _, err := w.tx.ExecContext(ctx, `DELETE FROM generations WHERE generation_key=?`, w.key); err != nil {
			return ReplaceResult{}, err
		}
		w.key = existingKey
		if err := w.tx.QueryRowContext(ctx, `SELECT record_count FROM generations WHERE generation_key=?`, w.key).Scan(&count); err != nil {
			return ReplaceResult{}, err
		}
	} else if !errors.Is(err, sql.ErrNoRows) {
		return ReplaceResult{}, err
	} else {
		// This id is new for the environment/site, but the same content may already
		// be published under a different id. Promoting our fingerprint would violate
		// UNIQUE(environment,site,fingerprint) and surface a raw driver error, so
		// detect the collision and return a typed error instead of reconciling.
		var conflictID string
		conflictErr := w.tx.QueryRowContext(ctx, `SELECT id FROM generations WHERE environment=? AND site=? AND fingerprint=?`, w.metadata.Environment, w.metadata.Site, fingerprint).Scan(&conflictID)
		if conflictErr == nil {
			return ReplaceResult{}, duplicateContentError{conflictID}
		}
		if !errors.Is(conflictErr, sql.ErrNoRows) {
			return ReplaceResult{}, conflictErr
		}
		if _, err := w.tx.ExecContext(ctx, `UPDATE generations SET id=?,fingerprint=?,complete=1,record_count=? WHERE generation_key=?`, id, fingerprint, count, w.key); err != nil {
			return ReplaceResult{}, err
		}
	}
	if _, err := w.tx.ExecContext(ctx, `INSERT INTO current_generations(environment,site,generation_key) VALUES(?,?,?) ON CONFLICT(environment,site) DO UPDATE SET generation_key=excluded.generation_key`, w.metadata.Environment, w.metadata.Site, w.key); err != nil {
		return ReplaceResult{}, err
	}
	if _, err := w.tx.ExecContext(ctx, `DELETE FROM generations WHERE environment=? AND site=? AND generation_key<>?`, w.metadata.Environment, w.metadata.Site, w.key); err != nil {
		return ReplaceResult{}, fmt.Errorf("prune superseded catalog generations: %w", err)
	}
	if err := w.replaceResourceEntries(ctx, id); err != nil {
		return ReplaceResult{}, err
	}
	if err := checkIntegrity(ctx, w.tx); err != nil {
		return ReplaceResult{}, err
	}
	if err := w.tx.Commit(); err != nil {
		return ReplaceResult{}, err
	}
	if w.publicationTarget != nil {
		defer os.RemoveAll(w.stagingDirectory)
	}
	if err := w.db.Close(); err != nil {
		return ReplaceResult{}, err
	}
	w.closed = true
	if w.publicationTarget != nil {
		return w.publicationTarget.publishStaged(ctx, w, count, fingerprint)
	}
	return ReplaceResult{id, databaseRelativePath, count}, nil
}

func (w *GenerationWriter) replaceResourceEntries(ctx context.Context, generationID string) error {
	requested, err := w.requestedScopes(ctx)
	if err != nil {
		return err
	}
	selectedKinds := `SELECT CASE scope WHEN 'users' THEN 'user' WHEN 'groups' THEN 'group' WHEN 'projects' THEN 'project' WHEN 'workbooks' THEN 'workbook' WHEN 'datasources' THEN 'datasource' WHEN 'flows' THEN 'flow' WHEN 'views' THEN 'view' END FROM generation_scopes WHERE generation_key=? AND requested=1 AND scope<>'permissions'`
	if _, err := w.tx.ExecContext(ctx, `DELETE FROM resource_entries WHERE environment=? AND site=? AND kind IN (`+selectedKinds+`)`, w.metadata.Environment, w.metadata.Site, w.key); err != nil {
		return fmt.Errorf("replace catalog resource entries: %w", err)
	}
	_, err = w.tx.ExecContext(ctx, `INSERT INTO resource_entries(environment,site,kind,luid,name,project_path,project_luid,owner,payload,coverage,observed_at)
		SELECT ?,?,kind,luid,name,project_path,'',owner,X'','summary',? FROM catalog_records WHERE generation_key=? AND requested=1`,
		w.metadata.Environment, w.metadata.Site, w.metadata.GeneratedAt.UTC().Format(generationTimeLayout), w.key)
	if err != nil {
		return fmt.Errorf("seed catalog resource entries: %w", err)
	}
	for _, scope := range []string{"workbooks", "datasources", "flows"} {
		if !requested[scope] {
			continue
		}
		if _, err := w.tx.ExecContext(ctx, "UPDATE resource_entries SET project_luid=COALESCE((SELECT project_id FROM "+scope+" WHERE generation_key=? AND id=resource_entries.luid),'') WHERE environment=? AND site=? AND kind=?", w.key, w.metadata.Environment, w.metadata.Site, strings.TrimSuffix(scope, "s")); err != nil {
			return err
		}
	}
	// Preserve the complete list projection and add canonical identity fields.
	for _, scope := range []string{"users", "groups", "projects", "workbooks", "datasources", "flows"} {
		if !requested[scope] {
			continue
		}
		kind := strings.TrimSuffix(scope, "s")
		_, err := w.tx.ExecContext(ctx, `UPDATE resource_entries SET payload=CAST(json_set(
			(SELECT list_payload FROM `+scope+` WHERE generation_key=? AND id=resource_entries.luid),
			'$.luid',luid,'$.name',name,'$.project_path',project_path,'$.owner_luid',owner) AS BLOB)
			WHERE environment=? AND site=? AND kind=? AND EXISTS
			(SELECT 1 FROM `+scope+` WHERE generation_key=? AND id=resource_entries.luid)`,
			w.key, w.metadata.Environment, w.metadata.Site, kind, w.key)
		if err != nil {
			return fmt.Errorf("seed catalog %s list payloads: %w", kind, err)
		}
	}
	if _, err := w.tx.ExecContext(ctx, `DELETE FROM resource_scope_snapshots WHERE environment=? AND site=? AND kind IN (`+selectedKinds+`)`, w.metadata.Environment, w.metadata.Site, w.key); err != nil {
		return fmt.Errorf("replace catalog resource scope snapshots: %w", err)
	}
	_, err = w.tx.ExecContext(ctx, `INSERT INTO resource_scope_snapshots(environment,site,kind,generation_id,generated_at,complete,source,record_count)
		SELECT ?,?,
			CASE scope WHEN 'users' THEN 'user' WHEN 'groups' THEN 'group' WHEN 'projects' THEN 'project' WHEN 'workbooks' THEN 'workbook' WHEN 'datasources' THEN 'datasource' WHEN 'flows' THEN 'flow' WHEN 'views' THEN 'view' END,
			?,?,complete,?,
			(SELECT count(*) FROM catalog_records r WHERE r.generation_key=? AND r.requested=1 AND r.kind=CASE generation_scopes.scope WHEN 'users' THEN 'user' WHEN 'groups' THEN 'group' WHEN 'projects' THEN 'project' WHEN 'workbooks' THEN 'workbook' WHEN 'datasources' THEN 'datasource' WHEN 'flows' THEN 'flow' WHEN 'views' THEN 'view' END)
		FROM generation_scopes WHERE generation_key=? AND requested=1 AND scope<>'permissions'`,
		w.metadata.Environment, w.metadata.Site, generationID, w.metadata.GeneratedAt.UTC().Format(generationTimeLayout), w.metadata.Source, w.key, w.key)
	if err != nil {
		return fmt.Errorf("seed catalog resource scope snapshots: %w", err)
	}
	return nil
}
func (w *GenerationWriter) Rollback() error {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.stagingDirectory != "" {
		defer os.RemoveAll(w.stagingDirectory)
	}
	if w.closed {
		return nil
	}
	w.closed = true
	err := w.tx.Rollback()
	closeErr := w.db.Close()
	if err != nil && !errors.Is(err, sql.ErrTxDone) {
		return err
	}
	return closeErr
}

func (w *GenerationWriter) writeRecords(ctx context.Context, records []Record) error {
	w.mu.Lock()
	defer w.mu.Unlock()
	statement, err := w.tx.PrepareContext(ctx, `INSERT INTO catalog_records(generation_key,luid,kind,name,project_path,owner,requested) VALUES(?,?,?,?,?,?,1)`)
	if err != nil {
		return err
	}
	defer statement.Close()
	copyRecords := append([]Record(nil), records...)
	sortRecords(copyRecords)
	for index, r := range copyRecords {
		if strings.TrimSpace(r.LUID) == "" || strings.TrimSpace(r.Kind) == "" || strings.TrimSpace(r.Name) == "" {
			return fmt.Errorf("catalog record %d requires LUID, kind, and name", index)
		}
		for name, value := range map[string]string{"LUID": r.LUID, "kind": r.Kind, "name": r.Name, "project path": r.ProjectPath, "owner": r.Owner} {
			if err := validateField(fmt.Sprintf("catalog record %d %s", index, name), value); err != nil {
				return err
			}
		}
		if _, err := statement.ExecContext(ctx, w.key, r.LUID, r.Kind, r.Name, r.ProjectPath, r.Owner); err != nil {
			return err
		}
	}
	return nil
}

func (w *GenerationWriter) buildCatalogRecords(ctx context.Context) error {
	paths, err := w.projectPaths(ctx)
	if err != nil {
		return err
	}
	requested, err := w.requestedScopes(ctx)
	if err != nil {
		return err
	}
	insert, err := w.tx.PrepareContext(ctx, `INSERT INTO catalog_records(generation_key,luid,kind,name,project_path,owner,requested) VALUES(?,?,?,?,?,?,?)`)
	if err != nil {
		return err
	}
	defer insert.Close()
	type resource struct {
		scope, kind, query string
		relation           func([]string) (string, string, error)
	}
	empty := func([]string) (string, string, error) { return "", "", nil }
	resources := []resource{
		{"users", "user", `SELECT id,name FROM users WHERE generation_key=? ORDER BY id`, empty},
		{"groups", "group", `SELECT id,name FROM groups WHERE generation_key=? ORDER BY id`, empty},
		{"projects", "project", `SELECT id,name,id,owner_id FROM projects WHERE generation_key=? ORDER BY id`, func(v []string) (string, string, error) { return paths[v[2]], v[3], nil }},
		{"workbooks", "workbook", `SELECT id,name,project_id,owner_id FROM workbooks WHERE generation_key=? ORDER BY id`, relation(paths)},
		{"datasources", "datasource", `SELECT id,name,project_id,owner_id FROM datasources WHERE generation_key=? ORDER BY id`, relation(paths)},
		{"flows", "flow", `SELECT id,name,project_id,owner_id FROM flows WHERE generation_key=? ORDER BY id`, relation(paths)},
	}
	for _, resource := range resources {
		rows, err := w.tx.QueryContext(ctx, resource.query, w.key)
		if err != nil {
			return err
		}
		for rows.Next() {
			values, err := scanStrings(rows)
			if err != nil {
				rows.Close()
				return err
			}
			path, owner, err := resource.relation(values)
			if err != nil {
				rows.Close()
				return err
			}
			if _, err := insert.ExecContext(ctx, w.key, values[0], resource.kind, values[1], path, owner, requested[resource.scope]); err != nil {
				rows.Close()
				return err
			}
		}
		if err := rows.Close(); err != nil {
			return err
		}
		if err := rows.Err(); err != nil {
			return err
		}
	}
	rows, err := w.tx.QueryContext(ctx, `SELECT v.id,v.name,w.project_id,w.owner_id FROM views v LEFT JOIN workbooks w ON w.generation_key=v.generation_key AND w.id=v.workbook_id WHERE v.generation_key=? ORDER BY v.id`, w.key)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id, name string
		var projectID, ownerID sql.NullString
		if err := rows.Scan(&id, &name, &projectID, &ownerID); err != nil {
			rows.Close()
			return err
		}
		if !projectID.Valid {
			rows.Close()
			return fmt.Errorf("view %q references a workbook outside the generation", id)
		}
		if _, err := insert.ExecContext(ctx, w.key, id, "view", name, paths[projectID.String], ownerID.String, requested["views"]); err != nil {
			rows.Close()
			return err
		}
	}
	if err := rows.Close(); err != nil {
		return err
	}
	return rows.Err()
}
func (w *GenerationWriter) requestedScopes(ctx context.Context) (map[string]bool, error) {
	rows, err := w.tx.QueryContext(ctx, `SELECT scope,requested FROM generation_scopes WHERE generation_key=?`, w.key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := map[string]bool{}
	for rows.Next() {
		var scope string
		var requested bool
		if err := rows.Scan(&scope, &requested); err != nil {
			return nil, err
		}
		result[scope] = requested
	}
	return result, rows.Err()
}
func (w *GenerationWriter) projectPaths(ctx context.Context) (map[string]string, error) {
	rows, err := w.tx.QueryContext(ctx, `SELECT id,name,parent_project_id FROM projects WHERE generation_key=?`, w.key)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	type project struct{ name, parent string }
	projects := map[string]project{}
	for rows.Next() {
		var id, name, parent string
		if err := rows.Scan(&id, &name, &parent); err != nil {
			return nil, err
		}
		// Preserve display paths, including slash-containing names. Distinct
		// project LUIDs can share a display path without losing catalog records.
		projects[id] = project{name, parent}
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	paths := map[string]string{}
	visiting := map[string]bool{}
	var resolve func(string) (string, error)
	resolve = func(id string) (string, error) {
		if id == "" {
			return "", nil
		}
		if path, ok := paths[id]; ok {
			return path, nil
		}
		p, ok := projects[id]
		if !ok {
			return "", fmt.Errorf("project %q is outside the generation", id)
		}
		if visiting[id] {
			return "", fmt.Errorf("project hierarchy contains a cycle at %q", id)
		}
		visiting[id] = true
		parent, err := resolve(p.parent)
		if err != nil {
			return "", err
		}
		delete(visiting, id)
		if parent == "" {
			paths[id] = p.name
		} else {
			paths[id] = parent + "/" + p.name
		}
		return paths[id], nil
	}
	for id := range projects {
		if _, err := resolve(id); err != nil {
			return nil, err
		}
	}
	return paths, nil
}
func relation(paths map[string]string) func([]string) (string, string, error) {
	return func(v []string) (string, string, error) {
		path, ok := paths[v[2]]
		if v[2] != "" && !ok {
			return "", "", fmt.Errorf("content %q references project %q outside the generation", v[0], v[2])
		}
		return path, v[3], nil
	}
}
func scanStrings(rows *sql.Rows) ([]string, error) {
	columns, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	values := make([]string, len(columns))
	targets := make([]any, len(columns))
	for i := range values {
		targets[i] = &values[i]
	}
	if err := rows.Scan(targets...); err != nil {
		return nil, err
	}
	return values, nil
}

// totalHydratedRows reports the number of gettable catalog records. It counts
// catalog_records (built by buildCatalogRecords, or written directly by the
// Replace path) rather than summing the typed scope tables: the permissions
// scope populates its own table but never yields a catalog_record, so summing
// scopes would inflate RecordCount past what Search/Get can return.
func (w *GenerationWriter) totalHydratedRows(ctx context.Context) (int, error) {
	var total int
	if err := w.tx.QueryRowContext(ctx, `SELECT count(*) FROM catalog_records WHERE generation_key=?`, w.key).Scan(&total); err != nil {
		return 0, fmt.Errorf("count catalog records: %w", err)
	}
	return total, nil
}

func (w *GenerationWriter) fingerprint(ctx context.Context) (string, error) {
	// Stream the same canonical JSON representation as the original map encoding,
	// retaining content identities without materializing the full inventory.
	metadata := w.metadata
	metadata.ID = ""
	encoded, err := json.Marshal(metadata)
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	io.WriteString(hash, `{"Metadata":`)
	hash.Write(encoded)
	io.WriteString(hash, `,"Tables":{`)
	tables := make([]string, 0, len(batchColumns)+1)
	for table := range batchColumns {
		tables = append(tables, table)
	}
	tables = append(tables, "catalog_records")
	sort.Strings(tables)
	firstTable := true
	for _, table := range tables {
		columns := batchColumns[table]
		order := strings.Join(columns, ",")
		if table == "catalog_records" {
			columns = []string{"luid", "kind", "name", "project_path", "owner", "requested"}
			order = "kind,name,project_path,luid"
		}
		rows, err := w.tx.QueryContext(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE generation_key=? ORDER BY %s", strings.Join(columns, ","), table, order), w.key)
		if err != nil {
			return "", err
		}
		firstRow := true
		for rows.Next() {
			values := make([]any, len(columns))
			targets := make([]any, len(columns))
			for index := range values {
				targets[index] = &values[index]
			}
			if err := rows.Scan(targets...); err != nil {
				rows.Close()
				return "", err
			}
			if firstRow {
				if !firstTable {
					io.WriteString(hash, ",")
				}
				name, _ := json.Marshal(table)
				hash.Write(name)
				io.WriteString(hash, ":[")
				firstTable = false
			} else {
				io.WriteString(hash, ",")
			}
			value, err := json.Marshal(values)
			if err != nil {
				rows.Close()
				return "", err
			}
			hash.Write(value)
			firstRow = false
		}
		if err := rows.Err(); err != nil {
			rows.Close()
			return "", err
		}
		if err := rows.Close(); err != nil {
			return "", err
		}
		if !firstRow {
			io.WriteString(hash, "]")
		}
	}
	io.WriteString(hash, "}")
	if w.partialPermissions {
		io.WriteString(hash, `,"PartialPermissions":true`)
	}
	io.WriteString(hash, "}")
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func validateBatchRow(scope string, index int, row []any) error {
	required := 2
	if scope == "permissions" {
		required = 6
	}
	for i := 0; i < required; i++ {
		value, ok := row[i].(string)
		if !ok || strings.TrimSpace(value) == "" {
			return fmt.Errorf("catalog scope %q row %d column %q is required", scope, index, batchColumns[scope][i])
		}
	}
	for i, value := range row {
		if text, ok := value.(string); ok {
			if err := validateField(fmt.Sprintf("catalog scope %q row %d column %q", scope, index, batchColumns[scope][i]), text); err != nil {
				return err
			}
		}
	}
	return nil
}
func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
func sortRecords(records []Record) {
	sort.Slice(records, func(i, j int) bool {
		if records[i].Kind != records[j].Kind {
			return records[i].Kind < records[j].Kind
		}
		if records[i].Name != records[j].Name {
			return records[i].Name < records[j].Name
		}
		if records[i].ProjectPath != records[j].ProjectPath {
			return records[i].ProjectPath < records[j].ProjectPath
		}
		return records[i].LUID < records[j].LUID
	})
}
