package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned workbook seam.
type Resolver interface {
	ResolveWorkbook(context.Context, identity.Selector) (Workbook, error)
}

// Action inspects one workbook.
type Action struct{ resolver Resolver }

// New creates a workbook inspect action.
func New(resolver Resolver) *Action { return &Action{resolver: resolver} }

// Execute resolves one authoritative workbook.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "workbook.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.inspect", Summary: "Workbook inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook resolution before retrying."}
	}
	var validationErr error
	input, validationErr = normalizeInput(input)
	if validationErr != nil {
		return Output{}, validationErr
	}
	workbook, err := a.resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return Output{}, err
		}
		var resolution *identity.ResolutionError
		if errors.As(err, &resolution) {
			switch resolution.Kind {
			case identity.ResolutionAmbiguous:
				return Output{}, resolveUsageError("workbook.inspect.ambiguous", input, "Workbook selector matched more than one workbook.", err)
			case identity.ResolutionNotFound:
				return Output{}, resolveUsageError("workbook.inspect.not_found", input, "No workbook matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return Output{}, resolveUsageError("workbook.inspect.usage", input, "Workbook selection requires a LUID or exact name with --project or --project-id.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workbook selector, then retry.")
		return Output{}, &errs.Error{ID: "workbook.inspect.resolve", Kind: errs.KindOperation, Operation: "workbook.inspect", Environment: input.Environment, Site: input.Site, Summary: "Workbook resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if workbook.LUID == "" || workbook.Name == "" || (input.Selector.LUID != "" && workbook.LUID != string(input.Selector.LUID)) || (input.Selector.Name != "" && workbook.Name != input.Selector.Name) || (input.Selector.ProjectLUID != "" && workbook.ProjectLUID != string(input.Selector.ProjectLUID)) {
		return Output{}, &errs.Error{ID: "workbook.inspect.identity_mismatch", Kind: errs.KindOperation, Operation: "workbook.inspect", Environment: input.Environment, Site: input.Site, Summary: "Workbook adapter returned a mismatched authoritative identity.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id."}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Workbook: workbook, RequestID: workbook.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "workbook", "pull", "--id", workbook.LUID)}}, nil
}

func resolveUsageError(id string, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "workbook.inspect", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", TableauRequestID: errs.TableauRequestID(cause)}
}
