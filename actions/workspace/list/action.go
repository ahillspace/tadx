// Package list implements workspace.list.
package list

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/output"

	"github.com/ahillspace/tadx/internal/errs"
)

// Input selects one bounded workspace registry page.
type Input struct {
	Limit  int
	Cursor string
}

// Workspace is one complete registered workspace record.
type Workspace struct {
	Name          string `json:"name"`
	ID            string `json:"id"`
	Root          string `json:"root"`
	Default       bool   `json:"default"`
	Available     bool   `json:"available"`
	ManifestValid bool   `json:"manifest_valid"`
}

// Page is one complete bounded workspace page.
type Page struct {
	Returned   int         `json:"returned"`
	Total      int         `json:"total"`
	Limit      int         `json:"limit"`
	NextCursor string      `json:"-"`
	Items      []Workspace `json:"workspaces"`
}

// Output is the stable list result.
type Output struct {
	Page Page     `json:"page"`
	Help []string `json:"help"`
}

type compactWorkspace struct {
	Name      string `json:"name"`
	Default   bool   `json:"default"`
	Available bool   `json:"available"`
}

type compactOutput struct {
	Page       pageSummary        `json:"page"`
	Workspaces []compactWorkspace `json:"workspaces"`
	Details    string             `json:"details"`
	Help       []string           `json:"help"`
}

type fullOutput struct {
	Page       pageSummary `json:"page"`
	Workspaces []Workspace `json:"workspaces"`
	Help       []string    `json:"help"`
}

type pageSummary = output.Page

// CompactOutput returns workspace names and availability only.
func (o Output) CompactOutput() any {
	items := make([]compactWorkspace, len(o.Page.Items))
	for index, item := range o.Page.Items {
		items[index] = compactWorkspace{Name: item.Name, Default: item.Default, Available: item.Available}
	}
	page := pageSummary{Returned: o.Page.Returned, Total: o.Page.Total, Limit: o.Page.Limit, NextCursor: o.Page.NextCursor}
	return compactOutput{Page: page, Workspaces: items, Details: "--full", Help: o.Help}
}

// FullOutput returns bounded stable identities for the same page.
func (o Output) FullOutput() any {
	page := pageSummary{Returned: o.Page.Returned, Total: o.Page.Total, Limit: o.Page.Limit, NextCursor: o.Page.NextCursor}
	return fullOutput{Page: page, Workspaces: o.Page.Items, Help: o.Help}
}

// Lister reads one bounded registry page.
type Lister interface {
	List(context.Context, int, string) (Page, error)
}

// Action orchestrates workspace.list.
type Action struct{ lister Lister }

// New creates workspace.list.
func New(lister Lister) *Action { return &Action{lister: lister} }

// Execute lists one bounded page.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.lister == nil {
		return Output{}, runtimeError("workspace listing is not configured")
	}
	if input.Limit == 0 {
		input.Limit = 20
	}
	if input.Limit < 1 || input.Limit > 10000 {
		return Output{}, usage("limit must be between 1 and 10000")
	}
	page, err := a.lister.List(ctx, input.Limit, input.Cursor)
	if err != nil {
		return Output{}, &errs.Error{ID: "workspace.list.failed", Kind: errs.KindOperation, Operation: "workspace.list", Summary: "Workspace listing failed.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Repair the workspace registry, then retry."}
	}
	if page.Returned != len(page.Items) || page.Returned > input.Limit {
		return Output{}, runtimeError("workspace listing returned an invalid bounded page")
	}
	var help []string
	if len(page.Items) > 0 {
		help = []string{commandhint.Command("workspace", "status", "--workspace", page.Items[0].Name)}
	}
	return Output{Page: page, Help: help}, nil
}

func usage(message string) error {
	return &errs.Error{ID: "workspace.list.usage", Kind: errs.KindUsage, Operation: "workspace.list", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Correct the workspace list input and retry."}
}

func runtimeError(message string) error {
	return &errs.Error{ID: "workspace.list.runtime", Kind: errs.KindRuntime, Operation: "workspace.list", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Configure named workspace storage before retrying."}
}
