package flow

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type MoveResolver interface {
	Resolver
	ProjectResolver
}
type Mover interface {
	MoveFlow(context.Context, string, string) (MoveResult, error)
}

// Move previews or moves one exact flow.
func Move(ctx context.Context, resolver MoveResolver, mover Mover, input MoveInput, preview bool) (MoveOutput, error) {
	if err := ValidateMoveInput(input); err != nil {
		return MoveOutput{}, err
	}
	ctx = beginProjectResolution(ctx, resolver)
	if resolver == nil || mover == nil {
		return MoveOutput{}, &errs.Error{ID: "flow.move.unconfigured", Kind: errs.KindRuntime, Operation: "flow.move", Summary: "Flow move is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure flow move before retrying."}
	}
	if input.Environment == "" || (input.Site == "" && !input.TargetResolved) {
		return MoveOutput{}, moveUsage("environment", "flow move requires an explicit resolved environment and site")
	}
	flow, err := resolver.ResolveFlow(ctx, input.FlowSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return MoveOutput{}, &errs.Error{ID: "flow.move.resolve", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Flow resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	project, err := resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project, then retry.")
		return MoveOutput{}, &errs.Error{ID: "flow.move.project", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Destination project resolution failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	plan := MovePlan{Mode: "preview", Operation: "flow.move", Environment: input.Environment, Site: input.Site, Source: contentIdentity(flow), Destination: project, NoOp: flow.ProjectLUID == project.LUID}
	output := MoveOutput{Plan: plan, Help: []string{"Run without --preview to move this exact flow."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	ctx = beginProjectResolution(ctx, resolver)
	current, err := resolver.ResolveFlow(ctx, input.FlowSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact flow selector, then retry.")
		return MoveOutput{}, &errs.Error{ID: "flow.move.resolve", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Flow revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	destination, err := resolver.ResolveProject(ctx, input.ProjectSelector)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the exact destination project, then retry.")
		return MoveOutput{}, &errs.Error{ID: "flow.move.project", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "Destination project revalidation failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	if contentIdentity(current) != contentIdentity(flow) || destination != project {
		return MoveOutput{}, &errs.Error{ID: "flow.move.target_changed", Kind: errs.KindOperation, Operation: "flow.move", Environment: input.Environment, Site: input.Site, Summary: "The flow move source or destination changed during revalidation.", Cause: errors.New("flow move source or destination changed during revalidation"), Retryable: errs.Bool(false), CorrectiveAction: "Review a new preview before moving."}
	}
	if plan.NoOp {
		output.Result = &MoveResult{Status: "unchanged", FlowLUID: flow.LUID, ProjectLUID: project.LUID}
		return output, nil
	}
	result, err := mover.MoveFlow(ctx, flow.LUID, project.LUID)
	if err != nil {
		retryable, correctiveAction := errs.CompleteRetryAdvice(err, "Review the upstream error before moving again.")
		return MoveOutput{}, &errs.Error{ID: "flow.move.failed", Kind: errs.KindOperation, Operation: "flow.move", Resource: flow.LUID, Environment: input.Environment, Site: input.Site, Summary: "Flow move failed.", Cause: err, Retryable: retryable, CorrectiveAction: correctiveAction, TableauRequestID: errs.TableauRequestID(err)}
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "content", "flow", "inspect", "--id", flow.LUID)}
	return output, nil
}

func moveUsage(field, message string) error {
	return &errs.Error{ID: "flow.move.usage", Kind: errs.KindUsage, Operation: "flow.move", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the flow move input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}

// ValidateMoveInput checks caller-controlled arguments before dependency setup.
func ValidateMoveInput(input MoveInput) error {
	if strings.TrimSpace(input.Environment) == "" {
		return moveUsage("environment", "flow move requires an explicit environment")
	}
	selector := input.FlowSelector
	if selector.LUID == "" && (strings.TrimSpace(selector.Name) == "" || strings.TrimSpace(selector.ProjectPath) == "") {
		return moveUsage("selector", "flow move requires a LUID or exact name and project path")
	}
	if selector.LUID != "" && (selector.Name != "" || selector.ProjectPath != "") {
		return moveUsage("selector", "a LUID cannot be combined with name or project selectors")
	}
	project := input.ProjectSelector
	if (project.LUID == "" && strings.TrimSpace(project.ProjectPath) == "") || (project.LUID != "" && project.ProjectPath != "") {
		return moveUsage("project", "flow move requires one exact destination project selector")
	}
	return nil
}
