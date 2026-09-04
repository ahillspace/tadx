package list

import (
	"context"
	"sort"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	// DefaultLimit bounds discovery output when the caller does not choose a limit.
	DefaultLimit = 20
	// MaxLimit is the largest permitted discovery page.
	MaxLimit = 100
)

// Source supplies registry discovery views.
type Source interface {
	List(context.Context) ([]Capability, error)
}

// Action lists capabilities without depending on CLI plumbing.
type Action struct {
	source Source
}

// New constructs a capability list action.
func New(source Source) *Action {
	return &Action{source: source}
}

// Execute returns a deterministic, bounded capability inventory.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.source == nil {
		return Output{}, &errs.Error{ID: "capability.list.unconfigured", Kind: errs.KindRuntime, Operation: "capability.list", Summary: "Capability list is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure a capability source before retrying."}
	}
	limit := input.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return Output{}, usageError("limit must be between 1 and 100")
	}
	offset := 0
	if input.Cursor != "" {
		parsed, err := strconv.Atoi(input.Cursor)
		if err != nil || parsed < 0 {
			return Output{}, usageError("cursor must be a non-negative integer")
		}
		offset = parsed
	}

	items, err := a.source.List(ctx)
	if err != nil {
		return Output{}, err
	}
	if items == nil {
		items = []Capability{}
	}
	for index := range items {
		items[index].ExecutionEnabled = items[index].State == "implemented" && (!items[index].RemoteMutation || input.MutationsEnabled)
	}
	sort.Slice(items, func(i, j int) bool { return items[i].ID < items[j].ID })
	filtered := items[:0]
	for _, item := range items {
		if matches(input, item) {
			filtered = append(filtered, item)
		}
	}
	if offset > len(filtered) {
		return Output{}, usageError("cursor is past the end of the filtered result")
	}
	end := min(offset+limit, len(filtered))
	pageItems := append([]Capability(nil), filtered[offset:end]...)
	if pageItems == nil {
		pageItems = []Capability{}
	}
	nextCursor := ""
	if end < len(filtered) {
		nextCursor = strconv.Itoa(end)
	}
	return Output{
		Page:         Pagination{Returned: len(pageItems), Total: len(filtered), Limit: limit, NextCursor: nextCursor},
		Capabilities: pageItems,
		Help:         help(input, limit, nextCursor),
	}, nil
}

func matches(input Input, item Capability) bool {
	if !equalFilter(input.Domain, item.Domain) || !equalFilter(input.Resource, item.Resource) || !equalFilter(input.Owner, item.Owner) {
		return false
	}
	if input.Product != "" && !strings.Contains(strings.ToLower(item.Product), strings.ToLower(input.Product)) {
		return false
	}
	return input.Mutation == nil || item.RemoteMutation == *input.Mutation
}

func equalFilter(filter, value string) bool {
	return filter == "" || strings.EqualFold(filter, value)
}

func help(input Input, limit int, nextCursor string) []string {
	result := []string{"tadx capability get <id>"}
	if nextCursor == "" {
		return result
	}
	parts := []string{"tadx capability list"}
	for _, field := range []struct{ name, value string }{
		{"domain", input.Domain}, {"resource", input.Resource}, {"owner", input.Owner}, {"product", input.Product},
	} {
		if field.value != "" {
			parts = append(parts, "--"+field.name, commandArgument(field.value))
		}
	}
	if input.Mutation != nil {
		parts = append(parts, "--mutation="+strconv.FormatBool(*input.Mutation))
	}
	parts = append(parts, "--limit", strconv.Itoa(limit), "--cursor", nextCursor)
	return append(result, strings.Join(parts, " "))
}

func commandArgument(value string) string {
	if strings.ContainsAny(value, " \t\r\n\"") {
		return strconv.Quote(value)
	}
	return value
}

func usageError(summary string) error {
	return &errs.Error{ID: "capability.list.usage", Kind: errs.KindUsage, Operation: "capability.list", Summary: summary}
}
