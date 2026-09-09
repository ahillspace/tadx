package catalog

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

const maximumResourceScopeRows = 10_000_000

// ReplaceResourceScope atomically publishes one complete resource-kind
// inventory without changing any other kind or the current full generation.
// Failed or incomplete collection must not call this method; use
// UpsertResources for partial read-through records.
func (s *Store) ReplaceResourceScope(ctx context.Context, replacement ResourceScopeReplacement) (ReplaceResult, error) {
	normalized, err := normalizeResourceScopeReplacement(replacement)
	if err != nil {
		return ReplaceResult{}, err
	}
	generationID, err := resourceScopeGenerationID(normalized)
	if err != nil {
		return ReplaceResult{}, err
	}
	db, err := s.open(ctx)
	if err != nil {
		return ReplaceResult{}, err
	}
	defer db.Close()
	tx, err := db.BeginTx(ctx, nil)
	if err != nil {
		return ReplaceResult{}, err
	}
	defer tx.Rollback()
	if _, err := tx.ExecContext(ctx, `CREATE TEMP TABLE IF NOT EXISTS tadx_resource_scope_stage (
		luid TEXT PRIMARY KEY,name TEXT NOT NULL,project_path TEXT NOT NULL,project_luid TEXT NOT NULL,owner TEXT NOT NULL,
		payload BLOB NOT NULL,coverage TEXT NOT NULL CHECK(coverage IN ('summary','detail')),observed_at TEXT NOT NULL
	) STRICT`); err != nil {
		return ReplaceResult{}, fmt.Errorf("create catalog resource scope stage: %w", err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM tadx_resource_scope_stage`); err != nil {
		return ReplaceResult{}, fmt.Errorf("clear catalog resource scope stage: %w", err)
	}
	insert, err := tx.PrepareContext(ctx, `INSERT INTO tadx_resource_scope_stage(luid,name,project_path,project_luid,owner,payload,coverage,observed_at) VALUES(?,?,?,?,?,?,?,?)`)
	if err != nil {
		return ReplaceResult{}, err
	}
	for _, entry := range normalized.Entries {
		if _, err := insert.ExecContext(ctx, entry.LUID, entry.Name, entry.ProjectPath, entry.ProjectLUID, entry.Owner, entry.Payload, entry.Coverage, entry.ObservedAt.UTC().Format(generationTimeLayout)); err != nil {
			insert.Close()
			return ReplaceResult{}, fmt.Errorf("stage catalog %s %q: %w", normalized.Kind, entry.LUID, err)
		}
	}
	if err := insert.Close(); err != nil {
		return ReplaceResult{}, err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM resource_entries
		WHERE environment=? AND site=? AND kind=?
		AND NOT EXISTS (SELECT 1 FROM tadx_resource_scope_stage staged WHERE staged.luid=resource_entries.luid)`, normalized.Environment, normalized.Site, normalized.Kind); err != nil {
		return ReplaceResult{}, fmt.Errorf("delete stale catalog %s records: %w", normalized.Kind, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO resource_entries(environment,site,kind,luid,name,project_path,project_luid,owner,payload,coverage,observed_at)
		SELECT ?,?,?,luid,name,project_path,project_luid,owner,payload,coverage,observed_at FROM tadx_resource_scope_stage WHERE 1
		ON CONFLICT(environment,site,kind,luid) DO UPDATE SET
		name=excluded.name,project_path=excluded.project_path,project_luid=excluded.project_luid,owner=excluded.owner,
		payload=excluded.payload,coverage=excluded.coverage,observed_at=excluded.observed_at`, normalized.Environment, normalized.Site, normalized.Kind); err != nil {
		return ReplaceResult{}, fmt.Errorf("publish catalog %s records: %w", normalized.Kind, err)
	}
	if _, err := tx.ExecContext(ctx, `INSERT INTO resource_scope_snapshots(environment,site,kind,generation_id,generated_at,complete,source,record_count)
		VALUES(?,?,?,?,?,1,?,?)
		ON CONFLICT(environment,site,kind) DO UPDATE SET generation_id=excluded.generation_id,generated_at=excluded.generated_at,complete=1,source=excluded.source,record_count=excluded.record_count`,
		normalized.Environment, normalized.Site, normalized.Kind, generationID, normalized.GeneratedAt.UTC().Format(generationTimeLayout), normalized.Source, len(normalized.Entries)); err != nil {
		return ReplaceResult{}, fmt.Errorf("publish catalog %s snapshot: %w", normalized.Kind, err)
	}
	var count int
	if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM resource_entries WHERE environment=? AND site=? AND kind=?`, normalized.Environment, normalized.Site, normalized.Kind).Scan(&count); err != nil {
		return ReplaceResult{}, err
	}
	if count != len(normalized.Entries) {
		return ReplaceResult{}, fmt.Errorf("catalog %s snapshot contains %d records; expected %d", normalized.Kind, count, len(normalized.Entries))
	}
	if err := checkIntegrity(ctx, tx); err != nil {
		return ReplaceResult{}, err
	}
	if err := tx.Commit(); err != nil {
		return ReplaceResult{}, err
	}
	return ReplaceResult{GenerationID: generationID, Path: s.RelativePath(), RecordCount: len(normalized.Entries)}, nil
}

func normalizeResourceScopeReplacement(input ResourceScopeReplacement) (ResourceScopeReplacement, error) {
	input.Environment = strings.TrimSpace(input.Environment)
	input.Site = strings.TrimSpace(input.Site)
	input.Kind = strings.TrimSpace(input.Kind)
	input.Source = strings.TrimSpace(input.Source)
	if input.Environment == "" || input.Kind == "" || input.Source == "" || input.GeneratedAt.IsZero() {
		return ResourceScopeReplacement{}, errors.New("catalog resource scope replacement requires environment, kind, source, and generation time")
	}
	if kindScope(input.Kind) == "" {
		return ResourceScopeReplacement{}, fmt.Errorf("catalog resource kind %q does not support complete inventory replacement", input.Kind)
	}
	if len(input.Entries) > maximumResourceScopeRows {
		return ResourceScopeReplacement{}, fmt.Errorf("catalog resource scope exceeds %d-row limit", maximumResourceScopeRows)
	}
	for name, value := range map[string]string{"environment": input.Environment, "site": input.Site, "kind": input.Kind, "source": input.Source} {
		if err := validateField(name, value); err != nil {
			return ResourceScopeReplacement{}, err
		}
	}
	seen := make(map[string]struct{}, len(input.Entries))
	entries := make([]ResourceEntry, len(input.Entries))
	for index, entry := range input.Entries {
		entry.LUID = strings.TrimSpace(entry.LUID)
		entry.Name = strings.TrimSpace(entry.Name)
		if entry.LUID == "" || entry.Name == "" {
			return ResourceScopeReplacement{}, fmt.Errorf("catalog resource entry %d requires authoritative LUID and name", index)
		}
		if _, duplicate := seen[entry.LUID]; duplicate {
			return ResourceScopeReplacement{}, fmt.Errorf("catalog resource scope returned duplicate LUID %q", entry.LUID)
		}
		seen[entry.LUID] = struct{}{}
		for label, value := range map[string]string{"LUID": entry.LUID, "name": entry.Name, "project path": entry.ProjectPath, "owner": entry.Owner} {
			if err := validateField(label, value); err != nil {
				return ResourceScopeReplacement{}, fmt.Errorf("catalog resource entry %d: %w", index, err)
			}
		}
		for label, value := range map[string]string{"environment": entry.Environment, "site": entry.Site, "kind": entry.Kind} {
			value = strings.TrimSpace(value)
			want := map[string]string{"environment": input.Environment, "site": input.Site, "kind": input.Kind}[label]
			if value != "" && value != want {
				return ResourceScopeReplacement{}, fmt.Errorf("catalog resource entry %d %s %q does not match replacement %q", index, label, value, want)
			}
		}
		entry.Environment, entry.Site, entry.Kind = input.Environment, input.Site, input.Kind
		if entry.Coverage == "" {
			entry.Coverage = "summary"
		}
		if entry.Coverage != "summary" && entry.Coverage != "detail" {
			return ResourceScopeReplacement{}, fmt.Errorf("catalog resource entry %d has unsupported coverage %q", index, entry.Coverage)
		}
		if entry.Payload == nil {
			entry.Payload = []byte{}
		}
		if entry.ObservedAt.IsZero() {
			entry.ObservedAt = input.GeneratedAt
		}
		entries[index] = entry
	}
	sort.Slice(entries, func(left, right int) bool { return entries[left].LUID < entries[right].LUID })
	input.Entries = entries
	return input, nil
}

func resourceScopeGenerationID(input ResourceScopeReplacement) (string, error) {
	canonical := struct {
		Environment string
		Site        string
		Kind        string
		Source      string
		Entries     []ResourceEntry
	}{input.Environment, input.Site, input.Kind, input.Source, input.Entries}
	encoded, err := json.Marshal(canonical)
	if err != nil {
		return "", fmt.Errorf("encode catalog resource scope fingerprint: %w", err)
	}
	digest := sha256.Sum256(encoded)
	return "sha256:" + hex.EncodeToString(digest[:]), nil
}
