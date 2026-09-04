package catalog

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

// UpsertResources records successful live reads without changing generation coverage.
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
	statement, err := tx.PrepareContext(ctx, `INSERT INTO resource_entries(environment,site,kind,luid,name,project_path,owner,payload,coverage,observed_at)
		VALUES(?,?,?,?,?,?,?,?,?,?)
		ON CONFLICT(environment,site,kind,luid) DO UPDATE SET
		name=excluded.name,project_path=excluded.project_path,owner=excluded.owner,
		payload=CASE WHEN resource_entries.coverage='detail' AND excluded.coverage='summary' THEN resource_entries.payload ELSE excluded.payload END,
		coverage=CASE WHEN resource_entries.coverage='detail' THEN 'detail' ELSE excluded.coverage END,
		observed_at=CASE WHEN resource_entries.coverage='detail' AND excluded.coverage='summary' THEN resource_entries.observed_at ELSE excluded.observed_at END`)
	if err != nil {
		return err
	}
	defer statement.Close()
	for index, entry := range entries {
		entry.Environment = strings.TrimSpace(entry.Environment)
		entry.Site = strings.TrimSpace(entry.Site)
		entry.Kind = strings.TrimSpace(entry.Kind)
		entry.LUID = strings.TrimSpace(entry.LUID)
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.Environment == "" || entry.Kind == "" || entry.LUID == "" || entry.Name == "" || entry.ObservedAt.IsZero() {
			return fmt.Errorf("catalog resource entry %d requires environment, kind, LUID, name, and observation time", index)
		}
		if entry.Coverage != "summary" && entry.Coverage != "detail" {
			return fmt.Errorf("catalog resource entry %d has unsupported coverage %q", index, entry.Coverage)
		}
		if entry.Payload == nil {
			entry.Payload = []byte{}
		}
		if _, err := statement.ExecContext(ctx, entry.Environment, entry.Site, entry.Kind, entry.LUID, entry.Name, entry.ProjectPath, entry.Owner, entry.Payload, entry.Coverage, entry.ObservedAt.UTC().Format(generationTimeLayout)); err != nil {
			return fmt.Errorf("upsert catalog resource entry %d: %w", index, err)
		}
	}
	return tx.Commit()
}

// ReadResources returns one bounded local resource page without contacting Tableau.
func (s *Store) ReadResources(ctx context.Context, query ResourceQuery) (ResourceResult, error) {
	query.Environment = strings.TrimSpace(query.Environment)
	query.Site = strings.TrimSpace(query.Site)
	query.Kind = strings.TrimSpace(query.Kind)
	if query.Environment == "" || query.Kind == "" {
		return ResourceResult{}, errors.New("catalog resource read requires environment and kind")
	}
	if query.Offset < 0 || query.Limit < 1 || query.Limit > maxLimit {
		return ResourceResult{}, fmt.Errorf("catalog resource limit must be between 1 and %d and offset must be nonnegative", maxLimit)
	}
	db, err := s.open(ctx)
	if err != nil {
		return ResourceResult{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return ResourceResult{}, err
	}
	defer tx.Rollback()

	meta, metaErr := currentGeneration(ctx, tx, query.Environment, query.Site)
	if metaErr != nil && !errors.Is(metaErr, sql.ErrNoRows) {
		return ResourceResult{}, metaErr
	}
	complete := false
	if metaErr == nil {
		scope := resourceScope(query.Kind)
		var requested, scopeComplete bool
		err := tx.QueryRowContext(ctx, `SELECT requested,complete FROM generation_scopes WHERE generation_key=? AND scope=?`, meta.key, scope).Scan(&requested, &scopeComplete)
		if err == nil {
			complete = requested && scopeComplete
		} else if !errors.Is(err, sql.ErrNoRows) {
			return ResourceResult{}, err
		}
	}

	where, args := resourceWhere(query)
	var total int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM resource_entries `+where, args...).Scan(&total); err != nil {
		return ResourceResult{}, err
	}
	if metaErr != nil && total == 0 {
		return ResourceResult{}, uninitializedError{}
	}
	if !complete && total == 0 {
		return ResourceResult{}, unavailableScopeError{resourceScope(query.Kind)}
	}
	rows, err := tx.QueryContext(ctx, `SELECT environment,site,kind,luid,name,project_path,owner,payload,coverage,observed_at FROM resource_entries `+where+` ORDER BY name,project_path,luid LIMIT ? OFFSET ?`, append(args, query.Limit, query.Offset)...)
	if err != nil {
		return ResourceResult{}, err
	}
	defer rows.Close()
	result := ResourceResult{Total: total, Coverage: "partial"}
	if complete {
		result.Coverage = "complete"
		result.GenerationID = meta.id
		result.GeneratedAt = meta.generatedAt
		result.Stale, _ = staleness(s.now, meta.generatedAt, meta.id)
	}
	for rows.Next() {
		var entry ResourceEntry
		var observed string
		if err := rows.Scan(&entry.Environment, &entry.Site, &entry.Kind, &entry.LUID, &entry.Name, &entry.ProjectPath, &entry.Owner, &entry.Payload, &entry.Coverage, &observed); err != nil {
			return ResourceResult{}, err
		}
		entry.ObservedAt, err = time.Parse(generationTimeLayout, observed)
		if err != nil {
			return ResourceResult{}, err
		}
		if entry.ObservedAt.After(result.NewestObserved) {
			result.NewestObserved = entry.ObservedAt
		}
		result.Entries = append(result.Entries, entry)
	}
	if err := rows.Err(); err != nil {
		return ResourceResult{}, err
	}
	if query.LUID != "" || query.Name != "" {
		if len(result.Entries) == 0 {
			return ResourceResult{}, resourceNotFoundError{}
		}
		if len(result.Entries) > 1 {
			return ResourceResult{}, ambiguousSelectorError{}
		}
	}
	if !complete && !result.NewestObserved.IsZero() {
		result.Stale, _ = staleness(s.now, result.NewestObserved, "read-through")
	}
	if err := tx.Commit(); err != nil {
		return ResourceResult{}, err
	}
	return result, nil
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
	}{{"luid", query.LUID}, {"name", query.Name}, {"project_path", query.ProjectPath}} {
		if value.value != "" {
			parts = append(parts, value.column+"=?")
			args = append(args, value.value)
		}
	}
	return strings.Join(parts, " AND "), args
}
