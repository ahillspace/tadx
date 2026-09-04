package get

import (
	"context"
	"errors"
	"strings"

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
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.Name = strings.TrimSpace(input.Selector.Name)
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		message := "datasource selection requires a LUID or exact name and project path"
		return Output{}, &errs.Error{ID: "datasource.get.usage", Kind: errs.KindUsage, Operation: "datasource.get", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: message}}}
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		message := "a LUID is authoritative and cannot be combined with name or project selectors"
		return Output{}, &errs.Error{ID: "datasource.get.usage", Kind: errs.KindUsage, Operation: "datasource.get", Summary: "A LUID is authoritative and cannot be combined with name or project selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: message}}}
	}
	item, err := a.resolver.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return Output{}, err
		}
		var resolution *identity.ResolutionError
		if errors.As(err, &resolution) {
			switch resolution.Kind {
			case identity.ResolutionAmbiguous:
				return Output{}, resolveUsageError("datasource.get.ambiguous", input, "Datasource selector matched more than one datasource.", err)
			case identity.ResolutionNotFound:
				return Output{}, resolveUsageError("datasource.get.not_found", input, "No datasource matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return Output{}, resolveUsageError("datasource.get.usage", input, "Datasource selection requires a LUID or exact name and project path.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact datasource selector, then retry.")
		return Output{}, &errs.Error{ID: "datasource.get.resolve", Kind: errs.KindOperation, Operation: "datasource.get", Environment: input.Environment, Site: input.Site, Summary: "Datasource resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if item.LUID == "" || item.Name == "" || (input.Selector.LUID != "" && item.LUID != string(input.Selector.LUID)) {
		return Output{}, &errs.Error{ID: "datasource.get.identity_mismatch", Kind: errs.KindOperation, Operation: "datasource.get", Environment: input.Environment, Site: input.Site, Summary: "Datasource adapter returned a mismatched authoritative identity.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path."}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Datasource: item, RequestID: item.RequestID, Help: []string{"tadx content datasource list"}}, nil
}

func resolveUsageError(id string, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "datasource.get", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", TableauRequestID: errs.TableauRequestID(cause)}
}
