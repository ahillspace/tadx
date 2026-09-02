package get

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned workbook seam.
type Resolver interface {
	ResolveWorkbook(context.Context, identity.Selector) (Workbook, error)
}

// Action gets one workbook.
type Action struct{ resolver Resolver }

// New creates a workbook get action.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative workbook.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "workbook.get.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.get", Summary: "Workbook get is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook resolution before retrying."}
	}
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return Output{}, &errs.Error{ID: "workbook.get.usage", Kind: errs.KindUsage, Operation: "workbook.get", Summary: "Workbook selection requires a LUID or exact name and project path.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "workbook selection requires a LUID or exact name and project path"}}}
	}
	workbook, err := a.resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workbook selector, then retry.")
		return Output{}, &errs.Error{ID: "workbook.get.resolve", Kind: errs.KindOperation, Operation: "workbook.get", Environment: input.Environment, Site: input.Site, Summary: "Workbook resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Workbook: workbook, RequestID: workbook.RequestID, Help: []string{"tadx content workbook pull --id " + workbook.LUID}}, nil
}
