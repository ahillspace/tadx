package get

import (
	"context"
	"errors"
	"strings"

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
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.Name = strings.TrimSpace(input.Selector.Name)
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return Output{}, &errs.Error{ID: "workbook.get.usage", Kind: errs.KindUsage, Operation: "workbook.get", Summary: "Workbook selection requires a LUID or exact name and project path.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "workbook selection requires a LUID or exact name and project path"}}}
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		return Output{}, &errs.Error{ID: "workbook.get.usage", Kind: errs.KindUsage, Operation: "workbook.get", Summary: "A LUID is authoritative and cannot be combined with name or project selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: "a LUID is authoritative and cannot be combined with name or project selectors"}}}
	}
	workbook, err := a.resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		var resolution *identity.ResolutionError
		if errors.As(err, &resolution) {
			switch resolution.Kind {
			case identity.ResolutionAmbiguous:
				return Output{}, resolveUsageError("workbook.get.ambiguous", input, "Workbook selector matched more than one workbook.", err)
			case identity.ResolutionNotFound:
				return Output{}, resolveUsageError("workbook.get.not_found", input, "No workbook matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return Output{}, resolveUsageError("workbook.get.usage", input, "Workbook selection requires a LUID or exact name and project path.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workbook selector, then retry.")
		return Output{}, &errs.Error{ID: "workbook.get.resolve", Kind: errs.KindOperation, Operation: "workbook.get", Environment: input.Environment, Site: input.Site, Summary: "Workbook resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if workbook.LUID == "" || workbook.Name == "" || (input.Selector.LUID != "" && workbook.LUID != string(input.Selector.LUID)) {
		return Output{}, &errs.Error{ID: "workbook.get.identity_mismatch", Kind: errs.KindOperation, Operation: "workbook.get", Environment: input.Environment, Site: input.Site, Summary: "Workbook adapter returned a mismatched authoritative identity.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path."}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Workbook: workbook, RequestID: workbook.RequestID, Help: []string{"tadx content workbook pull --id " + workbook.LUID}}, nil
}

func resolveUsageError(id string, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "workbook.get", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", TableauRequestID: errs.TableauRequestID(cause)}
}
