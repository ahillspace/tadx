package datasource

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

type Deleter interface {
	DeleteDatasource(context.Context, string) (DeleteResult, error)
}

func Delete(ctx context.Context, resolver Resolver, deleter Deleter, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if resolver == nil || deleter == nil {
		return DeleteOutput{}, &errs.Error{ID: "datasource.delete.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.delete", Summary: "Datasource delete is not configured.", Retryable: new(false), CorrectiveAction: "Configure datasource delete before retrying."}
	}
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return DeleteOutput{}, deleteUsage("environment", "datasource delete requires an explicit resolved environment and site")
	}
	item, err := resolver.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		return DeleteOutput{}, deleteResolveError(input, "Datasource resolution failed.", err)
	}
	output := DeleteOutput{Plan: DeletePlan{Mode: "preview", Operation: "datasource.delete", Environment: input.Environment, Site: input.Site, Target: deleteIdentity(item)}, Help: []string{"Run without --preview to delete this exact datasource."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := resolver.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		return DeleteOutput{}, deleteResolveError(input, "Datasource revalidation failed.", err)
	}
	if deleteIdentity(current) != deleteIdentity(item) {
		return DeleteOutput{}, &errs.Error{ID: "datasource.delete.target_changed", Kind: errs.KindOperation, Operation: "datasource.delete", Resource: item.LUID, Environment: input.Environment, Site: input.Site, Summary: "The datasource delete target changed during revalidation.", Cause: errors.New("datasource delete target changed during revalidation"), Retryable: new(false), CorrectiveAction: "Review a new preview before deleting."}
	}
	result, err := deleter.DeleteDatasource(ctx, item.LUID)
	if err != nil {
		retryable, action := errs.CompleteRetryAdvice(err, "Review the upstream dependency, permission, or missing-resource error before deleting again.")
		return DeleteOutput{}, &errs.Error{ID: "datasource.delete.failed", Kind: errs.KindOperation, Operation: "datasource.delete", Resource: item.LUID, Environment: input.Environment, Site: input.Site, Summary: "Datasource delete failed.", Cause: err, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "datasource", "list")}
	return output, nil
}

func deleteResolveError(input DeleteInput, summary string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, "Review the exact datasource selector, then retry.")
	return &errs.Error{ID: "datasource.delete.resolve", Kind: errs.KindOperation, Operation: "datasource.delete", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

func deleteUsage(field, message string) error {
	return &errs.Error{ID: "datasource.delete.usage", Kind: errs.KindUsage, Operation: "datasource.delete", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the datasource delete input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

// ValidateDeleteInput checks caller-controlled arguments before dependency setup.
func ValidateDeleteInput(input DeleteInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return errs.New(errs.KindUsage, "datasource delete requires an explicit environment")
	}
	selector := input.Selector
	if strings.TrimSpace(string(selector.LUID)) == "" && (strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "") {
		return errs.New(errs.KindUsage, "datasource selection requires a LUID or exact name and project path")
	}
	if selector.LUID != "" && (selector.Name != "" || selector.ProjectPath != "") {
		return errs.New(errs.KindUsage, "a LUID cannot be combined with name or project selectors")
	}
	return nil
}
