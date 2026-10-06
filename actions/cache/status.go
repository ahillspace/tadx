// cache.status reports bounded local cache generation status.
package cache

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/value"
)

// StatusInput selects one resolved local cache generation.
type StatusInput struct {
	Environment string
	Site        string
}

// StatusResult is the source-facing generation status.
type StatusResult struct {
	Retained    []value.CachedObservation
	Coverage    []value.CacheCoverage
	ID          string
	Environment string
	Site        string
	GeneratedAt string
	Age         string
	Complete    bool
	Stale       bool
	Source      string
	Path        string
	Records     int
	Warnings    []string
}

// StatusGeneration is the bounded status projection.
type StatusGeneration struct {
	Coverage    []value.CacheCoverage `json:"coverage,omitempty"`
	ID          string                `json:"id"`
	Environment string                `json:"environment"`
	Site        string                `json:"site"`
	GeneratedAt string                `json:"generated_at"`
	Records     int                   `json:"records"`
	Complete    bool                  `json:"complete"`
	Stale       bool                  `json:"stale"`
	Age         string                `json:"age,omitempty"`
	Source      string                `json:"source,omitempty"`
}

// StatusOutput is the stable cache.status document.
type StatusOutput struct {
	Retained   []value.CachedObservation
	Status     string
	Generation StatusGeneration
	Path       string
	Warnings   []string
	Help       []string
}

// StatusCompactGeneration contains status decision fields.
type StatusCompactGeneration struct {
	Coverage    []value.CacheCoverage `json:"coverage,omitempty"`
	ID          string                `json:"id"`
	Environment string                `json:"environment"`
	Site        string                `json:"site"`
	GeneratedAt string                `json:"generated_at"`
	Records     int                   `json:"records"`
	Complete    bool                  `json:"complete"`
	Stale       bool                  `json:"stale"`
}

// StatusCompactResult is the default bounded status projection.
type StatusCompactResult struct {
	Retained   []value.CachedObservation `json:"retained_observations,omitempty"`
	Status     string                    `json:"status"`
	Generation StatusCompactGeneration   `json:"generation"`
	Warnings   []string                  `json:"warnings,omitempty"`
	Details    string                    `json:"details"`
	Help       []string                  `json:"help"`
}

// StatusFullResult is the expanded bounded status projection.
type StatusFullResult struct {
	Retained   []value.CachedObservation `json:"retained_observations,omitempty"`
	Status     string                    `json:"status"`
	Generation StatusGeneration          `json:"generation"`
	Path       string                    `json:"path,omitempty"`
	Warnings   []string                  `json:"warnings,omitempty"`
	Help       []string                  `json:"help"`
}

// StatusUninitializedResult describes a selected cache without inventing a generation.
type StatusUninitializedResult struct {
	Retained    []value.CachedObservation `json:"retained_observations,omitempty"`
	Status      string                    `json:"status"`
	Environment string                    `json:"environment"`
	Site        string                    `json:"site"`
	Help        []string                  `json:"help"`
}

// CompactOutput returns freshness and completeness fields.
func (o StatusOutput) CompactOutput() any {
	g := o.Generation
	if o.Status == "uninitialized" {
		return StatusUninitializedResult{Retained: o.compactRetained(), Status: o.Status, Environment: g.Environment, Site: g.Site, Help: o.Help}
	}
	return StatusCompactResult{Retained: o.compactRetained(), Status: o.Status, Generation: StatusCompactGeneration{Coverage: g.Coverage, ID: g.ID, Environment: g.Environment, Site: g.Site, GeneratedAt: g.GeneratedAt, Records: g.Records, Complete: g.Complete, Stale: g.Stale}, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded generation source details.
func (o StatusOutput) FullOutput() any {
	if o.Status == "uninitialized" {
		return StatusUninitializedResult{Retained: o.Retained, Status: o.Status, Environment: o.Generation.Environment, Site: o.Generation.Site, Help: o.Help}
	}
	return StatusFullResult{Retained: o.Retained, Status: o.Status, Generation: o.Generation, Path: o.Path, Warnings: o.Warnings, Help: o.Help}
}

func (o StatusOutput) compactRetained() []value.CachedObservation {
	var retained []value.CachedObservation
	for _, item := range o.Retained {
		// Rows belonging to the selected generation already appear in coverage.
		covered := false
		for _, scope := range o.Generation.Coverage {
			if scope.Requested && scope.Scope == item.Kind+"s" && scope.Records == item.Records && item.Oldest == o.Generation.GeneratedAt && item.Newest == o.Generation.GeneratedAt {
				covered = true
				break
			}
		}
		if covered {
			continue
		}
		item.Oldest, item.Newest = "", ""
		retained = append(retained, item)
	}
	return retained
}

// StatusSource reads one local cache generation status.
type StatusSource interface {
	Status(context.Context, StatusInput) (StatusResult, error)
}

// ReadStatus returns current generation age, completeness, source, and stale state.
func ReadStatus(ctx context.Context, source StatusSource, input StatusInput) (StatusOutput, error) {
	result, err := source.Status(ctx, input)
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			return StatusOutput{}, statusError("cache.status.cancelled", errs.KindOperation, input, "Cache status was canceled.", err)
		}
		return StatusOutput{}, statusError("cache.status.failed", errs.KindOperation, input, "Cache status failed.", err)
	}
	state := "current"
	if result.Stale {
		state = "stale"
	}
	if !result.Complete {
		state = "partial"
	}
	if result.ID == "" {
		state = "uninitialized"
	}
	generation := StatusGeneration{ID: result.ID, Environment: result.Environment, Site: result.Site, GeneratedAt: result.GeneratedAt, Records: result.Records, Complete: result.Complete, Stale: result.Stale, Age: result.Age, Source: result.Source}
	generation.Coverage = result.Coverage
	return StatusOutput{Retained: result.Retained, Status: state, Generation: generation, Path: result.Path, Warnings: output.BoundWarnings(result.Warnings), Help: []string{commandhint.Environment(result.Environment, "cache", "refresh")}}, nil
}

func statusError(id string, kind errs.Kind, input StatusInput, summary string, cause error) error {
	advice := "Refresh or repair the selected cache generation, then retry."
	if incompatible, ok := errors.AsType[interface {
		error
		CacheSchemaIncompatible() bool
	}](cause); ok && incompatible.CacheSchemaIncompatible() {
		id = "cache.status.schema_incompatible"
		advice = "Preserve the cache and repair its schema or use a compatible TADX build; identical refresh attempts cannot repair inconsistent schema markers."
	}
	return &errs.Error{ID: id, Kind: kind, Operation: "cache.status", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: advice}
}
