package env

import (
	"cmp"
	"context"
	"slices"
	"strconv"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

const (
	// ListDefaultLimit bounds profile output when the caller does not choose a limit.
	ListDefaultLimit = 20
	// ListMaxLimit is the largest permitted profile page.
	ListMaxLimit = 10000
)

type ListReader interface {
	List(context.Context) ([]Profile, error)
}

// List returns one bounded page of stored environment profiles.
func (s *Service) List(ctx context.Context, input ListInput) (ListOutput, error) {
	if s == nil || s.store == nil {
		return ListOutput{}, &errs.Error{ID: "env.profile.list.unconfigured", Kind: errs.KindRuntime, Operation: "env.profile.list", Summary: "Environment profile listing is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the environment profile store before retrying."}
	}
	if input.All && (input.Limit != 0 || input.Cursor != "") {
		return ListOutput{}, listUsageError("--all cannot be combined with --limit or --cursor")
	}
	limit := input.Limit
	if input.All {
		limit = ListMaxLimit
	} else if limit == 0 {
		limit = ListDefaultLimit
	}
	if limit < 1 || limit > ListMaxLimit {
		return ListOutput{}, listUsageError("limit must be between 1 and 10000")
	}
	offset := 0
	if input.Cursor != "" {
		value, err := strconv.Atoi(input.Cursor)
		if err != nil || value < 0 {
			return ListOutput{}, listUsageError("cursor must be a non-negative integer")
		}
		offset = value
	}
	profiles, err := s.store.List(ctx)
	if err != nil {
		retryable, advice := errs.CompleteRetryAdvice(err, "Review the selected configuration file, then retry.")
		return ListOutput{}, &errs.Error{ID: "env.profile.list.read", Kind: errs.KindOperation, Operation: "env.profile.list", Summary: "Environment profiles could not be read.", Cause: err, Retryable: retryable, CorrectiveAction: advice, Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	profiles = append([]Profile(nil), profiles...)
	slices.SortFunc(profiles, func(left, right Profile) int { return cmp.Compare(left.Alias, right.Alias) })
	if input.All && len(profiles) > ListMaxLimit {
		return ListOutput{}, listUsageError("--all exceeds the 10000-record bound")
	}
	if offset > len(profiles) {
		return ListOutput{}, listUsageError("cursor is past the end of the result")
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
	var help []string
	if len(pageProfiles) > 0 {
		help = append(help, commandhint.Command("env", "get", pageProfiles[0].Alias))
	}
	if nextCursor != "" {
		help = append(help, "tadx env list --limit "+strconv.Itoa(min(limit*2, ListMaxLimit)))
	}
	return ListOutput{Page: Page{Returned: len(pageProfiles), Total: len(profiles), Limit: limit, NextCursor: nextCursor}, Profiles: pageProfiles, Help: help}, nil
}

func listUsageError(summary string) error {
	return &errs.Error{ID: "env.profile.list.usage", Kind: errs.KindUsage, Operation: "env.profile.list", Summary: summary}
}

type ListInput struct {
	All    bool   `json:"all,omitzero"`
	Cursor string `json:"cursor,omitempty"`
	Limit  int    `json:"limit,omitempty"`
}

type Page = output.Page

type ListOutput struct {
	Page     Page      `json:"page"`
	Profiles []Profile `json:"environments"`
	Help     []string  `json:"help"`
}

type ListCompactProfile struct {
	Alias   string `json:"alias"`
	Default bool   `json:"default"`
}

type ListCompactResult struct {
	Page     Page                 `json:"page"`
	Profiles []ListCompactProfile `json:"environments"`
	Details  string               `json:"details"`
	Help     []string             `json:"help"`
}

func (o ListOutput) CompactOutput() any {
	profiles := make([]ListCompactProfile, len(o.Profiles))
	for index, profile := range o.Profiles {
		profiles[index] = ListCompactProfile{Alias: profile.Alias, Default: profile.Default}
	}
	return ListCompactResult{Page: o.Page, Profiles: profiles, Details: "--full", Help: o.Help}
}

func (o ListOutput) FullOutput() any { return o }
