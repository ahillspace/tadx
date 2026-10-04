package inventory

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"time"

	corecache "github.com/ahillspace/tadx/internal/cache"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
	"github.com/ahillspace/tadx/internal/value"
)

const generationSource = "tableau-rest"

// GenerationRunner collects fixed scopes into a generation writer.
type GenerationRunner interface {
	Run(context.Context, tableaucache.RunRequest, tableaucache.BatchWriter) (tableaucache.Result, error)
}

// GenerationHydrator owns complete-generation collection and publication.
// The caller supplies a target-bound store and authenticated executor factory.
type GenerationHydrator struct {
	Store       *corecache.Store
	Now         func() time.Time
	ExecutorFor func(context.Context, string, string) (tableaucache.Executor, error)
	NewRunner   func(tableaucache.Executor) (GenerationRunner, error)
	CheckScope  func(string) error
}

// Hydrate publishes a generation only after the collector completes its scope plan.
func (h GenerationHydrator) Hydrate(ctx context.Context, input value.CacheGenerationRequest) (value.CacheGenerationReceipt, error) {
	requested := generationScopes(input.RequestedScopes)
	plan, err := tableaucache.PlanScopes(requested)
	if err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	if err := CheckScopeCapabilities(h.CheckScope, plan.Collected); err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	if !slices.Equal(generationScopeStrings(plan.Requested), input.RequestedScopes) || !slices.Equal(generationScopeStrings(plan.Implicit), input.ImplicitScopes) {
		return value.CacheGenerationReceipt{}, errors.New("cache action and collector scope plans differ")
	}
	executor, err := h.ExecutorFor(ctx, input.Environment, input.Site)
	if err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	runner, err := h.NewRunner(executor)
	if err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	now := h.Now
	if now == nil {
		now = time.Now
	}
	generatedAt := now().UTC()
	writer, err := h.Store.BeginRefreshGeneration(ctx, corecache.GenerationMetadata{
		Environment: input.Environment, Site: input.Site, GeneratedAt: generatedAt, Source: generationSource,
		RequestedScopes: slices.Clone(input.RequestedScopes), ImplicitScopes: slices.Clone(input.ImplicitScopes),
	})
	if err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	defer writer.Rollback()
	started := time.Now()
	result, err := runner.Run(ctx, tableaucache.RunRequest{RequestedScopes: requested}, generationBatchWriter{writer: writer})
	if err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	if !slices.Equal(generationScopeStrings(result.RequestedScopes), input.RequestedScopes) || !slices.Equal(generationScopeStrings(result.ImplicitScopes), input.ImplicitScopes) {
		return value.CacheGenerationReceipt{}, errors.New("cache collector returned a different scope plan")
	}
	collected := append(slices.Clone(result.RequestedScopes), result.ImplicitScopes...)
	var warnings []string
	if result.DeniedPermissions > 0 {
		collected = slices.DeleteFunc(collected, func(scope tableaucache.Scope) bool { return scope == tableaucache.ScopePermissions })
		if err := writer.MarkPermissionsIncomplete(ctx); err != nil {
			return value.CacheGenerationReceipt{}, err
		}
		warnings = append(warnings, fmt.Sprintf("Cache permission coverage is incomplete: %d workbook permission reads were denied (HTTP 403). Missing rules are unknown, not empty permissions; readable inventory was retained.", result.DeniedPermissions))
	}
	if err := writer.CompleteScopes(ctx, generationScopeStrings(collected)); err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	counts, total, err := generationScopeCounts(plan.Collected, result.Counts)
	if err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	if result.Requests > math.MaxInt {
		return value.CacheGenerationReceipt{}, errors.New("cache request count exceeds the receipt bound")
	}
	published, err := writer.Publish(ctx)
	if err != nil {
		return value.CacheGenerationReceipt{}, err
	}
	return value.CacheGenerationReceipt{
		GenerationID: published.GenerationID, GeneratedAt: generatedAt, Complete: result.DeniedPermissions == 0, Source: generationSource,
		Path: published.Path, RecordCount: published.RecordCount, HydratedRecordCount: total,
		RequestedScopes: slices.Clone(input.RequestedScopes), ImplicitScopes: slices.Clone(input.ImplicitScopes),
		ScopeCounts: counts, DeniedPermissions: result.DeniedPermissions, Warnings: warnings,
		Diagnostics: value.CacheGenerationDiagnostics{Requests: int(result.Requests), FailedRequests: result.DeniedPermissions, Duration: time.Since(started).Round(time.Millisecond).String()},
	}, nil
}

func generationScopes(values []string) []tableaucache.Scope {
	result := make([]tableaucache.Scope, len(values))
	for i, value := range values {
		result[i] = tableaucache.Scope(value)
	}
	return result
}

func generationScopeStrings(values []tableaucache.Scope) []string {
	result := make([]string, len(values))
	for i, value := range values {
		result[i] = string(value)
	}
	return result
}

func generationScopeCounts(scopes []tableaucache.Scope, counts map[tableaucache.Scope]int64) ([]value.CacheScopeCount, int, error) {
	result := make([]value.CacheScopeCount, 0, len(scopes))
	total := 0
	for _, scope := range scopes {
		count := counts[scope]
		if count < 0 || count > math.MaxInt-int64(total) {
			return nil, 0, errors.New("cache record count exceeds the receipt bound")
		}
		total += int(count)
		result = append(result, value.CacheScopeCount{Scope: string(scope), Records: int(count)})
	}
	return result, total, nil
}

type generationBatchWriter struct{ writer *corecache.GenerationWriter }

func (w generationBatchWriter) WriteBatch(ctx context.Context, batch tableaucache.Batch) error {
	columns := make([]string, len(batch.Columns))
	for i, column := range batch.Columns {
		columns[i] = column.Name
	}
	return w.writer.WriteBatch(ctx, corecache.Batch{Scope: string(batch.Scope), Columns: columns, Rows: batch.Rows})
}
