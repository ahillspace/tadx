// Package refresh orchestrates complete cache generation hydration.
package refresh

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

// Input selects one resolved site and the cache scopes to hydrate.
type Input struct {
	Environment string
	Site        string
	Scopes      []string
	Preview     bool
}

// HydrationRequest is the normalized action-owned request passed to storage-backed hydration.
type HydrationRequest struct {
	Environment     string
	Site            string
	RequestedScopes []string
	ImplicitScopes  []string
}

// ScopeCount is one bounded per-scope hydration count.
type ScopeCount struct {
	Scope   string `json:"scope"`
	Records int    `json:"records"`
}

// Diagnostics contains bounded operational hydration measurements.
type Diagnostics struct {
	Requests       int    `json:"requests"`
	FailedRequests int    `json:"failed_requests"`
	Duration       string `json:"duration,omitempty"`
}

// HydrationResult is a row-free receipt for one internally persisted generation.
type HydrationResult struct {
	GenerationID        string
	GeneratedAt         time.Time
	Complete            bool
	Source              string
	Path                string
	RecordCount         int
	HydratedRecordCount int
	RequestedScopes     []string
	ImplicitScopes      []string
	ScopeCounts         []ScopeCount
	Diagnostics         Diagnostics
	Warnings            []string
	DeniedPermissions   int
}

// GenerationOutput is the bounded refresh generation projection.
type GenerationOutput struct {
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
	ScopeCounts     []ScopeCount          `json:"scope_counts,omitempty"`
}

// Output is the stable cache.refresh document.
type Output struct {
	Plan        *Plan            `json:"-"`
	Status      string           `json:"status"`
	Generation  GenerationOutput `json:"generation"`
	Path        string           `json:"path"`
	Warnings    []string         `json:"warnings,omitempty"`
	Diagnostics Diagnostics      `json:"diagnostics"`
	Help        []string         `json:"help"`
}

// Plan identifies the inventory generation request without hydrating or publishing it.
type Plan struct {
	Environment     string   `json:"environment"`
	Site            string   `json:"site"`
	RequestedScopes []string `json:"requested_scopes"`
	ImplicitScopes  []string `json:"implicit_scopes"`
}

type PreviewResult struct {
	Status string   `json:"status"`
	Plan   Plan     `json:"plan"`
	Help   []string `json:"help"`
}

// CompactGeneration contains refresh decision fields.
type CompactGeneration struct {
	Coverage    []value.CacheCoverage `json:"coverage,omitempty"`
	Complete    bool                  `json:"complete"`
	ID          string                `json:"id"`
	Environment string                `json:"environment"`
	Site        string                `json:"site"`
	GeneratedAt string                `json:"generated_at"`
	Records     int                   `json:"records"`
}

// CompactResult is the default bounded refresh projection.
type CompactResult struct {
	Status     string            `json:"status"`
	Generation CompactGeneration `json:"generation"`
	Path       string            `json:"path"`
	Warnings   []string          `json:"warnings,omitempty"`
	Details    string            `json:"details"`
	Help       []string          `json:"help"`
}

// CompactOutput returns a row-free operational receipt.
func (o Output) CompactOutput() any {
	if o.Plan != nil {
		return PreviewResult{Status: o.Status, Plan: *o.Plan, Help: o.Help}
	}
	g := o.Generation
	return CompactResult{Status: o.Status, Generation: CompactGeneration{Coverage: g.Coverage, Complete: g.Complete, ID: g.ID, Environment: g.Environment, Site: g.Site, GeneratedAt: g.GeneratedAt, Records: g.Records}, Path: o.Path, Warnings: o.Warnings, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded generation provenance and diagnostics.
func (o Output) FullOutput() any {
	if o.Plan != nil {
		return PreviewResult{Status: o.Status, Plan: *o.Plan, Help: o.Help}
	}
	return o
}

// ValidateInput checks scopes without requiring resolved credentials or a site.
func ValidateInput(input Input) error {
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

// Hydrator persists one complete cache generation internally and returns only its receipt.
type Hydrator interface {
	Hydrate(context.Context, HydrationRequest) (HydrationResult, error)
}

// Refresh plans a resolved cache target and returns a row-free hydration receipt.
func Refresh(ctx context.Context, hydrator Hydrator, input Input) (Output, error) {
	requested, implicit, err := normalizeScopes(input.Scopes)
	if err != nil {
		return Output{}, failure("cache.refresh.usage", errs.KindUsage, input, err.Error(), err)
	}
	request := HydrationRequest{
		Environment:     input.Environment,
		Site:            input.Site,
		RequestedScopes: requested,
		ImplicitScopes:  implicit,
	}
	if input.Preview {
		return Output{Status: "preview", Plan: &Plan{Environment: input.Environment, Site: input.Site, RequestedScopes: requested, ImplicitScopes: implicit}, Help: []string{"Run without --preview to hydrate and publish this local inventory generation. Preview validates configuration and scopes; provider access and inventory completeness are checked during refresh."}}, nil
	}
	result, err := hydrator.Hydrate(ctx, request)
	if err != nil {
		return Output{}, classify(err, input)
	}
	if err := validateResult(result, request); err != nil {
		return Output{}, failure("cache.refresh.incomplete", errs.KindOperation, input, "Cache inventory is incomplete and was not published.", err)
	}
	generation := GenerationOutput{
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
	return Output{
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

func validateResult(result HydrationResult, request HydrationRequest) error {
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

func classify(err error, input Input) error {
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

func failure(id string, kind errs.Kind, input Input, summary string, cause error) error {
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
