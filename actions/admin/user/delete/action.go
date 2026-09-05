package delete

import (
	"context"
	"errors"
	"reflect"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct{ Environment, Site, UserLUID string }
type User struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type Plan struct {
	Mode                  string `json:"mode"`
	Operation             string `json:"operation"`
	Environment           string `json:"environment"`
	Site                  string `json:"site"`
	Target                User   `json:"target"`
	OwnershipReassignment bool   `json:"ownership_reassignment"`
}
type Result struct {
	Status           string `json:"status"`
	UserLUID         string `json:"user_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Result  *Result  `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}
type CompactMutationResult struct {
	Status   string `json:"status"`
	UserLUID string `json:"user_luid"`
}
type CompactResult struct {
	Plan    Plan                   `json:"plan"`
	Result  *CompactMutationResult `json:"result,omitempty"`
	Details string                 `json:"details"`
	Help    []string               `json:"help"`
}
type FullResult struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

func (o Output) CompactOutput() any {
	var result *CompactMutationResult
	if o.Result != nil {
		result = &CompactMutationResult{Status: o.Result.Status, UserLUID: o.Result.UserLUID}
	}
	return CompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
}

type Resolver interface {
	ResolveUser(context.Context, string) (User, error)
}
type Deleter interface {
	DeleteUser(context.Context, string) (Result, error)
}
type Action struct {
	resolver Resolver
	deleter  Deleter
}

func New(r Resolver, d Deleter) *Action { return &Action{resolver: r, deleter: d} }
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, errors.New("admin user delete is not configured")
	}
	if in.Environment == "" || in.Site == "" || in.UserLUID == "" {
		return Output{}, &errs.Error{ID: "admin.user.delete.usage", Kind: errs.KindUsage, Operation: "admin.user.delete", Summary: "admin user delete requires explicit environment, site, and user LUID", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment, site, and user LUID.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin user delete requires explicit environment, site, and user LUID"}}}
	}
	user, err := a.resolver.ResolveUser(ctx, in.UserLUID)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.user.delete", Environment: in.Environment, Site: in.Site, Target: user, OwnershipReassignment: false}, Details: "--full", Help: []string{"Run without --preview to remove this exact site user without ownership reassignment."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.resolver.ResolveUser(ctx, in.UserLUID)
	if err != nil {
		return Output{}, err
	}
	if !reflect.DeepEqual(current, user) {
		return Output{}, errors.New("the user delete target changed during revalidation")
	}
	result, err := a.deleter.DeleteUser(ctx, user.LUID)
	if err != nil {
		if result.Status == "unknown" {
			luid := result.UserLUID
			if luid == "" {
				luid = user.LUID
			}
			return Output{}, &errs.Error{ID: "admin.user.delete.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.user.delete", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The user delete outcome could not be determined safely.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the target site and Tableau request before attempting another user delete.", TableauRequestID: result.TableauRequestID}
		}
		return Output{}, err
	}
	out.Result = &result
	out.Help = []string{"tadx admin user list"}
	return out, nil
}
