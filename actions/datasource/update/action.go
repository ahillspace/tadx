package update

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

type Resolver interface {
	ResolveDatasource(context.Context, identity.Selector) (Datasource, error)
	FindDatasources(context.Context, string, string) ([]Datasource, error)
}
type Updater interface {
	UpdateDatasource(context.Context, Request) (Result, error)
}
type Action struct {
	resolver Resolver
	updater  Updater
}

func New(r Resolver, u Updater) *Action { return &Action{r, u} }
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.updater == nil {
		return Output{}, &errs.Error{ID: "datasource.update.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.update", Summary: "Datasource update is not configured.", Retryable: errs.Bool(false), CorrectiveAction: "Configure datasource update before retrying."}
	}
	if err := validate(in); err != nil {
		return Output{}, err
	}
	target, err := a.resolver.ResolveDatasource(ctx, in.Selector)
	if err != nil {
		return Output{}, operationError("datasource.update.resolve", in, "", "Datasource resolution failed.", "Review the exact datasource selector, then retry.", err)
	}
	request, changes := changedRequest(target, in)
	if err = a.collision(ctx, in, target, request.Name); err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "datasource.update", Environment: in.Environment, Site: in.Site, Target: target, Changes: changes, NoOp: len(changes) == 0}, Help: []string{"Run without --preview to update this exact datasource."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.resolver.ResolveDatasource(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return Output{}, operationError("datasource.update.resolve", in, target.LUID, "Datasource revalidation failed.", "Review a new preview before updating.", err)
	}
	if current.Name != target.Name || current.ProjectLUID != target.ProjectLUID || current.OwnerLUID != target.OwnerLUID {
		return Output{}, operationError("datasource.update.target_changed", in, target.LUID, "The datasource changed during revalidation.", "Review a new preview before updating.", errors.New("datasource identity changed during revalidation"))
	}
	request, changes = changedRequest(current, in)
	out.Plan.Target, out.Plan.Changes, out.Plan.NoOp = current, changes, len(changes) == 0
	if err = a.collision(ctx, in, current, request.Name); err != nil {
		return Output{}, err
	}
	if out.Plan.NoOp {
		out.Result = &Result{Status: "unchanged", DatasourceLUID: current.LUID, DatasourceName: current.Name, ProjectLUID: current.ProjectLUID, OwnerLUID: current.OwnerLUID}
		return out, nil
	}
	result, err := a.updater.UpdateDatasource(ctx, request)
	if err != nil {
		return Output{}, operationError("datasource.update.failed", in, current.LUID, "Datasource update failed.", "Inspect the exact datasource before retrying: "+commandhint.Environment(in.Environment, "content", "datasource", "inspect", "--id", current.LUID), err)
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "inspect", "--id", current.LUID)}
	return out, nil
}
func changedRequest(target Datasource, in Input) (Request, []Change) {
	r := Request{LUID: target.LUID}
	changes := []Change{}
	if in.Name != nil && *in.Name != target.Name {
		r.Name = in.Name
		changes = append(changes, Change{Field: "name", Before: target.Name, After: *in.Name})
	}
	if in.OwnerLUID != nil && *in.OwnerLUID != target.OwnerLUID {
		r.OwnerLUID = in.OwnerLUID
		changes = append(changes, Change{Field: "owner_luid", Before: target.OwnerLUID, After: *in.OwnerLUID})
	}
	return r, changes
}
func (a *Action) collision(ctx context.Context, in Input, target Datasource, name *string) error {
	if name == nil {
		return nil
	}
	matches, err := a.resolver.FindDatasources(ctx, *name, target.ProjectLUID)
	if err != nil {
		return operationError("datasource.update.collision", in, target.LUID, "Datasource collision check failed.", "Review the target project before updating.", err)
	}
	for _, match := range matches {
		if match.LUID != target.LUID {
			return operationError("datasource.update.collision", in, target.LUID, "The project already contains this datasource name.", "Choose another datasource name.", errors.New("exact datasource name collision in target project"))
		}
	}
	return nil
}
func validate(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return usage("environment", "datasource update requires an explicit resolved environment and site")
	}
	return ValidateInput(in)
}
func usage(field, message string) error {
	return &errs.Error{ID: "datasource.update.usage", Kind: errs.KindUsage, Operation: "datasource.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the datasource update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func operationError(id string, in Input, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.update", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}
