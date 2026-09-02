package delete

import (
	"context"
	"errors"
	"reflect"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct{ Environment, Site, GroupLUID string }
type Group struct {
	LUID   string `json:"luid"`
	Name   string `json:"name"`
	Domain string `json:"domain,omitempty"`
}
type Plan struct {
	Mode             string `json:"mode"`
	Operation        string `json:"operation"`
	Environment      string `json:"environment"`
	Site             string `json:"site"`
	Target           Group  `json:"target"`
	DeletesUsers     bool   `json:"deletes_users"`
	PermissionImpact string `json:"permission_impact"`
}
type Result struct {
	Status           string `json:"status"`
	GroupLUID        string `json:"group_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}
type CompactMutationResult struct {
	Status    string `json:"status"`
	GroupLUID string `json:"group_luid"`
}
type CompactResult struct {
	Plan    Plan                   `json:"plan"`
	Applied bool                   `json:"applied"`
	Result  *CompactMutationResult `json:"result,omitempty"`
	Details string                 `json:"details"`
	Help    []string               `json:"help"`
}
type FullResult struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactMutationResult
	if o.Result != nil {
		result = &CompactMutationResult{Status: o.Result.Status, GroupLUID: o.Result.GroupLUID}
	}
	return CompactResult{Plan: o.Plan, Applied: o.Applied, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Applied: o.Applied, Result: o.Result, Help: o.Help}
}

type Resolver interface {
	ResolveGroup(context.Context, string) (Group, error)
}
type Deleter interface {
	DeleteGroup(context.Context, string) (Result, error)
}
type Action struct {
	resolver Resolver
	deleter  Deleter
}

func New(r Resolver, d Deleter) *Action { return &Action{resolver: r, deleter: d} }
func (a *Action) Execute(ctx context.Context, in Input, apply bool) (Output, error) {
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, errors.New("admin group delete is not configured")
	}
	if in.Environment == "" || in.Site == "" || in.GroupLUID == "" {
		return Output{}, &errs.Error{ID: "admin.group.delete.usage", Kind: errs.KindUsage, Operation: "admin.group.delete", Summary: "admin group delete requires explicit environment, site, and group LUID", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment, site, and group LUID.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin group delete requires explicit environment, site, and group LUID"}}}
	}
	g, err := a.resolver.ResolveGroup(ctx, in.GroupLUID)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.group.delete", Environment: in.Environment, Site: in.Site, Target: g, DeletesUsers: false, PermissionImpact: "unknown"}, Details: "--full", Help: []string{"Add --apply to delete this exact group without deleting users."}}
	if !apply {
		return out, nil
	}
	current, err := a.resolver.ResolveGroup(ctx, in.GroupLUID)
	if err != nil {
		return Output{}, err
	}
	if !reflect.DeepEqual(current, g) {
		return Output{}, errors.New("the group delete target changed after preview")
	}
	result, err := a.deleter.DeleteGroup(ctx, g.LUID)
	if err != nil {
		if result.Status == "unknown" {
			luid := result.GroupLUID
			if luid == "" {
				luid = g.LUID
			}
			return Output{}, &errs.Error{ID: "admin.group.delete.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.group.delete", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The group delete outcome could not be determined safely.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the target site and Tableau request before attempting another group delete.", TableauRequestID: result.TableauRequestID}
		}
		return Output{}, err
	}
	out.Applied = true
	out.Result = &result
	out.Help = []string{"tadx admin group list"}
	return out, nil
}
