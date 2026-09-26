package workbook

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Inspect resolves one authoritative workbook.
func Inspect(ctx context.Context, resolver Resolver, input InspectInput) (InspectOutput, error) {
	if resolver == nil {
		return InspectOutput{}, &errs.Error{ID: "workbook.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.inspect", Summary: "Workbook inspection is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook resolution before retrying."}
	}
	var validationErr error
	input, validationErr = inspectNormalizeInput(input)
	if validationErr != nil {
		return InspectOutput{}, validationErr
	}
	workbook, err := resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		if _, ok := errors.AsType[*errs.Error](err); ok {
			return InspectOutput{}, err
		}
		if resolution, ok := errors.AsType[*identity.ResolutionError](err); ok {
			switch resolution.Kind {
			case identity.ResolutionAmbiguous:
				return InspectOutput{}, inspectResolveUsageError("workbook.inspect.ambiguous", input, "Workbook selector matched more than one workbook.", err)
			case identity.ResolutionNotFound:
				return InspectOutput{}, inspectResolveUsageError("workbook.inspect.not_found", input, "No workbook matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return InspectOutput{}, inspectResolveUsageError("workbook.inspect.usage", input, "Workbook selection requires a LUID or exact name with --project or --project-id.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact workbook selector, then retry.")
		return InspectOutput{}, &errs.Error{ID: "workbook.inspect.resolve", Kind: errs.KindOperation, Operation: "workbook.inspect", Environment: input.Environment, Site: input.Site, Summary: "Workbook resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if workbook.LUID == "" || workbook.Name == "" || (input.Selector.LUID != "" && workbook.LUID != string(input.Selector.LUID)) || (input.Selector.Name != "" && workbook.Name != input.Selector.Name) || (input.Selector.ProjectLUID != "" && workbook.ProjectLUID != string(input.Selector.ProjectLUID)) {
		return InspectOutput{}, &errs.Error{ID: "workbook.inspect.identity_mismatch", Kind: errs.KindOperation, Operation: "workbook.inspect", Environment: input.Environment, Site: input.Site, Summary: "Workbook adapter returned a mismatched authoritative identity.", Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id."}
	}
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, Workbook: workbook, RequestID: workbook.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "workbook", "pull", "--id", workbook.LUID)}}, nil
}

func inspectResolveUsageError(id string, input InspectInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "workbook.inspect", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidateInspectInput checks an exact selector before authentication.
func ValidateInspectInput(input InspectInput) error {
	_, err := inspectNormalizeInput(input)
	return err
}

func inspectNormalizeInput(input InspectInput) (InspectInput, error) {
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	input.Selector.ProjectLUID = identity.LUID(strings.TrimSpace(string(input.Selector.ProjectLUID)))
	if input.Selector.LUID == "" && (input.Selector.Name == "" || (input.Selector.ProjectPath == "" && input.Selector.ProjectLUID == "")) {
		message := "workbook selection requires a LUID or exact name and project path or project LUID"
		return input, &errs.Error{ID: "workbook.inspect.usage", Kind: errs.KindUsage, Operation: "workbook.inspect", Summary: "Workbook selection requires a LUID or exact name and project path or project LUID.", Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: message}}}
	}
	if input.Selector.ProjectPath != "" && input.Selector.ProjectLUID != "" {
		return input, &errs.Error{ID: "workbook.inspect.usage", Kind: errs.KindUsage, Operation: "workbook.inspect", Summary: "An exact name cannot be combined with both project path and project LUID.", Retryable: new(false), CorrectiveAction: "Provide exactly one of --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: "provide exactly one project path or project LUID"}}}
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "" || input.Selector.ProjectLUID != "") {
		return input, &errs.Error{ID: "workbook.inspect.usage", Kind: errs.KindUsage, Operation: "workbook.inspect", Summary: "A LUID is authoritative and cannot be combined with name or project selectors.", Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: "a LUID is authoritative and cannot be combined with name or project selectors"}}}
	}
	return input, nil
}
