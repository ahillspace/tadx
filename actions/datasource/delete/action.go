package delete

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type Resolver interface {
	ResolveDatasource(context.Context, identity.Selector) (Datasource, error)
}

type Deleter interface {
	DeleteDatasource(context.Context, string) (Result, error)
}

type Action struct {
	resolver Resolver
	deleter  Deleter
}

func New(resolver Resolver, deleter Deleter) *Action {
	return &Action{resolver: resolver, deleter: deleter}
}

func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, &errs.Error{ID: "datasource.delete.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.delete", Summary: "Datasource delete is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure datasource delete before retrying."}
	}
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) {
		return Output{}, usage("environment", "datasource delete requires an explicit resolved environment and site")
	}
	item, err := a.resolver.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		return Output{}, resolveError(input, "Datasource resolution failed.", err)
	}
	output := Output{Plan: Plan{Mode: "preview", Operation: "datasource.delete", Environment: input.Environment, Site: input.Site, Target: item}, Help: []string{"Run without --preview to delete this exact datasource."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.resolver.ResolveDatasource(ctx, input.Selector)
	if err != nil {
		return Output{}, resolveError(input, "Datasource revalidation failed.", err)
	}
	if current != item {
		return Output{}, &errs.Error{ID: "datasource.delete.target_changed", Kind: errs.KindOperation, Operation: "datasource.delete", Resource: item.LUID, Environment: input.Environment, Site: input.Site, Summary: "The datasource delete target changed during revalidation.", Cause: errors.New("datasource delete target changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before deleting."}
	}
	result, err := a.deleter.DeleteDatasource(ctx, item.LUID)
	if err != nil {
		retryable, action := errs.CompleteRetryAdvice(err, "Review the upstream dependency, permission, or missing-resource error before deleting again.")
		return Output{}, &errs.Error{ID: "datasource.delete.failed", Kind: errs.KindOperation, Operation: "datasource.delete", Resource: item.LUID, Environment: input.Environment, Site: input.Site, Summary: "Datasource delete failed.", Cause: err, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "datasource", "list")}
	return output, nil
}

func resolveError(input Input, summary string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, "Review the exact datasource selector, then retry.")
	return &errs.Error{ID: "datasource.delete.resolve", Kind: errs.KindOperation, Operation: "datasource.delete", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

func usage(field, message string) error {
	return &errs.Error{ID: "datasource.delete.usage", Kind: errs.KindUsage, Operation: "datasource.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the datasource delete input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
