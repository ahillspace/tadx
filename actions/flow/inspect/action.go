package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

// Resolver is the action-owned flow seam.
type Resolver interface {
	ResolveFlow(context.Context, identity.Selector) (Flow, error)
}
type Action struct{ resolver Resolver }

func New(resolver Resolver) *Action { return &Action{resolver: resolver} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, &errs.Error{ID: "flow.inspect.unconfigured", Kind: errs.KindRuntime, Operation: "flow.inspect", Summary: "Flow inspection is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure the flow resolver before retrying."}
	}
	var validationErr error
	input, validationErr = normalizeInput(input)
	if validationErr != nil {
		return Output{}, validationErr
	}
	flow, err := a.resolver.ResolveFlow(ctx, input.Selector)
	if err != nil {
		if _, ok := errors.AsType[*errs.Error](err); ok {
			return Output{}, err
		}
		if resolution, ok := errors.AsType[*identity.ResolutionError](err); ok {
			switch resolution.Kind {
			case identity.ResolutionAmbiguous:
				return Output{}, resolveUsageError("flow.inspect.ambiguous", input, "Flow selector matched more than one flow.", err)
			case identity.ResolutionNotFound:
				return Output{}, resolveUsageError("flow.inspect.not_found", input, "No flow matched the selector.", err)
			case identity.ResolutionInvalidSelector:
				return Output{}, resolveUsageError("flow.inspect.usage", input, "Flow selection requires a LUID or exact name with --project or --project-id.", err)
			}
		}
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return Output{}, &errs.Error{ID: "flow.inspect.resolve", Kind: errs.KindOperation, Operation: "flow.inspect", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if flow.LUID == "" || flow.Name == "" || (input.Selector.LUID != "" && flow.LUID != string(input.Selector.LUID)) || (input.Selector.Name != "" && flow.Name != input.Selector.Name) || (input.Selector.ProjectLUID != "" && flow.ProjectLUID != string(input.Selector.ProjectLUID)) {
		return Output{}, &errs.Error{ID: "flow.inspect.identity_mismatch", Kind: errs.KindOperation, Operation: "flow.inspect", Environment: input.Environment, Site: input.Site, Summary: "Flow adapter returned a mismatched authoritative identity.", Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id."}
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, Flow: flow, RequestID: flow.RequestID, Help: []string{commandhint.Environment(input.Environment, "content", "flow", "pull", "--id", flow.LUID)}}, nil
}

func resolveUsageError(id string, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: errs.KindUsage, Operation: "flow.inspect", Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide a LUID or an exact name with --project or --project-id.", TableauRequestID: errs.TableauRequestID(cause)}
}
