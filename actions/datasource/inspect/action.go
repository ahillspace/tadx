package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned exact datasource seam.
type Resolver interface {
	ResolveDatasource(context.Context, identity.Selector) (Datasource, error)
}

// Action inspects one exact published datasource.
type Action struct{ resolver Resolver }

// New creates a datasource inspect action.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative published datasource.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "datasource.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.inspect", Summary: "Datasource inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the datasource resolver before retrying."}
	}
	var validationErr error
	input, validationErr = normalizeInput(input)
	if validationErr != nil {
		return Output{}, validationErr
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
				return Output{}, resolveUsageError("datasource.inspect.ambiguous", input, "Datasource selector matched more than one datasource.", err)
			case identity.ResolutionNotFound:
				return Output{}, resolveUsageError("datasource.inspect.not_found", input, "No datasource matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return Output{}, resolveUsageError("datasource.inspect.usage", input, "Datasource selection requires a LUID or exact name with --project or --project-id.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact datasource selector, then retry.")
		return Output{}, &errs.Error{ID: "datasource.inspect.resolve", Kind: errs.KindOperation, Operation: "datasource.inspect", Environment: input.Environment, Site: input.Site, Summary: "Datasource resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if item.LUID == "" || item.Name == "" || (input.Selector.LUID != "" && item.LUID != string(input.Selector.LUID)) || (input.Selector.Name != "" && item.Name != input.Selector.Name) || (input.Selector.ProjectLUID != "" && item.ProjectLUID != string(input.Selector.ProjectLUID)) {
		return Output{}, &errs.Error{ID: "datasource.inspect.identity_mismatch", Kind: errs.KindOperation, Operation: "datasource.inspect", Environment: input.Environment, Site: input.Site, Summary: "Datasource adapter returned a mismatched authoritative identity.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id."}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Datasource: item, RequestID: item.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "datasource", "schema", "--id", item.LUID)}}, nil
}

func resolveUsageError(id string, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "datasource.inspect", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", TableauRequestID: errs.TableauRequestID(cause)}
}
