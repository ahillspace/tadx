package create

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct {
	Environment, Site, Name, MinimumSiteRole string
	ExternalUserEnabled                      *bool
}
type Group struct {
	LUID           string `json:"luid"`
	Name           string `json:"name"`
	RequestID      string `json:"-"`
	MutationStatus string `json:"-"`
}
type Request struct {
	Name, MinimumSiteRole string
	ExternalUserEnabled   *bool
}
type Plan struct {
	Mode                string `json:"mode"`
	Operation           string `json:"operation"`
	Environment         string `json:"environment"`
	Site                string `json:"site"`
	Name                string `json:"name"`
	MinimumSiteRole     string `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool  `json:"external_user_enabled,omitempty"`
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

type Finder interface {
	FindGroups(context.Context, string) ([]Group, error)
}
type Creator interface {
	CreateGroup(context.Context, Request) (Group, error)
}
type Action struct {
	finder  Finder
	creator Creator
}

func New(f Finder, c Creator) *Action { return &Action{finder: f, creator: c} }
func (a *Action) Execute(ctx context.Context, in Input, apply bool) (Output, error) {
	if a == nil || a.finder == nil || a.creator == nil {
		return Output{}, errors.New("admin group create is not configured")
	}
	if in.Environment == "" || in.Site == "" || in.Name == "" {
		return Output{}, &errs.Error{ID: "admin.group.create.usage", Kind: errs.KindUsage, Operation: "admin.group.create", Summary: "admin group create requires explicit environment, site, and name", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment, site, and group name.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin group create requires explicit environment, site, and name"}}}
	}
	found, err := a.finder.FindGroups(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	if len(found) > 0 {
		return Output{}, errors.New("an exact group name collision exists")
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.group.create", Environment: in.Environment, Site: in.Site, Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled}, Details: "--full", Help: []string{"Add --apply to create this exact group."}}
	if !apply {
		return out, nil
	}
	found, err = a.finder.FindGroups(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	if len(found) > 0 {
		return Output{}, errors.New("the group create target changed after preview")
	}
	g, err := a.creator.CreateGroup(ctx, Request{Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled})
	if err != nil {
		if g.MutationStatus == "unknown" {
			return Output{}, &errs.Error{ID: "admin.group.create.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.group.create", Resource: g.LUID, Environment: in.Environment, Site: in.Site, Summary: "The group create outcome could not be determined safely.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the target site and Tableau request before attempting another group create.", TableauRequestID: g.RequestID}
		}
		return Output{}, err
	}
	out.Applied = true
	out.Result = &Result{Status: "created", GroupLUID: g.LUID, TableauRequestID: g.RequestID}
	out.Help = []string{"tadx admin group get --id " + g.LUID}
	return out, nil
}
