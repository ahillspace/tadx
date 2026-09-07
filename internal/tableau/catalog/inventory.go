package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// InventorySnapshot is one deterministic Tableau resource inventory.
// Columns and Rows use the package's fixed positional schema so callers can
// adapt one traversal to resource-specific action types without another REST
// pagination implementation.
type InventorySnapshot struct {
	Scope             Scope
	Columns           []Column
	Rows              [][]any
	Dependencies      []InventoryTable
	Total             int
	Requests          int64
	TableauRequestIDs []string
	FinalConcurrency  int
	SkippedRows       int
}

// InventoryTable contains the valid rows from one implicit dependency scope.
type InventoryTable struct {
	Scope   Scope
	Columns []Column
	Rows    [][]any
}

// InventoryOptions selects explicit tolerance for malformed inventory records.
type InventoryOptions struct {
	// SkipMalformedRecords excludes invalid records but retains strict XML,
	// pagination, transport, and duplicate-identity validation.
	SkipMalformedRecords bool
}

// CollectInventory traverses all requested and dependency pages, then returns
// valid rows in authoritative LUID order. By default malformed records fail the
// collection; tolerant callers must account for SkippedRows before publishing.
func (e *Engine) CollectInventory(ctx context.Context, scope Scope, options ...InventoryOptions) (InventorySnapshot, error) {
	if !isInventoryScope(scope) {
		return InventorySnapshot{}, fmt.Errorf("catalog scope %q is not a resource inventory", scope)
	}
	writer := &inventoryWriter{rows: make(map[Scope][][]any)}
	tolerateMalformed := len(options) > 0 && options[0].SkipMalformedRecords
	result, err := e.run(ctx, RunRequest{RequestedScopes: []Scope{scope}}, writer, tolerateMalformed)
	if err != nil {
		return InventorySnapshot{}, err
	}
	columns, ok := ColumnsForScope(scope)
	if !ok {
		return InventorySnapshot{}, fmt.Errorf("catalog scope %q has no fixed schema", scope)
	}
	rows := writer.snapshot(scope)
	sortInventoryRows(rows)
	if int64(len(rows)) != result.Counts[scope] {
		return InventorySnapshot{}, errors.New("catalog inventory row count changed while collecting")
	}
	dependencies := make([]InventoryTable, 0, len(result.ImplicitScopes))
	for _, dependencyScope := range result.ImplicitScopes {
		dependencyColumns, ok := ColumnsForScope(dependencyScope)
		if !ok {
			return InventorySnapshot{}, fmt.Errorf("catalog dependency scope %q has no fixed schema", dependencyScope)
		}
		dependencyRows := writer.snapshot(dependencyScope)
		sortInventoryRows(dependencyRows)
		if int64(len(dependencyRows)) != result.Counts[dependencyScope] {
			return InventorySnapshot{}, errors.New("catalog dependency row count changed while collecting")
		}
		dependencies = append(dependencies, InventoryTable{Scope: dependencyScope, Columns: dependencyColumns, Rows: dependencyRows})
	}
	return InventorySnapshot{
		Scope:             scope,
		Columns:           columns,
		Rows:              rows,
		Dependencies:      dependencies,
		Total:             len(rows),
		Requests:          result.Requests,
		TableauRequestIDs: append([]string(nil), result.TableauRequestIDs...),
		FinalConcurrency:  result.FinalConcurrency,
		SkippedRows:       result.SkippedRows[scope],
	}, nil
}

func isInventoryScope(scope Scope) bool {
	switch scope {
	case ScopeUsers, ScopeGroups, ScopeProjects, ScopeWorkbooks, ScopeDatasources, ScopeFlows, ScopeViews:
		return true
	default:
		return false
	}
}

type inventoryWriter struct {
	mu   sync.Mutex
	rows map[Scope][][]any
}

func (w *inventoryWriter) WriteBatch(_ context.Context, batch Batch) error {
	if !isInventoryScope(batch.Scope) {
		return nil
	}
	if len(batch.Columns) == 0 || batch.Columns[0].Name != "id" {
		return fmt.Errorf("catalog scope %q omitted its authoritative identity column", batch.Scope)
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	for _, row := range batch.Rows {
		if len(row) != len(batch.Columns) {
			return fmt.Errorf("catalog scope %q returned a row with %d values for %d columns", batch.Scope, len(row), len(batch.Columns))
		}
		identity, ok := row[0].(string)
		if !ok || strings.TrimSpace(identity) == "" {
			return fmt.Errorf("catalog scope %q returned an invalid authoritative identity", batch.Scope)
		}
		w.rows[batch.Scope] = append(w.rows[batch.Scope], append([]any(nil), row...))
	}
	return nil
}

func (w *inventoryWriter) snapshot(scope Scope) [][]any {
	w.mu.Lock()
	defer w.mu.Unlock()
	rows := make([][]any, len(w.rows[scope]))
	for index, row := range w.rows[scope] {
		rows[index] = append([]any(nil), row...)
	}
	return rows
}

func sortInventoryRows(rows [][]any) {
	sort.Slice(rows, func(left, right int) bool {
		return rows[left][0].(string) < rows[right][0].(string)
	})
}
