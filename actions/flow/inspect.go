package flow

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Inspect resolves one exact flow.
func Inspect(ctx context.Context, resolver Resolver, input InspectInput) (InspectOutput, error) {
	if resolver == nil {
		return InspectOutput{}, &errs.Error{ID: "flow.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "flow.inspect", Summary: "Flow inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the flow resolver before retrying."}
	}
	var validationErr error
	input, validationErr = inspectNormalizeInput(input)
	if validationErr != nil {
		return InspectOutput{}, validationErr
	}
	flow, err := resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		if _, ok := errors.AsType[*errs.Error](err); ok {
			return InspectOutput{}, err
		}
		if resolution, ok := errors.AsType[*identity.ResolutionError](err); ok {
			switch resolution.Kind {
			case identity.ResolutionAmbiguous:
				return InspectOutput{}, inspectResolveUsageError("flow.inspect.ambiguous", input, "Flow selector matched more than one flow.", err)
			case identity.ResolutionNotFound:
				return InspectOutput{}, inspectResolveUsageError("flow.inspect.not_found", input, "No flow matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return InspectOutput{}, inspectResolveUsageError("flow.inspect.usage", input, "Flow selection requires a LUID or exact name with --project or --project-id.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return InspectOutput{}, &errs.Error{ID: "flow.inspect.resolve", Kind: errs.KindOperation, Operation: "flow.inspect", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if flow.LUID == "" || flow.Name == "" || (input.Selector.LUID != "" && flow.LUID != string(input.Selector.LUID)) || (input.Selector.Name != "" && flow.Name != input.Selector.Name) || (input.Selector.ProjectLUID != "" && flow.ProjectLUID != string(input.Selector.ProjectLUID)) {
		return InspectOutput{}, &errs.Error{ID: "flow.inspect.identity_mismatch", Kind: errs.KindOperation, Operation: "flow.inspect", Environment: input.Environment, Site: input.Site, Summary: "Flow adapter returned a mismatched authoritative identity.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id."}
	}
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, Flow: flow, RequestID: flow.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "flow", "pull", "--id", flow.LUID)}}, nil
}

func inspectResolveUsageError(id string, input InspectInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "flow.inspect", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidateInspectInput checks an exact selector before authentication.
func ValidateInspectInput(input InspectInput) error {
	_, err := inspectNormalizeInput(input)
	return err
}

func inspectNormalizeInput(input InspectInput) (InspectInput, error) {
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.Name = strings.TrimSpace(input.Selector.Name)
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	input.Selector.ProjectLUID = identity.LUID(strings.TrimSpace(string(input.Selector.ProjectLUID)))
	if input.Selector.LUID == "" && (input.Selector.Name == "" || (input.Selector.ProjectPath == "" && input.Selector.ProjectLUID == "")) {
		return input, &errs.Error{ID: "flow.inspect.usage", Kind: errs.KindUsage, Operation: "flow.inspect", Summary: "Flow selection requires a LUID or exact name and project path or project LUID.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "flow selection requires a LUID or exact name and project path or project LUID"}}}
	}
	if input.Selector.ProjectPath != "" && input.Selector.ProjectLUID != "" {
		return input, &errs.Error{ID: "flow.inspect.usage", Kind: errs.KindUsage, Operation: "flow.inspect", Summary: "An exact name cannot be combined with both project path and project LUID.", Retryable: errs.Bool(false), CorrectiveAction: "Provide exactly one of --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: "provide exactly one project path or project LUID"}}}
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "" || input.Selector.ProjectLUID != "") {
		return input, &errs.Error{ID: "flow.inspect.usage", Kind: errs.KindUsage, Operation: "flow.inspect", Summary: "A LUID is authoritative and cannot be combined with name or project selectors.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: "a LUID is authoritative and cannot be combined with name or project selectors"}}}
	}
	return input, nil
}
