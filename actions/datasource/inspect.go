package datasource

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

// Resolver is the action-owned exact datasource seam.

// Inspect resolves one authoritative published datasource.
func Inspect(ctx context.Context, resolver Resolver, input InspectInput) (InspectOutput, error) {
	if resolver == nil {
		return InspectOutput{}, &errs.Error{ID: "datasource.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.inspect", Summary: "Datasource inspection is not configured.", Retryable: new(false), CorrectiveAction: "Configure the datasource resolver before retrying."}
	}
	var validationErr error
	input, validationErr = inspectNormalizeInput(input)
	if validationErr != nil {
		return InspectOutput{}, validationErr
	}
	item, err := resolver.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		var structured *errs.Error
		if errors.As(err, &structured) {
			return InspectOutput{}, err
		}
		var resolution *identity.ResolutionError
		if errors.As(err, &resolution) {
			switch resolution.Kind {
			case identity.ResolutionAmbiguous:
				return InspectOutput{}, inspectResolveUsageError("datasource.inspect.ambiguous", input, "Datasource selector matched more than one datasource.", err)
			case identity.ResolutionNotFound:
				return InspectOutput{}, inspectResolveUsageError("datasource.inspect.not_found", input, "No datasource matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return InspectOutput{}, inspectResolveUsageError("datasource.inspect.usage", input, "Datasource selection requires a LUID or exact name with --project or --project-id.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact datasource selector, then retry.")
		return InspectOutput{}, &errs.Error{ID: "datasource.inspect.resolve", Kind: errs.KindOperation, Operation: "datasource.inspect", Environment: input.Environment, Site: input.Site, Summary: "Datasource resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if item.LUID == "" || item.Name == "" || (input.Selector.LUID != "" && item.LUID != string(input.Selector.LUID)) || (input.Selector.Name != "" && item.Name != input.Selector.Name) || (input.Selector.ProjectLUID != "" && item.ProjectLUID != string(input.Selector.ProjectLUID)) {
		return InspectOutput{}, &errs.Error{ID: "datasource.inspect.identity_mismatch", Kind: errs.KindOperation, Operation: "datasource.inspect", Environment: input.Environment, Site: input.Site, Summary: "Datasource adapter returned a mismatched authoritative identity.", Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id."}
	}
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, Datasource: inspectRecord(item), RequestID: item.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "datasource", "schema", "--id", item.LUID)}}, nil
}

func inspectResolveUsageError(id string, input InspectInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "datasource.inspect", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", TableauRequestID: errs.TableauRequestID(cause)}
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
		message := "datasource selection requires a LUID or exact name and project path or project LUID"
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: "Datasource selection requires a LUID or exact name and project path or project LUID.", Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: message}}}
	}
	if input.Selector.ProjectPath != "" && input.Selector.ProjectLUID != "" {
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: "An exact name cannot be combined with both project path and project LUID.", Retryable: new(false), CorrectiveAction: "Provide exactly one of --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: "provide exactly one project path or project LUID"}}}
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "" || input.Selector.ProjectLUID != "") {
		message := "a LUID is authoritative and cannot be combined with name or project selectors"
		return input, &errs.Error{ID: "datasource.inspect.usage", Kind: errs.KindUsage, Operation: "datasource.inspect", Summary: "A LUID is authoritative and cannot be combined with name or project selectors.", Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "conflict", Message: message}}}
	}
	return input, nil
}
