package workbook

import (
	"context"
	"errors"
	"fmt"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Deleter performs one exact remote workbook deletion.
type Deleter interface {
	DeleteWorkbook(context.Context, string) (DeleteResult, error)
}

// Delete previews or deletes a workbook after revalidating its authoritative LUID.
func Delete(ctx context.Context, resolver Resolver, deleter Deleter, input DeleteInput, preview bool) (DeleteOutput, error) {
	if resolver == nil || deleter == nil {
		return DeleteOutput{}, &errs.Error{ID: "workbook.delete.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.delete", Summary: "Workbook delete is not configured.", Retryable: new(false), CorrectiveAction: "Configure workbook delete before retrying."}
	}
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return DeleteOutput{}, deleteUsage("environment", "workbook delete requires an explicit resolved environment and site")
	}
	var validationErr error
	input, validationErr = deleteNormalizeInput(input)
	if validationErr != nil {
		return DeleteOutput{}, validationErr
	}
	target, err := resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		return DeleteOutput{}, deleteOperationError("workbook.delete.resolve", input, "Workbook resolution failed.", "Review the exact workbook selector, then retry.", err, "")
	}
	if target.LUID == "" {
		return DeleteOutput{}, deleteOperationError("workbook.delete.resolve", input, "Workbook resolution failed.", "Review the exact workbook selector, then retry.", errors.New("resolved workbook omitted its authoritative LUID"), "")
	}
	plan := DeletePlan{Mode: "preview", Operation: "workbook.delete", Environment: input.Environment, Site: input.Site, Target: deleteIdentity(target)}
	output := DeleteOutput{Plan: plan, Help: []string{"Run without --preview to delete this exact workbook."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := resolver.ResolveWorkbook(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return DeleteOutput{}, deleteOperationError("workbook.delete.resolve", input, "Workbook revalidation failed.", "Review a new preview before deleting.", err, target.LUID)
	}
	if current.LUID != target.LUID {
		return DeleteOutput{}, deleteOperationError("workbook.delete.target_changed", input, "The workbook delete target identity changed during revalidation.", "Review a new preview before deleting.", fmt.Errorf("resolved workbook LUID changed from %q to %q", target.LUID, current.LUID), target.LUID)
	}
	result, err := deleter.DeleteWorkbook(ctx, target.LUID)
	if err != nil {
		return DeleteOutput{}, deleteOperationError("workbook.delete.failed", input, "Workbook delete failed.", "Review the upstream error before deleting again.", err, target.LUID)
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "workbook", "list")}
	return output, nil
}

func deleteOperationError(id string, input DeleteInput, summary, fallback string, cause error, resource string) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "workbook.delete", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(cause)}
}

func deleteUsage(field, message string) error {
	return &errs.Error{ID: "workbook.delete.usage", Kind: errs.KindUsage, Operation: "workbook.delete", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the workbook delete input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func deleteSelectorUsage(code, message, summary string) error {
	return &errs.Error{ID: "workbook.delete.usage", Kind: errs.KindUsage, Operation: "workbook.delete", Summary: summary, Retryable: new(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: code, Message: message}}}
}

// ValidateDeleteInput checks an exact selector before authentication.
func ValidateDeleteInput(input DeleteInput) error { _, err := deleteNormalizeInput(input); return err }

func deleteNormalizeInput(input DeleteInput) (DeleteInput, error) {
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return input, deleteSelectorUsage("required", "workbook selection requires a LUID or exact name and project path", "Workbook selection requires a LUID or exact name and project path.")
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		return input, deleteSelectorUsage("conflict", "a LUID is authoritative and cannot be combined with name or project selectors", "A LUID is authoritative and cannot be combined with name or project selectors.")
	}
	return input, nil
}
