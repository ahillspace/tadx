// Package refresh orchestrates complete cache generation hydration.
package refresh

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"path/filepath"
	"slices"
	"strings"
	"unicode/utf8"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

const (
	maxReceiptRunes = 512
)

var supportedScopes = []string{"users", "groups", "projects", "workbooks", "datasources", "flows", "views", "permissions"}

// Hydrator persists one complete cache generation internally and returns only its receipt.
type Hydrator interface {
	Hydrate(context.Context, HydrationRequest) (HydrationResult, error)
}

// Action orchestrates cache.refresh.
type Action struct{ hydrator Hydrator }

// New creates cache.refresh.
func New(hydrator Hydrator) *Action { return &Action{hydrator: hydrator} }

// Execute validates one bounded hydration request and returns a row-free operational receipt.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || (!input.Preview && a.hydrator == nil) {
		return Output{}, failure("cache.refresh.unconfigured", errs.KindRuntime, input, "Cache refresh is not configured.", nil)
	}
	if strings.TrimSpace(input.Environment) == "" || !input.SiteResolved {
		return Output{}, failure("cache.refresh.usage", errs.KindUsage, input, "Cache refresh requires a resolved environment and site.", nil)
	}
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
	result, err := a.hydrator.Hydrate(ctx, request)
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
