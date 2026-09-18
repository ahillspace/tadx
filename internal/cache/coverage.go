package cache

import (
	"context"
	"database/sql"
	"fmt"
	"time"
)

// ScopeCoverage is evidence for a scope in the selected generation.
type ScopeCoverage struct {
	Scope               string
	Requested, Complete bool
	Records             int
}

// RetainedObservation describes independently retained rows, not a claim that
// they were refreshed with the current inventory generation.
type RetainedObservation struct {
	Kind           string
	Records        int
	Complete       bool
	Oldest, Newest time.Time
	Stale          bool
}

func retainedCoverage(ctx context.Context, tx *sql.Tx, environment, site string, now time.Time) ([]RetainedObservation, error) {
	rows, err := tx.QueryContext(ctx, `WITH kinds AS (
		SELECT kind FROM resource_entries WHERE environment=? AND site=?
		UNION SELECT kind FROM resource_scope_snapshots WHERE environment=? AND site=?)
		SELECT k.kind,count(e.luid),COALESCE(s.complete,0),COALESCE(min(e.observed_at),s.generated_at),COALESCE(max(e.observed_at),s.generated_at)
		FROM kinds k LEFT JOIN resource_entries e ON e.kind=k.kind AND e.environment=? AND e.site=?
		LEFT JOIN resource_scope_snapshots s ON s.kind=k.kind AND s.environment=? AND s.site=?
		GROUP BY k.kind ORDER BY k.kind LIMIT 33`, environment, site, environment, site, environment, site, environment, site)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	result := make([]RetainedObservation, 0)
	for rows.Next() {
		var item RetainedObservation
		var oldest, newest string
		if err := rows.Scan(&item.Kind, &item.Records, &item.Complete, &oldest, &newest); err != nil {
			return nil, err
		}
		item.Oldest, err = time.Parse(time.RFC3339Nano, oldest)
		if err != nil {
			return nil, fmt.Errorf("invalid retained observation timestamp: %w", err)
		}
		item.Newest, err = time.Parse(time.RFC3339Nano, newest)
		if err != nil {
			return nil, fmt.Errorf("invalid retained observation timestamp: %w", err)
		}
		item.Stale = now.Sub(item.Oldest) > staleAfter
		result = append(result, item)
	}
	if len(result) > 32 {
		return nil, fmt.Errorf("retained cache scope count exceeds its supported bound")
	}
	return result, rows.Err()
}

func generationCoverage(ctx context.Context, tx *sql.Tx, key int64) ([]ScopeCoverage, error) {
	rows, err := tx.QueryContext(ctx, `SELECT scope,requested,complete FROM generation_scopes WHERE generation_key=? ORDER BY scope`, key)
	if err != nil {
		return nil, err
	}
	coverage := make([]ScopeCoverage, 0, len(publicScopes))
	for rows.Next() {
		var scope ScopeCoverage
		if err := rows.Scan(&scope.Scope, &scope.Requested, &scope.Complete); err != nil {
			rows.Close()
			return nil, err
		}
		if _, ok := batchColumns[scope.Scope]; !ok {
			rows.Close()
			return nil, fmt.Errorf("unsupported saved cache scope %q", scope.Scope)
		}
		coverage = append(coverage, scope)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return nil, err
	}
	for i := range coverage {
		if err := tx.QueryRowContext(ctx, `SELECT count(*) FROM `+coverage[i].Scope+` WHERE generation_key=?`, key).Scan(&coverage[i].Records); err != nil {
			return nil, err
		}
	}
	return coverage, nil
}
