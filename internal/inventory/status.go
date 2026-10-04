package inventory

import (
	"context"
	"time"

	corecache "github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/value"
)

// StatusSource reads neutral facts from one selected local cache generation.
type StatusSource struct{ Store *corecache.Store }

func (s StatusSource) Status(ctx context.Context, input value.CacheSelection) (value.CacheStatusObservation, error) {
	result, err := s.Store.Status(ctx, corecache.Selection{Environment: input.Environment, Site: input.Site, SiteSelected: true})
	if err != nil {
		return value.CacheStatusObservation{}, err
	}
	retained := make([]value.CachedObservation, len(result.Retained))
	for i, item := range result.Retained {
		retained[i] = value.CachedObservation{Kind: item.Kind, Records: item.Records, Complete: item.Complete, Stale: item.Stale, Oldest: item.Oldest.UTC().Format(time.RFC3339Nano), Newest: item.Newest.UTC().Format(time.RFC3339Nano)}
	}
	if result.GenerationID == "" {
		return value.CacheStatusObservation{Retained: retained, Environment: result.Environment, Site: result.Site, Path: result.Path}, nil
	}
	coverage := make([]value.CacheCoverage, len(result.Coverage))
	for i, scope := range result.Coverage {
		coverage[i] = value.CacheCoverage{Scope: scope.Scope, Requested: scope.Requested, Complete: scope.Complete, Records: scope.Records}
	}
	return value.CacheStatusObservation{Retained: retained, Coverage: coverage, ID: result.GenerationID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt.UTC().Format(time.RFC3339Nano), Age: result.Age.String(), Complete: result.Complete, Stale: result.Stale, Source: result.Source, Path: result.Path, Records: result.RecordCount, Warnings: append([]string(nil), result.Warnings...)}, nil
}
