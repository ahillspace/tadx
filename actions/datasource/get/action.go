package get

import (
	"context"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned exact datasource seam.
type Resolver interface {
	ResolveDatasource(context.Context, identity.Selector) (Datasource, error)
}

// Action gets one exact published datasource.
type Action struct{ resolver Resolver }

// New creates a datasource get action.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative published datasource.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "datasource.get.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.get", Summary: "Datasource get is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the datasource resolver before retrying."}
	}
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		message := "datasource selection requires a LUID or exact name and project path"
		return Output{}, &errs.Error{ID: "datasource.get.usage", Kind: errs.KindUsage, Operation: "datasource.get", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: message}}}
	}
	item, err := a.resolver.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact datasource selector, then retry.")
		return Output{}, &errs.Error{ID: "datasource.get.resolve", Kind: errs.KindOperation, Operation: "datasource.get", Environment: input.Environment, Site: input.Site, Summary: "Datasource resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Datasource: item, RequestID: item.RequestID, Help: []string{"tadx content datasource list"}}, nil
}
