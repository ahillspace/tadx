package datasource

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"strings"
)

type UpdateResolver interface {
	Resolver
	CollisionReader
}
type Updater interface {
	UpdateDatasource(context.Context, UpdateRequest) (UpdateResult, error)
}

func Update(ctx context.Context, resolver UpdateResolver, updater Updater, in UpdateInput, preview bool) (UpdateOutput, error) {
	if resolver == nil || updater == nil {
		return UpdateOutput{}, &errs.Error{ID: "datasource.update.unconfigured", Kind: errs.KindRuntime, Operation: "datasource.update", Summary: "Datasource update is not configured.", Retryable: new(false), CorrectiveAction: "Configure datasource update before retrying."}
	}
	if err := updateValidate(in); err != nil {
		return UpdateOutput{}, err
	}
	target, err := resolver.ResolveDatasource(ctx, in.Selector)
	if err != nil {
		return UpdateOutput{}, updateOperationError("datasource.update.resolve", in, "", "Datasource resolution failed.", "Review the exact datasource selector, then retry.", err)
	}
	request, changes := updateChangedRequest(target, in)
	if err = updateCollision(ctx, resolver, in, target, request.Name); err != nil {
		return UpdateOutput{}, err
	}
	out := UpdateOutput{Plan: UpdatePlan{Mode: "preview", Operation: "datasource.update", Environment: in.Environment, Site: in.Site, Target: updateIdentity(target), Changes: changes, NoOp: len(changes) == 0}, Help: []string{"Run without --preview to update this exact datasource."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := resolver.ResolveDatasource(ctx, identity.Selector{LUID: identity.LUID(target.LUID)})
	if err != nil {
		return UpdateOutput{}, updateOperationError("datasource.update.resolve", in, target.LUID, "Datasource revalidation failed.", "Review a new preview before updating.", err)
	}
	if current.Name != target.Name || current.ProjectLUID != target.ProjectLUID || current.OwnerLUID != target.OwnerLUID {
		return UpdateOutput{}, updateOperationError("datasource.update.target_changed", in, target.LUID, "The datasource changed during revalidation.", "Review a new preview before updating.", errors.New("datasource identity changed during revalidation"))
	}
	request, changes = updateChangedRequest(current, in)
	out.Plan.Target, out.Plan.Changes, out.Plan.NoOp = updateIdentity(current), changes, len(changes) == 0
	if err = updateCollision(ctx, resolver, in, current, request.Name); err != nil {
		return UpdateOutput{}, err
	}
	if out.Plan.NoOp {
		out.Result = &UpdateResult{Status: "unchanged", DatasourceLUID: current.LUID, DatasourceName: current.Name, ProjectLUID: current.ProjectLUID, OwnerLUID: current.OwnerLUID}
		return out, nil
	}
	result, err := updater.UpdateDatasource(ctx, request)
	if err != nil {
		return UpdateOutput{}, updateOperationError("datasource.update.failed", in, current.LUID, "Datasource update failed.", "Inspect the exact datasource before retrying: "+commandhint.Environment(in.Environment, "content", "datasource", "inspect", "--id", current.LUID), err)
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "content", "datasource", "inspect", "--id", current.LUID)}
	return out, nil
}
func updateChangedRequest(target Record, in UpdateInput) (UpdateRequest, []UpdateChange) {
	r := UpdateRequest{LUID: target.LUID}
	changes := []UpdateChange{}
	if in.Name != nil && *in.Name != target.Name {
		r.Name = in.Name
		changes = append(changes, UpdateChange{Field: "name", Before: target.Name, After: *in.Name})
	}
	if in.OwnerLUID != nil && *in.OwnerLUID != target.OwnerLUID {
		r.OwnerLUID = in.OwnerLUID
		changes = append(changes, UpdateChange{Field: "owner_luid", Before: target.OwnerLUID, After: *in.OwnerLUID})
	}
	return r, changes
}
func updateCollision(ctx context.Context, resolver UpdateResolver, in UpdateInput, target Record, name *string) error {
	if name == nil {
		return nil
	}
	matches, err := resolver.FindDatasources(ctx, *name, target.ProjectLUID)
	if err != nil {
		return updateOperationError("datasource.update.collision", in, target.LUID, "Datasource collision check failed.", "Review the target project before updating.", err)
	}
	for _, match := range matches {
		if match.LUID != target.LUID {
			return updateOperationError("datasource.update.collision", in, target.LUID, "The project already contains this datasource name.", "Choose another datasource name.", errors.New("exact datasource name collision in target project"))
		}
	}
	return nil
}
func updateValidate(in UpdateInput) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.Site) == "" && !in.TargetResolved) {
		return updateUsage("environment", "datasource update requires an explicit resolved environment and site")
	}
	return ValidateUpdateInput(in)
}
func updateUsage(field, message string) error {
	return &errs.Error{ID: "datasource.update.usage", Kind: errs.KindUsage, Operation: "datasource.update", Summary: message, Retryable: new(false), CorrectiveAction: "Correct the datasource update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "required", Message: message}}}
}
func updateOperationError(id string, in UpdateInput, resource, summary, fallback string, cause error) error {
	retryable, action := errs.CompleteRetryAdvice(cause, fallback)
	return &errs.Error{ID: id, Kind: errs.KindOperation, Operation: "datasource.update", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: retryable, CorrectiveAction: action, TableauRequestID: errs.TableauRequestID(cause)}
}

// ValidateUpdateInput checks caller-controlled arguments before local or remote setup.
func ValidateUpdateInput(in UpdateInput) error {
	if strings.TrimSpace(in.Environment) == "" {
		return updateUsage("environment", "datasource update requires an explicit environment")
	}
	if in.Selector.LUID == "" && (strings.TrimSpace(in.Selector.Name) == "" || strings.TrimSpace(in.Selector.ProjectPath) == "") {
		return updateUsage("selector", "datasource update requires a LUID or exact name and project path")
	}
	if in.Selector.LUID != "" && (strings.TrimSpace(in.Selector.Name) != "" || strings.TrimSpace(in.Selector.ProjectPath) != "") {
		return updateUsage("selector", "a datasource LUID cannot be combined with name or project path")
	}
	if in.Name == nil && in.OwnerLUID == nil {
		return updateUsage("changes", "datasource update requires a name or owner LUID")
	}
	if in.Name != nil && strings.TrimSpace(*in.Name) == "" {
		return updateUsage("name", "datasource update name cannot be empty")
	}
	if in.OwnerLUID != nil {
		if err := identity.ValidateLUIDShape("owner", *in.OwnerLUID); err != nil {
			return updateUsage("owner_id", err.Error())
		}
	}
	return nil
}
