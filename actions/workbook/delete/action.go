package delete

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver owns authoritative workbook identity selection.
type Resolver interface {
	ResolveWorkbook(context.Context, identity.Selector) (Workbook, error)
}

// Deleter performs one exact remote workbook deletion.
type Deleter interface {
	DeleteWorkbook(context.Context, string) (Result, error)
}

// Action previews and applies one workbook deletion.
type Action struct {
	resolver Resolver
	deleter  Deleter
}

// New creates a workbook delete action.
func New(resolver Resolver, deleter Deleter) *Action {
	return &Action{resolver: resolver, deleter: deleter}
}

// Execute previews by default and revalidates the authoritative LUID before apply.
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, &errs.Error{ID: "workbook.delete.unconfigured", Kind: errs.KindRuntime, Operation: "workbook.delete", Summary: "Workbook delete is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure workbook delete before retrying."}
	}
	if input.Environment == "" || input.Site == "" {
		return Output{}, usage("environment", "workbook delete requires an explicit resolved environment and site")
	}
	input.Selector.LUID = identity.LUID(strings.TrimSpace(string(input.Selector.LUID)))
	input.Selector.Name = strings.TrimSpace(input.Selector.Name)
	input.Selector.ProjectPath = strings.TrimSpace(input.Selector.ProjectPath)
	if input.Selector.LUID == "" && (input.Selector.Name == "" || input.Selector.ProjectPath == "") {
		return Output{}, selectorUsage("required", "workbook selection requires a LUID or exact name and project path", "Workbook selection requires a LUID or exact name and project path.")
	}
	if input.Selector.LUID != "" && (input.Selector.Name != "" || input.Selector.ProjectPath != "") {
		return Output{}, selectorUsage("conflict", "a LUID is authoritative and cannot be combined with name or project selectors", "A LUID is authoritative and cannot be combined with name or project selectors.")
	}
	target, err := a.resolver.ResolveWorkbook(ctx, input.Selector)
	if err != nil {
		return Output{}, operationError("workbook.delete.resolve", input, "Workbook resolution failed.", "Review the exact workbook selector, then retry.", err, "")
	}
	if target.LUID == "" {
		return Output{}, operationError("workbook.delete.resolve", input, "Workbook resolution failed.", "Review the exact workbook selector, then retry.", errors.New("resolved workbook omitted its authoritative LUID"), "")
	}
	plan := Plan{Mode: "preview", Operation: "workbook.delete", Environment: input.Environment, Site: input.Site, Target: target}
	output := Output{Plan: plan, Help: []string{"Run without --preview to delete this exact workbook."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	current, err := a.resolver.ResolveWorkbook(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return Output{}, operationError("workbook.delete.resolve", input, "Workbook revalidation failed.", "Review a new preview before deleting.", err, target.LUID)
	}
	if current.LUID != target.LUID {
		return Output{}, operationError("workbook.delete.target_changed", input, "The workbook delete target identity changed during revalidation.", "Review a new preview before deleting.", fmt.Errorf("resolved workbook LUID changed from %q to %q", target.LUID, current.LUID), target.LUID)
	}
	result, err := a.deleter.DeleteWorkbook(ctx, target.LUID)
	if err != nil {
		return Output{}, operationError("workbook.delete.failed", input, "Workbook delete failed.", "Review the upstream error before deleting again.", err, target.LUID)
	}
	output.Result = &result
	output.Help = []string{"tadx content workbook list --environment " + input.Environment}
	return output, nil
}

func operationError(id string, input Input, summary, fallback string, cause error, resource string) error {
	retryable, correctiveAction := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "workbook.delete", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(cause)}
}

func usage(field, message string) error {
	return &errs.Error{ID: "workbook.delete.usage", Kind: errs.KindUsage, Operation: "workbook.delete", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the workbook delete input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

func selectorUsage(code, message, summary string) error {
	return &errs.Error{ID: "workbook.delete.usage", Kind: errs.KindUsage, Operation: "workbook.delete", Summary: summary, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name and project path.", Validation: []errs.ValidationDetail{{Field: "selector", Code: code, Message: message}}}
}
