package list

import (
	"cmp"
	"context"
	"slices"
	"strconv"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	// DefaultLimit bounds discovery output when the caller does not choose a limit.
	DefaultLimit = 20
	// MaxLimit is the largest permitted discovery page.
	MaxLimit = 10000
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
	if input.All && (input.Limit != 0 || input.Cursor != "") {
		return Output{}, usageError("--all cannot be combined with --limit or --cursor")
	}
	limit := input.Limit
	if input.All {
		limit = MaxLimit
	} else if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return Output{}, usageError("limit must be between 1 and 10000")
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
		items[index] = normalize(items[index])
		items[index].ExecutionEnabled = !items[index].PolicyDenied && items[index].ImplementationState == "implemented" && (!items[index].RemoteMutation || input.MutationsEnabled)
	}
	slices.SortFunc(items, func(left, right Capability) int { return cmp.Compare(left.ID, right.ID) })
	filtered := items[:0]
	for _, item := range items {
		if matches(input, item) {
			filtered = append(filtered, item)
		}
	}
	if input.All && len(filtered) > MaxLimit {
		return Output{}, usageError("--all exceeds the 10000-record bound; use narrower filters")
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
	if !input.All && end < len(filtered) {
		nextCursor = strconv.Itoa(end)
	}
	outOfScope := 0
	for _, item := range pageItems {
		if item.Disposition == "delegated" {
			outOfScope++
		}
	}
	help, nextCommand := help(input, limit, nextCursor, end, pageItems)
	policy := ""
	if input.MutationPolicyUnavailable {
		policy = "unavailable"
	}
	return Output{
		Page:           Pagination{Returned: len(pageItems), Total: len(filtered), Limit: limit, NextCursor: nextCursor},
		Capabilities:   pageItems,
		Counts:         Counts{Returned: len(pageItems), Matched: len(filtered), OutOfScope: outOfScope},
		MutationPolicy: policy,
		NextCommand:    nextCommand,
		Help:           help,
	}, nil
}

func normalize(item Capability) Capability {
	if item.ImplementationState == "" {
		item.ImplementationState = item.State
	}
	if item.State == "" {
		item.State = item.ImplementationState
	}
	if item.VerificationReadiness == "" && item.Blocked {
		item.VerificationReadiness = "blocked"
	}
	if item.Availability == "" {
		item.Availability = item.Product
	}
	if item.Product == "" {
		item.Product = item.Availability
	}
	return item
}

func matches(input Input, item Capability) bool {
	if !equalFilter(input.Domain, item.Domain) || !equalFilter(input.Resource, item.Resource) || !equalFilter(input.Owner, item.Owner) {
		return false
	}
	product := item.Availability
	if product == "" {
		product = item.Product
	}
	if input.Product != "" && !strings.Contains(strings.ToLower(product), strings.ToLower(input.Product)) {
		return false
	}
	return input.Mutation == nil || item.RemoteMutation == *input.Mutation
}

func equalFilter(filter, value string) bool {
	return filter == "" || strings.EqualFold(filter, value)
}

func help(input Input, limit int, nextCursor string, nextOffset int, items []Capability) ([]string, string) {
	var result []string
	if len(items) > 0 {
		result = []string{commandhint.Command("capability", "get", items[0].ID)}
	}
	if nextCursor == "" {
		return result, ""
	}
	parts := []string{"capability", "list"}
	for _, field := range []struct{ name, value string }{
		{"domain", input.Domain}, {"resource", input.Resource}, {"owner", input.Owner}, {"product", input.Product},
	} {
		if field.value != "" {
			parts = append(parts, "--"+field.name, field.value)
		}
	}
	if input.Mutation != nil {
		parts = append(parts, "--mutation="+strconv.FormatBool(*input.Mutation))
	}
	parts = append(parts, "--cursor", strconv.Itoa(nextOffset), "--limit", strconv.Itoa(limit))
	if input.Full {
		parts = append(parts, "--full")
	}
	if input.JSON {
		parts = append(parts, "--json")
	}
	nextCommand := commandhint.Command(parts...)
	result = append(result, "When more results are needed, use next_command.")
	return result, nextCommand
}

func usageError(summary string) error {
	return &errs.Error{ID: "capability.list.usage", Kind: errs.KindUsage, Operation: "capability.list", Summary: summary}
}
