package delete

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// Action deletes one authoritative Pulse definition.
type Action struct {
	reader  Reader
	deleter Deleter
}

// New creates a Pulse definition delete action.
func New(reader Reader, deleter Deleter) *Action { return &Action{reader: reader, deleter: deleter} }

// Execute reads and revalidates the exact target before deletion.
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.reader == nil || a.deleter == nil {
		return Output{}, failure("unconfigured", errs.KindRuntime, input, "Pulse definition delete is not configured.", nil)
	}
	input.LUID = strings.TrimSpace(input.LUID)
	if strings.TrimSpace(input.Environment) == "" || (strings.TrimSpace(input.Site) == "" && !input.TargetResolved) || input.LUID == "" {
		return Output{}, failure("usage", errs.KindUsage, input, "Pulse definition delete requires an environment, site, and exact LUID.", nil)
	}
	target, err := a.reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		return Output{}, failure("resolve", errs.KindOperation, input, "Pulse definition resolution failed.", err)
	}
	if target.LUID != input.LUID {
		return Output{}, failure("identity", errs.KindOperation, input, "Tableau returned a different Pulse definition identity.", errors.New("exact target LUID mismatch"))
	}
	mode := "execute"
	if input.Preview {
		mode = "preview"
	}
	output := Output{
		Plan:     Plan{Mode: mode, Operation: "pulse.definition.delete", Environment: input.Environment, Site: input.Site, Target: target},
		Warnings: []string{"Tableau determines dependency and cascade effects. Dependent resources are not enumerated."},
		Help:     []string{"tadx pulse definition list --environment " + input.Environment},
	}
	if input.Preview {
		output.Help = []string{"Remove --preview to delete this exact Pulse definition."}
		return output, nil
	}
	current, err := a.reader.GetDefinition(ctx, input.LUID)
	if err != nil {
		return Output{}, failure("revalidate", errs.KindOperation, input, "Pulse definition revalidation failed.", err)
	}
	if current.LUID != target.LUID {
		return Output{}, failure("target_changed", errs.KindOperation, input, "The Pulse definition identity changed before deletion.", errors.New("exact target LUID changed"))
	}
	result, err := a.deleter.DeleteDefinition(ctx, target.LUID)
	if err != nil {
		return Output{}, failure("failed", errs.KindOperation, input, "Pulse definition delete failed.", err)
	}
	output.Result = &result
	return output, nil
}

func failure(suffix string, kind errs.Kind, input Input, summary string, cause error) error {
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Inspect the remote delete outcome before retrying.")
	if kind == errs.KindUsage {
		retryable = errs.Bool(false)
		corrective = "Provide an environment, site, and exact Pulse definition LUID."
	}
	return &errs.Error{ID: "pulse.definition.delete." + suffix, Kind: kind, Operation: "pulse.definition.delete", Resource: input.LUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause)}
}
