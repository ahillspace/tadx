package workspace

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

// ListInput selects one bounded workspace registry page.
type ListInput struct {
	All    bool
	Limit  int
	Cursor string
}

// ListMaxLimit is the largest permitted workspace list page.
const ListMaxLimit = 10000

// ListedWorkspace is one complete registered workspace record.
type ListedWorkspace struct {
	Workspace

	Default       bool `json:"default"`
	Available     bool `json:"available"`
	ManifestValid bool `json:"manifest_valid"`
}

// ListPage is one complete bounded workspace page.
type ListPage struct {
	Returned   int               `json:"returned"`
	Total      int               `json:"total"`
	Limit      int               `json:"limit"`
	NextCursor string            `json:"-"`
	Items      []ListedWorkspace `json:"workspaces"`
}

// ListOutput is the stable list result.
type ListOutput struct {
	Page ListPage `json:"page"`
	Help []string `json:"help"`
}

type listCompactWorkspace struct {
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Available bool   `json:"available"`
}

type listCompactOutput struct {
	Page       output.Page            `json:"page"`
	Workspaces []listCompactWorkspace `json:"workspaces"`
	Details    string                 `json:"details"`
	Help       []string               `json:"help"`
}

type listFullOutput struct {
	Page       output.Page       `json:"page"`
	Workspaces []ListedWorkspace `json:"workspaces"`
	Help       []string          `json:"help"`
}

// CompactOutput returns workspace names and availability only.
func (o ListOutput) CompactOutput() any {
	items := make([]listCompactWorkspace, len(o.Page.Items))
	for index, item := range o.Page.Items {
		items[index] = listCompactWorkspace{Name: item.Name, Default: item.Default, Available: item.Available}
	}
	page := output.Page{Returned: o.Page.Returned, Total: o.Page.Total, Limit: o.Page.Limit, NextCursor: o.Page.NextCursor}
	return listCompactOutput{Page: page, Workspaces: items, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded stable identities for the same page.
func (o ListOutput) FullOutput() any {
	page := output.Page{Returned: o.Page.Returned, Total: o.Page.Total, Limit: o.Page.Limit, NextCursor: o.Page.NextCursor}
	return listFullOutput{Page: page, Workspaces: o.Page.Items, Help: o.Help}
}

// Lister reads one bounded registry page.
type Lister interface {
	List(context.Context, int, string) (ListPage, error)
}

// List lists one bounded page.
func (a *Service) List(ctx context.Context, input ListInput) (ListOutput, error) {
	if input.All {
		input.Limit = ListMaxLimit
	} else if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit < 1 || input.Limit > ListMaxLimit {
		return ListOutput{}, listUsage("limit must be between 1 and 10000")
	}
	page, err := a.Lister.List(ctx, input.Limit, input.Cursor)
	if err != nil {
		return ListOutput{}, &errs.Error{ID: "workspace.list.failed", Kind: errs.KindOperation, Operation: "workspace.list", Summary: "Workspace listing failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Repair the workspace registry, then retry.", Phase: errs.PhaseSetup, Outcome: errs.OutcomeNotAttempted}
	}
	if page.Returned != len(page.Items) || page.Returned > input.Limit {
		return ListOutput{}, listRuntimeError("workspace listing returned an invalid bounded page")
	}
	if input.All {
		if page.Total > ListMaxLimit {
			return ListOutput{}, listUsage("--all exceeds the 10000-record bound")
		}
		if page.Returned != page.Total || page.NextCursor != "" {
			return ListOutput{}, listRuntimeError("workspace listing --all returned an incomplete page")
		}
	}
	var help []string
	if len(page.Items) > 0 {
		help = []string{commandhint.Command("workspace", "status", "--workspace", page.Items[0].Name)}
	}
	return ListOutput{Page: page, Help: help}, nil
}

func listUsage(message string) error {
	return &errs.Error{ID: "workspace.list.usage", Kind: errs.KindUsage, Operation: "workspace.list", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Correct the workspace list input and retry."}
}

func listRuntimeError(message string) error {
	return &errs.Error{ID: "workspace.list.runtime", Kind: errs.KindRuntime, Operation: "workspace.list", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace storage before retrying."}
}
