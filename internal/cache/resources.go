package cache

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UpsertResources records successful live reads and invalidates any complete
// per-kind snapshot they can no longer exactly represent.
func (s *Store) UpsertResources(ctx context.Context, entries []ResourceEntry) error {
	if len(entries) == 0 {
		return nil
	}
	db, err := s.open(ctx)
	if err != nil {
		return err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	statement, err := tx.PrepareContext(ctx, `INSERT INTO resource_entries(environment,site,kind,luid,name,project_path,project_luid,owner,payload,coverage,observed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(environment,site,kind,luid) DO UPDATE SET
		name=excluded.name,project_path=excluded.project_path,project_luid=excluded.project_luid,owner=excluded.owner,
		payload=excluded.payload,coverage=excluded.coverage,observed_at=excluded.observed_at`)
	if err != nil {
		return err
	}
	defer statement.Close()
	touched := make(map[[3]string]struct{})
	for index, entry := range entries {
		entry.Environment = strings.TrimSpace(entry.Environment)
		entry.Site = strings.TrimSpace(entry.Site)
		entry.Kind = strings.TrimSpace(entry.Kind)
		entry.LUID = strings.TrimSpace(entry.LUID)
		if entry.Environment == "" || entry.Kind == "" || entry.LUID == "" || strings.TrimSpace(entry.Name) == "" || entry.ObservedAt.IsZero() {
			return fmt.Errorf("cache resource entry %d requires environment, kind, LUID, name, and observation time", index)
		}
		if entry.Coverage != "summary" && entry.Coverage != "detail" {
			return fmt.Errorf("cache resource entry %d has unsupported coverage %q", index, entry.Coverage)
		}
		if entry.Payload == nil {
			entry.Payload = []byte{}
		}
		if _, err := statement.ExecContext(ctx, entry.Environment, entry.Site, entry.Kind, entry.LUID, entry.Name, entry.ProjectPath, entry.ProjectLUID, entry.Owner, entry.Payload, entry.Coverage, entry.ObservedAt.UTC().Format(generationTimeLayout)); err != nil {
			return fmt.Errorf("upsert cache resource entry %d: %w", index, err)
		}
		touched[[3]string{entry.Environment, entry.Site, entry.Kind}] = struct{}{}
	}
	for key := range touched {
		if _, err := tx.ExecContext(ctx, `UPDATE resource_scope_snapshots SET complete=0 WHERE environment=? AND site=? AND kind=?`, key[0], key[1], key[2]); err != nil {
			return fmt.Errorf("invalidate cache %s snapshot: %w", key[2], err)
		}
	}
	return tx.Commit()
}

// ReadResources returns one bounded local resource page without contacting Tableau.
func (s *Store) ReadResources(ctx context.Context, query ResourceQuery) (ResourceResult, error) {
	query, err := normalizedResourceQuery(query)
	if err != nil {
		return ResourceResult{}, err
	}
	session, err := s.OpenResourceRead(ctx, query.Environment, query.Site)
	if err != nil {
		return ResourceResult{}, err
	}
	defer session.Close()
	return session.readResources(ctx, query)
}

// ResourceReadSession owns a target-bound, consistent cache snapshot for one
// command. Use it sequentially and close it before the command returns.
// Other commands may continue publishing cache generations through SQLite WAL.
type ResourceReadSession struct {
	store             *Store
	db                *sql.DB
	tx                *sql.Tx
	environment, site string
	plans             map[ResourceQuery]resourceReadPlan
}

type resourceReadPlan struct {
	query  ResourceQuery
	where  string
	args   []any
	result ResourceResult
}

// OpenResourceRead validates the cache once and begins a bounded read snapshot.
// Its context controls ownership even if the caller is cancelled before Close.
func (s *Store) OpenResourceRead(ctx context.Context, environment, site string) (*ResourceReadSession, error) {
	environment, site = strings.TrimSpace(environment), strings.TrimSpace(site)
	if environment == "" {
		return nil, errors.New("cache resource read requires environment")
	}
	db, err := s.open(ctx)
	if err != nil {
		return nil, err
	}
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		_ = db.Close()
		return nil, err
	}
	return &ResourceReadSession{store: s, db: db, tx: tx, environment: environment, site: site, plans: make(map[ResourceQuery]resourceReadPlan)}, nil
}

// Close releases the read transaction and its database connections.
func (s *ResourceReadSession) Close() error {
	if s == nil || s.tx == nil {
		return nil
	}
	err := s.tx.Rollback()
	if errors.Is(err, sql.ErrTxDone) {
		err = nil
	}
	s.tx = nil
	return errors.Join(err, s.db.Close())
}

// ReadResources returns a page within this session's unchanged snapshot.
// Matching scope counts and provenance are acquired once per query shape.
func (s *ResourceReadSession) ReadResources(ctx context.Context, query ResourceQuery) (ResourceResult, error) {
	query, err := normalizedResourceQuery(query)
	if err != nil {
		return ResourceResult{}, err
	}
	return s.readResources(ctx, query)
}

func normalizedResourceQuery(query ResourceQuery) (ResourceQuery, error) {
	query.Environment, query.Site, query.Kind = strings.TrimSpace(query.Environment), strings.TrimSpace(query.Site), strings.TrimSpace(query.Kind)
	if query.Environment == "" || query.Kind == "" {
		return ResourceQuery{}, errors.New("cache resource read requires environment and kind")
	}
	if query.Offset < 0 || query.Limit < 1 || query.Limit > maxLimit {
		return ResourceQuery{}, fmt.Errorf("cache resource limit must be between 1 and %d and offset must be nonnegative", maxLimit)
	}
	return query, nil
}

func (s *ResourceReadSession) readResources(ctx context.Context, query ResourceQuery) (ResourceResult, error) {
	if s == nil || s.tx == nil {
		return ResourceResult{}, sql.ErrTxDone
	}
	if err := ctx.Err(); err != nil {
		return ResourceResult{}, err
	}
	if query.Environment != s.environment || query.Site != s.site {
		return ResourceResult{}, errors.New("cache read session target does not match query")
	}
	if strings.HasPrefix(query.Cursor, partialInventoryCursorPrefix) {
		return s.readPartialInventory(ctx, query)
	}
	key := query
	key.Offset, key.Limit, key.Cursor, key.ExactlyOne = 0, 0, "", false
	plan, ok := s.plans[key]
	if !ok {
		var err error
		plan, err = s.prepare(ctx, key)
		if err != nil {
			return ResourceResult{}, err
		}
		s.plans[key] = plan
	}
	query.projectSnapshot = plan.query.projectSnapshot
	if query.Cursor != "" {
		if query.Offset != 0 || plan.result.Coverage != "complete" || plan.result.GenerationID == "" {
			return ResourceResult{}, invalidCursorError{}
		}
		generation, fingerprint, offset, err := decodeCursor(query.Cursor)
		if err != nil || generation != plan.result.GenerationID || fingerprint != resourceQueryFingerprint(query) || offset < 0 {
			return ResourceResult{}, invalidCursorError{}
		}
		query.Offset = offset
	}
	args := append(append([]any(nil), plan.args...), query.Limit, query.Offset)
	rows, err := s.tx.QueryContext(ctx, `SELECT environment,site,kind,luid,name,project_path,project_luid,owner,payload,coverage,observed_at FROM resource_entries `+plan.where+` ORDER BY name,project_path,luid LIMIT ? OFFSET ?`, args...)
	if err != nil {
		return ResourceResult{}, err
	}
	defer rows.Close()
	result := plan.result
	for rows.Next() {
		var entry ResourceEntry
		var observed string
		if err := rows.Scan(&entry.Environment, &entry.Site, &entry.Kind, &entry.LUID, &entry.Name, &entry.ProjectPath, &entry.ProjectLUID, &entry.Owner, &entry.Payload, &entry.Coverage, &observed); err != nil {
			return ResourceResult{}, err
		}
		entry.ObservedAt, err = time.Parse(generationTimeLayout, observed)
		if err != nil {
			return ResourceResult{}, err
		}
		result.Entries = append(result.Entries, entry)
		if result.NewestObserved.IsZero() || entry.ObservedAt.After(result.NewestObserved) {
			result.NewestObserved = entry.ObservedAt
		}
		if plan.result.Coverage != "complete" {
			stale, _ := staleness(s.store.now, entry.ObservedAt, "read-through")
			result.Stale = result.Stale || stale
		}
	}
	if err := rows.Err(); err != nil {
		return ResourceResult{}, err
	}
	if query.ExactlyOne || query.LUID != "" {
		if len(result.Entries) == 0 {
			return ResourceResult{}, resourceNotFoundError{}
		}
		if result.Total > 1 {
			return ResourceResult{}, ambiguousSelectorError{}
		}
	}
	if plan.result.Coverage == "complete" && query.Offset+len(result.Entries) < result.Total {
		result.NextCursor = encodeCursor(result.GenerationID, resourceQueryFingerprint(query), query.Offset+len(result.Entries))
	}
	if err := ctx.Err(); err != nil {
		return ResourceResult{}, err
	}
	return result, nil
}

func (s *ResourceReadSession) prepare(ctx context.Context, query ResourceQuery) (resourceReadPlan, error) {
	tx := s.tx
	snapshot, snapshotErr := currentResourceScopeSnapshot(ctx, tx, query.Environment, query.Site, query.Kind)
	if snapshotErr != nil && !errors.Is(snapshotErr, sql.ErrNoRows) {
		return resourceReadPlan{}, snapshotErr
	}
	meta, metaErr := currentGeneration(ctx, tx, query.Environment, query.Site)
	if metaErr != nil && !errors.Is(metaErr, sql.ErrNoRows) {
		return resourceReadPlan{}, metaErr
	}
	if query.LUID == "" && query.Name != "" && query.ProjectPath != "" && (query.Kind == "workbook" || query.Kind == "datasource" || query.Kind == "flow") {
		if err := validateCachedProjectPath(ctx, tx, query, meta); err != nil {
			return resourceReadPlan{}, err
		}
	}
	complete := snapshotErr == nil && snapshot.complete
	generationID := snapshot.id
	generatedAt := snapshot.generatedAt
	if snapshotErr != nil && metaErr == nil {
		scope := resourceScope(query.Kind)
		var requested, scopeComplete bool
		err := tx.QueryRowContext(ctx, `SELECT requested,complete FROM generation_scopes WHERE generation_key=? AND scope=?`, meta.key, scope).Scan(&requested, &scopeComplete)
		if err == nil {
			complete = requested && scopeComplete
			if complete {
				generationID, generatedAt = meta.id, meta.generatedAt
			}
		} else if !errors.Is(err, sql.ErrNoRows) {
			return resourceReadPlan{}, err
		}
	}
	where, args := resourceWhere(query)
	if query.ProjectName != "" {
		if !complete {
			return resourceReadPlan{}, projectFilterUnavailableError{}
		}
		filter, filterArgs, err := cachedProjectNameFilter(ctx, tx, &query, meta)
		if err != nil {
			return resourceReadPlan{}, err
		}
		where += " AND " + filter
		args = append(args, filterArgs...)
	}

	var total int
	var inventoryObserved sql.NullString
	if err := tx.QueryRowContext(ctx, `SELECT count(*),max(observed_at) FROM resource_entries `+where, args...).Scan(&total, &inventoryObserved); err != nil {
		return resourceReadPlan{}, err
	}
	if snapshotErr != nil && metaErr != nil && total == 0 {
		return resourceReadPlan{}, uninitializedError{}
	}
	if !complete && total == 0 {
		return resourceReadPlan{}, unavailableScopeError{resourceScope(query.Kind)}
	}
	result := ResourceResult{Total: total, Coverage: "partial"}
	if inventoryObserved.Valid {
		observed, err := time.Parse(generationTimeLayout, inventoryObserved.String)
		if err != nil {
			return resourceReadPlan{}, err
		}
		result.InventoryObservedAt = observed
	}
	if complete {
		result.Coverage = "complete"
		result.GenerationID = generationID
		result.GeneratedAt = generatedAt
		result.Stale, _ = staleness(s.store.now, generatedAt, generationID)
	}

	return resourceReadPlan{query: query, where: where, args: args, result: result}, nil
}

type ambiguousProjectPathError struct {
	path string
	ids  []string
}

func (e ambiguousProjectPathError) Error() string {
	return fmt.Sprintf("cache project path %q is ambiguous; matching project LUIDs: %s; select content by LUID", e.path, strings.Join(e.ids, ", "))
}
func (ambiguousProjectPathError) AmbiguousCacheSelector() bool { return true }

// Validate projects independently of the content name, including implicitly hydrated projects.
func validateCachedProjectPath(ctx context.Context, tx *sql.Tx, query ResourceQuery, meta generationMeta) error {
	snapshot, err := currentResourceScopeSnapshot(ctx, tx, query.Environment, query.Site, "project")
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return err
	}
	statement := `SELECT luid FROM resource_entries WHERE environment=? AND site=? AND kind='project' AND project_path=? ORDER BY luid`
	args := []any{query.Environment, query.Site, query.ProjectPath}
	if err != nil {
		var complete bool
		err = tx.QueryRowContext(ctx, `SELECT complete FROM generation_scopes WHERE generation_key=? AND scope='projects'`, meta.key).Scan(&complete)
		if err != nil && !errors.Is(err, sql.ErrNoRows) {
			return err
		}
		if !complete {
			return unavailableScopeError{"projects"}
		}
		statement = `SELECT luid FROM catalog_records WHERE generation_key=? AND kind='project' AND project_path=? ORDER BY luid`
		args = []any{meta.key, query.ProjectPath}
	} else if !snapshot.complete {
		return unavailableScopeError{"projects"}
	}
	rows, err := tx.QueryContext(ctx, statement, args...)
	if err != nil {
		return err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return err
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return err
	}
	if len(ids) > 1 {
		return ambiguousProjectPathError{query.ProjectPath, ids}
	}
	if len(ids) == 0 {
		return resourceNotFoundError{}
	}
	return nil
}

type resourceScopeSnapshot struct {
	id          string
	generatedAt time.Time
	complete    bool
	recordCount int
}

func currentResourceScopeSnapshot(ctx context.Context, q queryRower, environment, site, kind string) (resourceScopeSnapshot, error) {
	var snapshot resourceScopeSnapshot
	var generated string
	err := q.QueryRowContext(ctx, `SELECT generation_id,generated_at,complete,record_count FROM resource_scope_snapshots WHERE environment=? AND site=? AND kind=?`, environment, site, kind).Scan(&snapshot.id, &generated, &snapshot.complete, &snapshot.recordCount)
	if err != nil {
		return snapshot, err
	}
	snapshot.generatedAt, err = time.Parse(generationTimeLayout, generated)
	return snapshot, err
}

func resourceQueryFingerprint(query ResourceQuery) string {
	value := struct {
		Environment     string
		Site            string
		Kind            string
		LUID            string
		Name            string
		ProjectPath     string
		ProjectLUID     string
		ProjectName     string
		ProjectSnapshot string
		Limit           int
	}{query.Environment, query.Site, query.Kind, query.LUID, query.Name, query.ProjectPath, query.ProjectLUID, query.ProjectName, query.projectSnapshot, query.Limit}
	data, _ := json.Marshal(value)
	digest := sha256.Sum256(data)
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func resourceScope(kind string) string {
	if scope := kindScope(kind); scope != "" {
		return scope
	}
	return kind
}

func resourceWhere(query ResourceQuery) (string, []any) {
	parts := []string{"WHERE environment=?", "site=?", "kind=?"}
	args := []any{query.Environment, query.Site, query.Kind}
	for _, value := range []struct {
		column string
		value  string
	}{{"luid", query.LUID}, {"name", query.Name}, {"project_path", query.ProjectPath}, {"project_luid", query.ProjectLUID}} {
		if value.value != "" {
			parts = append(parts, value.column+"=?")
			args = append(args, value.value)
		}
	}
	return strings.Join(parts, " AND "), args
}
