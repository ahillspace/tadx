package create

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                           bool
	Environment, Site, Name, MinimumSiteRole string
	ExternalUserEnabled                      *bool
}
type Group struct {
	LUID                string `json:"luid"`
	Name                string `json:"name"`
	MinimumSiteRole     string `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool  `json:"external_user_enabled,omitempty"`
	RequestID           string `json:"-"`
	MutationStatus      string `json:"-"`
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
	UnverifiedSettings  []string `json:"unverified_settings,omitempty"`
	Status              string   `json:"status"`
	GroupLUID           string   `json:"group_luid"`
	MinimumSiteRole     string   `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool    `json:"external_user_enabled,omitempty"`
	TableauRequestID    string   `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Result  *Result  `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}
type CompactMutationResult struct {
	UnverifiedSettings  []string `json:"unverified_settings,omitempty"`
	Status              string   `json:"status"`
	GroupLUID           string   `json:"group_luid"`
	MinimumSiteRole     string   `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool    `json:"external_user_enabled,omitempty"`
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
		result = &CompactMutationResult{UnverifiedSettings: o.Result.UnverifiedSettings, Status: o.Result.Status, GroupLUID: o.Result.GroupLUID, MinimumSiteRole: o.Result.MinimumSiteRole, ExternalUserEnabled: o.Result.ExternalUserEnabled}
	}
	return CompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o Output) FullOutput() any {
	return FullResult{Plan: o.Plan, Result: o.Result, Help: o.Help}
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
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if a == nil || a.finder == nil || a.creator == nil {
		return Output{}, errors.New("admin group create is not configured")
	}
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	if in.Site == "" && !in.TargetResolved {
		return Output{}, &errs.Error{ID: "admin.group.create.usage", Kind: errs.KindUsage, Operation: "admin.group.create", Summary: "admin group create requires explicit environment, site, and name", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment, site, and group name.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin group create requires explicit environment, site, and name"}}}
	}
	found, err := a.finder.FindGroups(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	if len(found) > 0 {
		return Output{}, errors.New("an exact group name collision exists")
	}
	out := Output{Plan: Plan{Mode: "preview", Operation: "admin.group.create", Environment: in.Environment, Site: in.Site, Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled}, Details: "--full", Help: []string{"Run without --preview to create this exact group."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	out.Help = nil
	found, err = a.finder.FindGroups(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	if len(found) > 0 {
		return Output{}, errors.New("the group create target changed during revalidation")
	}
	g, err := a.creator.CreateGroup(ctx, Request{Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled})
	if err != nil {
		if g.MutationStatus == "unknown" {
			out.Help = []string{recoveryHint(in, g.LUID)}
			if g.LUID != "" {
				out.Result = &Result{Status: "unknown", GroupLUID: g.LUID, TableauRequestID: g.RequestID}
			}
			return out, &errs.Error{ID: "admin.group.create.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.group.create", Resource: g.LUID, Environment: in.Environment, Site: in.Site, Summary: "The group create outcome could not be determined safely.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact group and Tableau request before retrying: " + recoveryHint(in, g.LUID), TableauRequestID: g.RequestID, Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
		}
		return Output{}, err
	}
	out.Result = &Result{Status: "created", GroupLUID: g.LUID, MinimumSiteRole: g.MinimumSiteRole, ExternalUserEnabled: g.ExternalUserEnabled, TableauRequestID: g.RequestID}
	if in.MinimumSiteRole != "" && g.MinimumSiteRole == "" {
		out.Result.UnverifiedSettings = append(out.Result.UnverifiedSettings, "minimum_site_role")
	}
	if in.ExternalUserEnabled != nil && g.ExternalUserEnabled == nil {
		out.Result.UnverifiedSettings = append(out.Result.UnverifiedSettings, "external_user_enabled")
	}
	return out, nil
}
