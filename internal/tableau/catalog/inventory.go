package catalog

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"sync"
)

// InventorySnapshot is one complete, deterministic Tableau resource inventory.
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
}

// InventoryTable contains one complete implicit dependency scope.
type InventoryTable struct {
	Scope   Scope
	Columns []Column
	Rows    [][]any
}

// CollectInventory runs the hardened catalog traversal for one resource scope
// and returns rows only after every requested and implicit dependency page has
// succeeded. Concurrent page arrival is normalized into authoritative LUID
// order before the snapshot becomes visible to callers.
func (e *Engine) CollectInventory(ctx context.Context, scope Scope) (InventorySnapshot, error) {
	if !isInventoryScope(scope) {
		return InventorySnapshot{}, fmt.Errorf("catalog scope %q is not a resource inventory", scope)
	}
	writer := &inventoryWriter{rows: make(map[Scope][][]any)}
	result, err := e.Run(ctx, RunRequest{RequestedScopes: []Scope{scope}}, writer)
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
