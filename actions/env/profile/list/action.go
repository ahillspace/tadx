package list

import (
	"context"
	"sort"
	"strconv"

	"github.com/ahillspace/tadx/internal/errs"
)

const (
	DefaultLimit = 20
	MaxLimit     = 10000
)

type Reader interface {
	List(context.Context) ([]Profile, error)
}

type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }

func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil {
		return Output{}, &errs.Error{ID: "env.profile.list.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.list", Summary: "Environment profile listing is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	limit := input.Limit
	if limit == 0 {
		limit = DefaultLimit
	}
	if limit < 1 || limit > MaxLimit {
		return Output{}, usageError("limit must be between 1 and 10000")
	}
	offset := 0
	if input.Cursor != "" {
		value, err := strconv.Atoi(input.Cursor)
		if err != nil || value < 0 {
			return Output{}, usageError("cursor must be a non-negative integer")
		}
		offset = value
	}
	profiles, err := a.reader.List(ctx)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the selected configuration file, then retry.")
		return Output{}, &errs.Error{ID: "env.profile.list.read", Kind: errs.KindOperation, Operation: "env.profile.list", Summary: "Environment profiles could not be read.", Cause: err, Retryable: retryable, CorrectiveAction: advice}
	}
	profiles = append([]Profile(nil), profiles...)
	sort.Slice(profiles, func(i, j int) bool { return profiles[i].Alias < profiles[j].Alias })
	if offset > len(profiles) {
		return Output{}, usageError("cursor is past the end of the result")
	}
	end := min(offset+limit, len(profiles))
	pageProfiles := append([]Profile(nil), profiles[offset:end]...)
	if pageProfiles == nil {
		pageProfiles = []Profile{}
	}
	nextCursor := ""
	if end < len(profiles) {
		nextCursor = strconv.Itoa(end)
	}
	help := []string{"tadx env get <alias>"}
	if nextCursor != "" {
		help = append(help, "tadx env list --limit "+strconv.Itoa(min(limit*2, MaxLimit)))
	}
	return Output{Page: Page{Returned: len(pageProfiles), Total: len(profiles), Limit: limit, NextCursor: nextCursor}, Profiles: pageProfiles, Help: help}, nil
}

func usageError(summary string) error {
	return &errs.Error{ID: "env.profile.list.usage", Kind: errs.KindUsage, Operation: "env.profile.list", Summary: summary}
}
