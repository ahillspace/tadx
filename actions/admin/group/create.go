package group

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type CreateInput struct {
	Environment, Site, Name, MinimumSiteRole string
	ExternalUserEnabled                      *bool
}
type CreateRequest = value.AdminCreateGroupRequest
type CreatePlan struct {
	Mode                string `json:"mode"`
	Operation           string `json:"operation"`
	Environment         string `json:"environment"`
	Site                string `json:"site"`
	Name                string `json:"name"`
	MinimumSiteRole     string `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool  `json:"external_user_enabled,omitempty"`
}
type CreateResult struct {
	UnverifiedSettings  []string `json:"unverified_settings,omitempty"`
	Status              string   `json:"status"`
	GroupLUID           string   `json:"group_luid"`
	MinimumSiteRole     string   `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool    `json:"external_user_enabled,omitempty"`
	TableauRequestID    string   `json:"tableau_request_id,omitempty"`
}
type CreateOutput struct {
	Plan   CreatePlan    `json:"plan"`
	Result *CreateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type CreateCompactMutationResult struct {
	UnverifiedSettings  []string `json:"unverified_settings,omitempty"`
	Status              string   `json:"status"`
	GroupLUID           string   `json:"group_luid"`
	MinimumSiteRole     string   `json:"minimum_site_role,omitempty"`
	ExternalUserEnabled *bool    `json:"external_user_enabled,omitempty"`
}
type CreateCompactResult struct {
	Plan    CreatePlan                   `json:"plan"`
	Result  *CreateCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}

func (o CreateOutput) CompactOutput() any {
	var result *CreateCompactMutationResult
	if o.Result != nil {
		result = &CreateCompactMutationResult{UnverifiedSettings: o.Result.UnverifiedSettings, Status: o.Result.Status, GroupLUID: o.Result.GroupLUID, MinimumSiteRole: o.Result.MinimumSiteRole, ExternalUserEnabled: o.Result.ExternalUserEnabled}
	}
	return CreateCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o CreateOutput) FullOutput() any {
	return o
}

type CreateFinder interface {
	GroupExists(context.Context, string) (bool, error)
}
type CreateWriter interface {
	CreateGroup(context.Context, CreateRequest) (value.AdminGroup, error)
}

func Create(ctx context.Context, finder CreateFinder, creator CreateWriter, in CreateInput, preview bool) (CreateOutput, error) {
	found, err := finder.GroupExists(ctx, in.Name)
	if err != nil {
		return CreateOutput{}, err
	}
	if found {
		return CreateOutput{}, errors.New("an exact group name collision exists")
	}
	out := CreateOutput{Plan: CreatePlan{Mode: "preview", Operation: "admin.group.create", Environment: in.Environment, Site: in.Site, Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled}, Help: []string{"Run without --preview to create this exact group."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	out.Help = nil
	found, err = finder.GroupExists(ctx, in.Name)
	if err != nil {
		return CreateOutput{}, err
	}
	if found {
		return CreateOutput{}, errors.New("the group create target changed during revalidation")
	}
	g, err := creator.CreateGroup(ctx, CreateRequest{Name: in.Name, MinimumSiteRole: in.MinimumSiteRole, ExternalUserEnabled: in.ExternalUserEnabled})
	if err != nil {
		if g.MutationStatus == "unknown" {
			out.Help = []string{createRecoveryHint(in, g.LUID)}
			if g.LUID != "" {
				out.Result = &CreateResult{Status: "unknown", GroupLUID: g.LUID, TableauRequestID: g.RequestID}
			}
			resource := g.LUID
			if resource == "" {
				resource = in.Name
			}
			return out, &errs.Error{ID: "admin.group.create.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.group.create", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: "The group create outcome could not be determined safely.", Cause: err, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact group and Tableau request before retrying: " + createRecoveryHint(in, g.LUID), TableauRequestID: g.RequestID, Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
		}
		return CreateOutput{}, err
	}
	out.Result = &CreateResult{Status: "created", GroupLUID: g.LUID, MinimumSiteRole: g.MinimumSiteRole, ExternalUserEnabled: g.ExternalUserEnabled, TableauRequestID: g.RequestID}
	if in.MinimumSiteRole != "" && g.MinimumSiteRole == "" {
		out.Result.UnverifiedSettings = append(out.Result.UnverifiedSettings, "minimum_site_role")
	}
	if in.ExternalUserEnabled != nil && g.ExternalUserEnabled == nil {
		out.Result.UnverifiedSettings = append(out.Result.UnverifiedSettings, "external_user_enabled")
	}
	return out, nil
}

func createRecoveryHint(input CreateInput, id string) string {
	if id != "" {
		return commandhint.Environment(input.Environment, "admin", "group", "inspect", "--id", id)
	}
	return commandhint.Environment(input.Environment, "admin", "group", "inspect", "--name", input.Name)
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateCreateInput(in CreateInput) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.Name) == "" {
		return &errs.Error{ID: "admin.group.create.usage", Kind: errs.KindUsage, Operation: "admin.group.create", Summary: "admin group create requires explicit environment and name", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment and group name.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin group create requires explicit environment and name"}}}
	}
	return nil
}
