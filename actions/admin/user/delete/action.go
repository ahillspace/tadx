package delete

import (
	"context"
	"errors"
	"reflect"
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
	Applied bool     `json:"applied"`
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
		result = &CompactMutationResult{Status: o.Result.Status, UserLUID: o.Result.UserLUID}
	}
	return CompactResult{Plan: o.Plan, Applied: o.Applied, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Applied: o.Applied, Result: o.Result, Help: o.Help}
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
func (a *Action) Execute(ctx context.Context, in Input, apply bool) (Output, error) {
	if a == nil || a.resolver == nil || a.deleter == nil {
		return Output{}, errors.New("admin user delete is not configured")
	}
	if in.Environment == "" || in.Site == "" || in.UserLUID == "" {
		return Output{}, errors.New("admin user delete requires explicit environment, site, and user LUID")
	}
	user, err := a.resolver.ResolveUser(ctx, in.UserLUID)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.user.delete", Environment: in.Environment, Site: in.Site, Target: user, OwnershipReassignment: false}, Details: "--full", Help: []string{"Add --apply to remove this exact site user without ownership reassignment."}}
	if !apply {
		return out, nil
	}
	current, err := a.resolver.ResolveUser(ctx, in.UserLUID)
	if err != nil {
		return Output{}, err
	}
	if !reflect.DeepEqual(current, user) {
		return Output{}, errors.New("the user delete target changed after preview")
	}
	result, err := a.deleter.DeleteUser(ctx, user.LUID)
	if err != nil {
		return Output{}, err
	}
	out.Applied = true
	out.Result = &result
	out.Help = []string{"tadx admin user list"}
	return out, nil
}
