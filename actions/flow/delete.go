package flow

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type Deleter interface {
	DeleteFlow(context.Context, string) (DeleteResult, error)
}

// Delete previews or deletes one exact flow.
func Delete(ctx context.Context, resolver Resolver, deleter Deleter, input DeleteInput, preview bool) (DeleteOutput, error) {
	if err := ValidateDeleteInput(input); err != nil {
		return DeleteOutput{}, err
	}
	if resolver == nil || deleter == nil {
		return DeleteOutput{}, &errs.Error{ID: "flow.delete.unconfigured", Kind: errs.KindRuntime, Operation: "flow.delete", Summary: "Flow delete is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow delete before retrying."}
	}
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return DeleteOutput{}, deleteUsage("environment", "flow delete requires an explicit resolved environment and site")
	}
	flow, err := resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return DeleteOutput{}, &errs.Error{ID: "flow.delete.resolve", Kind: errs.KindOperation, Operation: "flow.delete", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	plan := DeletePlan{Mode: "preview", Operation: "flow.delete", Environment: input.Environment, Site: input.Site, Target: contentIdentity(flow)}
	output := DeleteOutput{Plan: plan, Help: []string{"Run without --preview to delete this exact flow."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return DeleteOutput{}, &errs.Error{ID: "flow.delete.resolve", Kind: errs.KindOperation, Operation: "flow.delete", Environment: input.Environment, Site: input.Site, Summary: "Flow revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if contentIdentity(current) != contentIdentity(flow) {
		return DeleteOutput{}, &errs.Error{ID: "flow.delete.target_changed", Kind: errs.KindOperation, Operation: "flow.delete", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "The flow delete target changed during revalidation.", Cause: errors.New("flow delete target changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before deleting."}
	}
	result, err := deleter.DeleteFlow(ctx, flow.LUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the upstream error before deleting again.")
		return DeleteOutput{}, &errs.Error{ID: "flow.delete.failed", Kind: errs.KindOperation, Operation: "flow.delete", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow delete failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "flow", "list")}
	return output, nil
}

func deleteUsage(field, message string) error {
	return &errs.Error{ID: "flow.delete.usage", Kind: errs.KindUsage, Operation: "flow.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow delete input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

// ValidateDeleteInput checks caller-controlled arguments before dependency setup.
func ValidateDeleteInput(input DeleteInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return errs.New(errs.KindUsage, "flow delete requires an explicit environment")
	}
	selector := input.Selector
	if strings.TrimSpace(string(selector.LUID)) == "" && (strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "") {
		return errs.New(errs.KindUsage, "flow selection requires a LUID or exact name and project path")
	}
	if selector.LUID != "" && (selector.Name != "" || selector.ProjectPath != "") {
		return errs.New(errs.KindUsage, "a LUID cannot be combined with name or project selectors")
	}
	return nil
}
