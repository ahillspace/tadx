// Package refresh orchestrates complete catalog generation hydration.
package refresh

import (
	"context"
	"errors"
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

// Hydrator persists one complete catalog generation internally and returns only its receipt.
type Hydrator interface {
	Hydrate(context.Context, HydrationRequest) (HydrationResult, error)
}

// Action orchestrates catalog.refresh.
type Action struct{ hydrator Hydrator }

// New creates catalog.refresh.
func New(hydrator Hydrator) *Action { return &Action{hydrator: hydrator} }

// Execute validates one bounded hydration request and returns a row-free operational receipt.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.hydrator == nil {
		return Output{}, failure("catalog.refresh.unconfigured", errs.KindRuntime, input, "Catalog refresh is not configured.", nil)
	}
	if strings.TrimSpace(input.Environment) == "" || !input.SiteResolved {
		return Output{}, failure("catalog.refresh.usage", errs.KindUsage, input, "Catalog refresh requires a resolved environment and site.", nil)
	}
	requested, implicit, err := normalizeScopes(input.Scopes)
	if err != nil {
		return Output{}, failure("catalog.refresh.usage", errs.KindUsage, input, err.Error(), err)
	}
	request := HydrationRequest{
		Environment:     input.Environment,
		Site:            input.Site,
		RequestedScopes: requested,
		ImplicitScopes:  implicit,
	}
	result, err := a.hydrator.Hydrate(ctx, request)
	if err != nil {
		return Output{}, classify(err, input)
	}
	if err := validateResult(result, request); err != nil {
		return Output{}, failure("catalog.refresh.incomplete", errs.KindOperation, input, "Catalog inventory is incomplete and was not published.", err)
	}
	generation := GenerationOutput{
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
	return Output{
		Status:      "refreshed",
		Generation:  generation,
		Path:        result.Path,
		Warnings:    output.BoundWarnings(result.Warnings),
		Diagnostics: result.Diagnostics,
		Help:        []string{"tadx catalog status --environment " + input.Environment},
	}, nil
}

func normalizeScopes(values []string) ([]string, []string, error) {
	if len(values) == 0 {
		return slices.Clone(supportedScopes), []string{}, nil
	}
	selected := make(map[string]bool, len(values))
	for _, value := range values {
		if !slices.Contains(supportedScopes, value) {
			return nil, nil, errors.New("Catalog refresh scope must be exactly one of users, groups, projects, workbooks, datasources, flows, views, or permissions.")
		}
		if selected[value] {
			return nil, nil, errors.New("Catalog refresh scopes must not contain duplicates.")
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
	if !result.Complete {
		return errors.New("catalog hydration did not complete")
	}
	if strings.TrimSpace(result.GenerationID) == "" || result.GeneratedAt.IsZero() || strings.TrimSpace(result.Source) == "" {
		return errors.New("catalog hydration omitted generation provenance")
	}
	for _, value := range []string{result.GenerationID, result.Source, result.Path, result.Diagnostics.Duration} {
		if utf8.RuneCountInString(value) > maxReceiptRunes {
			return errors.New("catalog hydration receipt exceeds the output bound")
		}
	}
	if result.RecordCount < 0 || result.HydratedRecordCount < result.RecordCount || result.Diagnostics.Requests < 0 || result.Diagnostics.FailedRequests < 0 {
		return errors.New("catalog hydration returned invalid operational counts")
	}
	if result.Diagnostics.FailedRequests != 0 {
		return errors.New("catalog hydration completed with failed requests")
	}
	if !slices.Equal(result.RequestedScopes, request.RequestedScopes) || !slices.Equal(result.ImplicitScopes, request.ImplicitScopes) {
		return errors.New("catalog hydration receipt scopes do not match the request")
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
			return errors.New("catalog hydration returned invalid scope counts")
		}
		seen[count.Scope] = true
		lastIndex = index
		countedRecords += count.Records
	}
	if len(result.ScopeCounts) > 0 && countedRecords != result.HydratedRecordCount {
		return errors.New("catalog hydration scope counts do not equal the total hydrated record count")
	}
	return nil
}

func validateRelativePath(value string) error {
	if strings.TrimSpace(value) == "" || filepath.IsAbs(value) || filepath.VolumeName(value) != "" || strings.Contains(value, `\`) || strings.HasPrefix(value, "../") || strings.Contains(value, "/../") {
		return errors.New("catalog hydration path must be relative and slash-delimited")
	}
	return nil
}

func classify(err error, input Input) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return failure("catalog.refresh.cancelled", errs.KindOperation, input, "Catalog refresh was canceled before publication.", err)
	}
	return failure("catalog.refresh.failed", errs.KindOperation, input, "Catalog refresh failed during hydration.", err)
}

func failure(id string, kind errs.Kind, input Input, summary string, cause error) error {
	retryable := errs.Bool(false)
	correctiveAction := "Resolve the reported catalog refresh problem, then retry."
	if cause != nil && kind == errs.KindOperation {
		retryable, correctiveAction = errs.CompleteRetryAdvice(cause, correctiveAction)
	}
	return &errs.Error{
		ID: id, Kind: kind, Operation: "catalog.refresh", Environment: input.Environment, Site: input.Site,
		Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction,
		TableauRequestID: errs.TableauRequestID(cause),
	}
}
