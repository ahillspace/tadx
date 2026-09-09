package catalog

import (
	"context"
	"fmt"
	"os"
	"strings"
)

// Each collection owns a private, disposable disk database. Its page cap bounds
// database pages; SQLite journal and temporary files are additional overhead.
const maximumStagePages = 262144 // 1 GiB with the database's 4096-byte pages.

func (s *Store) beginStagedRefresh(ctx context.Context, metadata GenerationMetadata) (*GenerationWriter, error) {
	directory, err := os.MkdirTemp("", "tadx-catalog-stage-")
	if err != nil {
		return nil, fmt.Errorf("create catalog staging directory: %w", err)
	}
	stage := NewStore(directory, s.now)
	writer, err := stage.beginGeneration(ctx, metadata, false)
	if err != nil {
		os.RemoveAll(directory)
		return nil, err
	}
	writer.publicationTarget = s
	writer.stagingDirectory = directory
	if _, err := writer.tx.ExecContext(ctx, fmt.Sprintf("PRAGMA max_page_count=%d", maximumStagePages)); err != nil {
		writer.Rollback()
		return nil, fmt.Errorf("bound catalog staging database: %w", err)
	}
	return writer, nil
}

func (s *Store) publishStaged(ctx context.Context, staged *GenerationWriter, count int, fingerprint string) (ReplaceResult, error) {
	writer, err := s.beginGeneration(ctx, staged.metadata, true)
	if err != nil {
		return ReplaceResult{}, err
	}
	defer writer.Rollback()
	// ATTACH is connection-local; every copy uses the same transaction connection.
	if _, err := writer.tx.ExecContext(ctx, "ATTACH DATABASE ? AS refresh_stage", staged.store.databasePath()); err != nil {
		return ReplaceResult{}, fmt.Errorf("attach catalog staging database: %w", err)
	}
	for scope, columns := range batchColumns {
		names := strings.Join(columns, ",")
		if _, err := writer.tx.ExecContext(ctx, fmt.Sprintf("INSERT INTO main.%s(generation_key,%s) SELECT ?,%s FROM refresh_stage.%s WHERE generation_key=?", scope, names, names, scope), writer.key, staged.key); err != nil {
			return ReplaceResult{}, fmt.Errorf("publish staged %s: %w", scope, err)
		}
	}
	if _, err := writer.tx.ExecContext(ctx, `DELETE FROM main.generation_scopes WHERE generation_key=?`, writer.key); err != nil {
		return ReplaceResult{}, err
	}
	if _, err := writer.tx.ExecContext(ctx, `INSERT INTO main.generation_scopes(generation_key,scope,requested,complete) SELECT ?,scope,requested,complete FROM refresh_stage.generation_scopes WHERE generation_key=?`, writer.key, staged.key); err != nil {
		return ReplaceResult{}, err
	}
	if _, err := writer.tx.ExecContext(ctx, `INSERT INTO main.catalog_records(generation_key,luid,kind,name,project_path,owner,requested) SELECT ?,luid,kind,name,project_path,owner,requested FROM refresh_stage.catalog_records WHERE generation_key=?`, writer.key, staged.key); err != nil {
		return ReplaceResult{}, err
	}
	writer.partialPermissions = staged.partialPermissions
	return writer.publishReady(ctx, count, fingerprint)
}
