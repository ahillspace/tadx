// Package refresh orchestrates complete catalog generation replacement.
package refresh

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	maxWarnings     = 20
	maxWarningRunes = 512
)

// Inventory reads one complete normalized remote inventory.
type Inventory interface {
	Read(context.Context, Input) (Snapshot, error)
}

// Writer atomically replaces one complete local generation.
type Writer interface {
	Replace(context.Context, Generation) (WriteResult, error)
}

// Action orchestrates catalog.refresh.
type Action struct {
	inventory Inventory
	writer    Writer
}

// New creates catalog.refresh.
func New(inventory Inventory, writer Writer) *Action {
	return &Action{inventory: inventory, writer: writer}
}

// Execute reads all admitted scopes before publishing one complete generation.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.inventory == nil || a.writer == nil {
		return Output{}, failure("catalog.refresh.unconfigured", errs.KindRuntime, input, "Catalog refresh is not configured.", nil)
	}
	if strings.TrimSpace(input.Environment) == "" || !input.SiteResolved {
		return Output{}, failure("catalog.refresh.usage", errs.KindUsage, input, "Catalog refresh requires a resolved environment and site.", nil)
	}
	scopes, err := normalizeScopes(input.Scopes)
	if err != nil {
		return Output{}, failure("catalog.refresh.usage", errs.KindUsage, input, err.Error(), err)
	}
	request := input
	request.Scopes = scopes
	snapshot, err := a.inventory.Read(ctx, request)
	if err != nil {
		return Output{}, classify(err, input, "read")
	}
	if snapshot.GeneratedAt.IsZero() || strings.TrimSpace(snapshot.Source) == "" || snapshot.Records == nil {
		return Output{}, failure("catalog.refresh.incomplete", errs.KindOperation, input, "Catalog inventory is incomplete and was not published.", nil)
	}
	records := append([]Record(nil), snapshot.Records...)
	result, err := a.writer.Replace(ctx, Generation{Environment: input.Environment, Site: input.Site, GeneratedAt: snapshot.GeneratedAt, Complete: true, Source: snapshot.Source, Scopes: scopes, Records: records})
	if err != nil {
		return Output{}, classify(err, input, "publish")
	}
	generation := GenerationOutput{ID: result.GenerationID, Environment: input.Environment, Site: input.Site, GeneratedAt: snapshot.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z07:00"), Records: result.RecordCount, Source: snapshot.Source, Scopes: scopes}
	return Output{Status: "refreshed", Generation: generation, Path: result.Path, Warnings: boundWarnings(snapshot.Warnings), Help: []string{"tadx catalog status --environment " + input.Environment}}, nil
}

func normalizeScopes(values []string) ([]string, error) {
	supported := []string{"projects", "workbooks", "datasources", "flows"}
	if len(values) == 0 {
		return supported, nil
	}
	selected := make(map[string]bool, len(values))
	for _, value := range values {
		valid := false
		for _, candidate := range supported {
			if value == candidate {
				valid = true
				break
			}
		}
		if !valid {
			return nil, errors.New("Catalog refresh scope must be exactly one of projects, workbooks, datasources, or flows.")
		}
		if selected[value] {
			return nil, errors.New("Catalog refresh scopes must not contain duplicates.")
		}
		selected[value] = true
	}
	result := make([]string, 0, len(selected))
	for _, value := range supported {
		if selected[value] {
			result = append(result, value)
		}
	}
	return result, nil
}

func classify(err error, input Input, phase string) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return failure("catalog.refresh.cancelled", errs.KindOperation, input, "Catalog refresh was canceled before publication.", err)
	}
	return failure("catalog.refresh.failed", errs.KindOperation, input, "Catalog refresh failed during "+phase+".", err)
}

func failure(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "catalog.refresh", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Resolve the reported catalog refresh problem, then retry."}
}

func boundWarnings(values []string) []string {
	if len(values) > maxWarnings {
		values = values[:maxWarnings]
	}
	bounded := make([]string, len(values))
	for index, value := range values {
		runes := []rune(value)
		if len(runes) > maxWarningRunes {
			value = string(runes[:maxWarningRunes-3]) + "..."
		}
		bounded[index] = value
	}
	return bounded
}
