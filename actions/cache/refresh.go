// Package cache provides local cache actions.
package cache

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/value"
)

// RefreshInput selects one resolved site and the cache scopes to hydrate.
type RefreshInput struct {
	Environment string
	Site        string
	Scopes      []string
	Preview     bool
}

// RefreshHydrationRequest is the normalized action-owned request passed to storage-backed hydration.
type RefreshHydrationRequest struct {
	Environment     string
	Site            string
	RequestedScopes []string
	ImplicitScopes  []string
}

// RefreshScopeCount is one bounded per-scope hydration count.
type RefreshScopeCount struct {
	Scope   string `json:"scope"`
	Records int    `json:"records"`
}

// RefreshDiagnostics contains bounded operational hydration measurements.
type RefreshDiagnostics struct {
	Requests       int    `json:"requests"`
	FailedRequests int    `json:"failed_requests"`
	Duration       string `json:"duration,omitempty"`
}

// RefreshHydrationResult is a row-free receipt for one internally persisted generation.
type RefreshHydrationResult struct {
	GenerationID        string
	GeneratedAt         time.Time
	Complete            bool
	Source              string
	Path                string
	RecordCount         int
	HydratedRecordCount int
	RequestedScopes     []string
	ImplicitScopes      []string
	ScopeCounts         []RefreshScopeCount
	Diagnostics         RefreshDiagnostics
	Warnings            []string
	DeniedPermissions   int
}

// RefreshGenerationOutput is the bounded refresh generation projection.
type RefreshGenerationOutput struct {
	Coverage        []value.CacheCoverage `json:"coverage,omitempty"`
	Complete        bool                  `json:"complete"`
	ID              string                `json:"id"`
	Environment     string                `json:"environment"`
	Site            string                `json:"site"`
	GeneratedAt     string                `json:"generated_at"`
	Records         int                   `json:"records"`
	HydratedRecords int                   `json:"hydrated_records,omitzero"`
	Source          string                `json:"source,omitempty"`
	Scopes          []string              `json:"scopes,omitempty"`
	ImplicitScopes  []string              `json:"implicit_scopes,omitempty"`
	ScopeCounts     []RefreshScopeCount   `json:"scope_counts,omitempty"`
}

// RefreshOutput is the stable cache.refresh document.
type RefreshOutput struct {
	Plan        *RefreshPlan
	Status      string
	Generation  RefreshGenerationOutput
	Path        string
	Warnings    []string
	Diagnostics RefreshDiagnostics
	Help        []string
}

// RefreshPlan identifies the inventory generation request without hydrating or publishing it.
type RefreshPlan struct {
	Environment     string   `json:"environment"`
	Site            string   `json:"site"`
	RequestedScopes []string `json:"requested_scopes"`
	ImplicitScopes  []string `json:"implicit_scopes"`
}

// RefreshPreviewResult is the preview result for one cache refresh request.
type RefreshPreviewResult struct {
	Status string      `json:"status"`
	Plan   RefreshPlan `json:"plan"`
	Help   []string    `json:"help"`
}

// RefreshCompactGeneration contains refresh decision fields.
type RefreshCompactGeneration struct {
	Coverage    []value.CacheCoverage `json:"coverage,omitempty"`
	Complete    bool                  `json:"complete"`
	ID          string                `json:"id"`
	Environment string                `json:"environment"`
	Site        string                `json:"site"`
	GeneratedAt string                `json:"generated_at"`
	Records     int                   `json:"records"`
}

// RefreshCompactResult is the default bounded refresh projection.
type RefreshCompactResult struct {
	Status     string                   `json:"status"`
	Generation RefreshCompactGeneration `json:"generation"`
	Path       string                   `json:"path"`
	Warnings   []string                 `json:"warnings,omitempty"`
	Details    string                   `json:"details"`
	Help       []string                 `json:"help"`
}

// RefreshFullResult is the expanded bounded refresh projection.
type RefreshFullResult struct {
	Status      string                  `json:"status"`
	Generation  RefreshGenerationOutput `json:"generation"`
	Path        string                  `json:"path"`
	Warnings    []string                `json:"warnings,omitempty"`
	Diagnostics RefreshDiagnostics      `json:"diagnostics"`
	Help        []string                `json:"help"`
}

// CompactOutput returns a row-free operational receipt.
func (o RefreshOutput) CompactOutput() any {
	if o.Plan != nil {
		return RefreshPreviewResult{Status: o.Status, Plan: *o.Plan, Help: o.Help}
	}
	g := o.Generation
	return RefreshCompactResult{Status: o.Status, Generation: RefreshCompactGeneration{Coverage: g.Coverage, Complete: g.Complete, ID: g.ID, Environment: g.Environment, Site: g.Site, GeneratedAt: g.GeneratedAt, Records: g.Records}, Path: o.Path, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded generation provenance and diagnostics.
func (o RefreshOutput) FullOutput() any {
	if o.Plan != nil {
		return RefreshPreviewResult{Status: o.Status, Plan: *o.Plan, Help: o.Help}
	}
	return RefreshFullResult{Status: o.Status, Generation: o.Generation, Path: o.Path, Warnings: o.Warnings, Diagnostics: o.Diagnostics, Help: o.Help}
}

// ValidateRefreshInput checks scopes without requiring resolved credentials or a site.
func ValidateRefreshInput(input RefreshInput) error {
	_, _, err := normalizeScopes(input.Scopes)
	if err != nil {
		return failure("cache.refresh.usage", errs.KindUsage, input, err.Error(), err)
	}
	return nil
}

const (
	maxReceiptRunes = 512
)

var supportedScopes = []string{"users", "groups", "projects", "workbooks", "datasources", "flows", "views", "permissions"}

// RefreshHydrator persists one complete cache generation internally and returns only its receipt.
type RefreshHydrator interface {
	Hydrate(context.Context, RefreshHydrationRequest) (RefreshHydrationResult, error)
}

// Refresh plans a resolved cache target and returns a row-free hydration receipt.
func Refresh(ctx context.Context, hydrator RefreshHydrator, input RefreshInput) (RefreshOutput, error) {
	requested, implicit, err := normalizeScopes(input.Scopes)
	if err != nil {
		return RefreshOutput{}, failure("cache.refresh.usage", errs.KindUsage, input, err.Error(), err)
	}
	request := RefreshHydrationRequest{
		Environment:     input.Environment,
		Site:            input.Site,
		RequestedScopes: requested,
		ImplicitScopes:  implicit,
	}
	if input.Preview {
		return RefreshOutput{Status: "preview", Plan: &RefreshPlan{Environment: input.Environment, Site: input.Site, RequestedScopes: requested, ImplicitScopes: implicit}, Help: []string{"Run without --preview to hydrate and publish this local inventory generation. Preview validates configuration and scopes; provider access and inventory completeness are checked during refresh."}}, nil
	}
	result, err := hydrator.Hydrate(ctx, request)
	if err != nil {
		return RefreshOutput{}, classify(err, input)
	}
	if err := validateResult(result, request); err != nil {
		return RefreshOutput{}, failure("cache.refresh.incomplete", errs.KindOperation, input, "Cache inventory is incomplete and was not published.", err)
	}
	generation := RefreshGenerationOutput{
		Complete:       result.Complete,
		ID:             result.GenerationID,
		Environment:    input.Environment,
		Site:           input.Site,
		GeneratedAt:    result.GeneratedAt.UTC().Format("2006-01-02T15:04:05Z07:00"),
		Records:        result.RecordCount,
		Source:         result.Source,
		Scopes:         slices.Clone(requested),
		ImplicitScopes: slices.Clone(implicit),
		ScopeCounts:    slices.Clone(result.ScopeCounts),
	}
	if result.HydratedRecordCount != result.RecordCount {
		generation.HydratedRecords = result.HydratedRecordCount
	}
	for _, count := range result.ScopeCounts {
		generation.Coverage = append(generation.Coverage, value.CacheCoverage{Scope: count.Scope, Records: count.Records, Requested: slices.Contains(requested, count.Scope), Complete: result.Complete || count.Scope != "permissions"})
	}
	state := "refreshed"
	if !result.Complete {
		state = "partial"
	}
	return RefreshOutput{
		Status:      state,
		Generation:  generation,
		Path:        result.Path,
		Warnings:    output.BoundWarnings(result.Warnings),
		Diagnostics: result.Diagnostics,
		Help:        []string{commandhint.Environment(input.Environment, "cache", "status")},
	}, nil
}

func normalizeScopes(values []string) ([]string, []string, error) {
	if len(values) == 0 {
		return slices.Clone(supportedScopes), []string{}, nil
	}
	selected := make(map[string]bool, len(values))
	for _, value := range values {
		if !slices.Contains(supportedScopes, value) {
			return nil, nil, errors.New("Cache refresh scope must be exactly one of users, groups, projects, workbooks, datasources, flows, views, or permissions.")
		}
		if selected[value] {
			return nil, nil, errors.New("Cache refresh scopes must not contain duplicates.")
		}
		selected[value] = true
	}
	requested := scopesInCanonicalOrder(selected)
	dependencies := make(map[string]bool)
	for scope := range selected {
		switch scope {
		case "workbooks", "datasources", "flows":
			dependencies["projects"] = true
		case "views", "permissions":
			dependencies["projects"] = true
			dependencies["workbooks"] = true
		}
	}
	for scope := range selected {
		delete(dependencies, scope)
	}
	return requested, scopesInCanonicalOrder(dependencies), nil
}

func scopesInCanonicalOrder(selected map[string]bool) []string {
	result := make([]string, 0, len(selected))
	for _, scope := range supportedScopes {
		if selected[scope] {
			result = append(result, scope)
		}
	}
	return result
}

func validateResult(result RefreshHydrationResult, request RefreshHydrationRequest) error {
	partialPermissions := result.DeniedPermissions > 0 && slices.Contains(request.RequestedScopes, "permissions") && !result.Complete && len(result.Warnings) > 0
	if !result.Complete && !partialPermissions {
		return errors.New("cache hydration did not complete")
	}
	if result.DeniedPermissions < 0 || (result.DeniedPermissions > 0 && !partialPermissions) {
		return errors.New("cache hydration returned invalid permission coverage")
	}
	if strings.TrimSpace(result.GenerationID) == "" || result.GeneratedAt.IsZero() || strings.TrimSpace(result.Source) == "" {
		return errors.New("cache hydration omitted generation provenance")
	}
	for _, value := range []string{result.GenerationID, result.Source, result.Path, result.Diagnostics.Duration} {
		if utf8.RuneCountInString(value) > maxReceiptRunes {
			return errors.New("cache hydration receipt exceeds the output bound")
		}
	}
	if result.RecordCount < 0 || result.HydratedRecordCount < result.RecordCount || result.Diagnostics.Requests < 0 || result.Diagnostics.FailedRequests < 0 {
		return errors.New("cache hydration returned invalid operational counts")
	}
	if result.Diagnostics.FailedRequests != result.DeniedPermissions {
		return errors.New("cache hydration completed with failed requests")
	}
	if !slices.Equal(result.RequestedScopes, request.RequestedScopes) || !slices.Equal(result.ImplicitScopes, request.ImplicitScopes) {
		return errors.New("cache hydration receipt scopes do not match the request")
	}
	if err := validateRelativePath(result.Path); err != nil {
		return err
	}
	seen := make(map[string]bool, len(result.ScopeCounts))
	lastIndex := -1
	countedRecords := 0
	for _, count := range result.ScopeCounts {
		index := slices.Index(supportedScopes, count.Scope)
		selected := slices.Contains(request.RequestedScopes, count.Scope) || slices.Contains(request.ImplicitScopes, count.Scope)
		if index < 0 || !selected || seen[count.Scope] || count.Records < 0 || index <= lastIndex {
			return errors.New("cache hydration returned invalid scope counts")
		}
		seen[count.Scope] = true
		lastIndex = index
		countedRecords += count.Records
	}
	if len(result.ScopeCounts) > 0 && countedRecords != result.HydratedRecordCount {
		return errors.New("cache hydration scope counts do not equal the total hydrated record count")
	}
	return nil
}

func validateRelativePath(value string) error {
	if strings.TrimSpace(value) == "" || filepath.IsAbs(value) || filepath.VolumeName(value) != "" || strings.Contains(value, `\`) || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return errors.New("cache hydration path must be relative and slash-delimited")
	}
	return nil
}

func classify(err error, input RefreshInput) error {
	if incompatible, ok := errors.AsType[interface {
		error
		CacheSchemaIncompatible() bool
	}](err); ok && incompatible.CacheSchemaIncompatible() {
		return &errs.Error{ID: "cache.refresh.schema_incompatible", Kind: errs.KindOperation, Operation: "cache.refresh", Environment: input.Environment, Site: input.Site, Summary: "The selected cache schema is inconsistent or unsupported; no replacement was published.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Preserve this cache and repair its schema or use a compatible TADX build. Use explicit live reads in the meantime; do not repeat this refresh unchanged.", Phase: errs.PhasePersistence, Outcome: errs.OutcomeNotAttempted}
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return failure("cache.refresh.cancelled", errs.KindOperation, input, "Cache refresh was canceled before publication.", err)
	}
	return failure("cache.refresh.failed", errs.KindOperation, input, "Cache refresh failed during hydration.", err)
}

func failure(id string, kind errs.Kind, input RefreshInput, summary string, cause error) error {
	retryable := errs.Bool(false)
	correctiveAction := "Resolve the reported cache refresh problem, then retry."
	if cause != nil && kind == errs.KindOperation {
		retryable, correctiveAction = errs.CompleteRetryAdvice(cause, correctiveAction)
	}
	return &errs.Error{
		ID: id, Kind: kind, Operation: "cache.refresh", Environment: input.Environment, Site: input.Site,
		Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction,
		TableauRequestID: errs.TableauRequestID(cause),
	}
}
