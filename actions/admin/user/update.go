package user

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type UpdateInput struct {
	Environment, Site, UserLUID, Username                                                          string
	FullName, Email, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID, Language, Locale *string
}
type UpdateUser struct {
	LUID               string `json:"luid"`
	Name               string `json:"name"`
	FullName           string `json:"full_name,omitempty"`
	Email              string `json:"email,omitempty"`
	SiteRole           string `json:"site_role,omitempty"`
	AuthSetting        string `json:"auth_setting,omitempty"`
	IdentityPoolName   string `json:"identity_pool_name,omitempty"`
	IdPConfigurationID string `json:"idp_configuration_id,omitempty"`
	Language           string `json:"language,omitempty"`
	Locale             string `json:"locale,omitempty"`
	RequestID          string `json:"-"`
	MutationStatus     string `json:"-"`
}
type UpdateRequest struct{ FullName, Email, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID, Language, Locale *string }
type UpdateChange struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type UpdatePlan struct {
	Mode        string         `json:"mode"`
	Operation   string         `json:"operation"`
	Environment string         `json:"environment"`
	Site        string         `json:"site"`
	Target      UpdateUser     `json:"target"`
	Changes     []UpdateChange `json:"changes"`
	NoOp        bool           `json:"no_op"`
}
type UpdateResult struct {
	Status           string     `json:"status"`
	UserLUID         string     `json:"user_luid"`
	User             UpdateUser `json:"user"`
	TableauRequestID string     `json:"tableau_request_id,omitempty"`
}
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type UpdateCompactResult struct {
	Plan   UpdatePlan `json:"plan"`
	Result *struct {
		Status   string     `json:"status"`
		UserLUID string     `json:"user_luid"`
		User     UpdateUser `json:"user"`
	} `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}

func (o UpdateOutput) CompactOutput() any {
	v := UpdateCompactResult{Plan: o.Plan, Details: "--full", Help: o.Help}
	if o.Result != nil {
		v.Result = &struct {
			Status   string     `json:"status"`
			UserLUID string     `json:"user_luid"`
			User     UpdateUser `json:"user"`
		}{o.Result.Status, o.Result.UserLUID, o.Result.User}
	}
	return v
}
func (o UpdateOutput) FullOutput() any { return o }

type UpdateWriter interface {
	UpdateUser(context.Context, string, UpdateRequest) (Record, error)
}

func Update(ctx context.Context, resolver Resolver, updater UpdateWriter, in UpdateInput, preview bool) (UpdateOutput, error) {
	req := UpdateRequest{in.FullName, in.Email, in.SiteRole, in.AuthSetting, in.IdentityPoolName, in.IdPConfigurationID, in.Language, in.Locale}
	record, err := resolver.ResolveUser(ctx, Selector{LUID: in.UserLUID})
	if err != nil {
		return UpdateOutput{}, err
	}
	user := updateUser(record)
	changes := userChanges(user, req)
	plan := UpdatePlan{Mode: "preview", Operation: "admin.user.update", Environment: in.Environment, Site: in.Site, Target: user, Changes: changes, NoOp: len(changes) == 0}
	out := UpdateOutput{Plan: plan, Help: []string{"Run without --preview to update this exact user."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := resolver.ResolveUser(ctx, Selector{LUID: in.UserLUID})
	if err != nil {
		return UpdateOutput{}, err
	}
	if !reflect.DeepEqual(updateUser(current), user) {
		return UpdateOutput{}, errors.New("the user update target changed during revalidation")
	}
	if plan.NoOp {
		out.Result = &UpdateResult{Status: "unchanged", UserLUID: user.LUID, User: user}
		return out, nil
	}
	updatedRecord, err := updater.UpdateUser(ctx, user.LUID, req)
	updated := updateUser(updatedRecord)
	updated.RequestID, updated.MutationStatus = updatedRecord.RequestID, updatedRecord.MutationStatus
	if err != nil {
		if updated.MutationStatus == "unknown" {
			luid := updated.LUID
			if luid == "" {
				luid = user.LUID
			}
			return UpdateOutput{}, updateOutcomeUnknown(in, luid, updated.RequestID, err)
		}
		return UpdateOutput{}, err
	}
	out.Result = &UpdateResult{Status: "updated", UserLUID: updated.LUID, User: updated, TableauRequestID: updated.RequestID}
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", updated.LUID)}
	return out, nil
}

func updateUsage(field, message string) error {
	return &errs.Error{ID: "admin.user.update.usage", Kind: errs.KindUsage, Operation: "admin.user.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the user update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func updateOutcomeUnknown(in UpdateInput, luid, requestID string, cause error) error {
	return &errs.Error{ID: "admin.user.update.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.user.update", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The user update outcome could not be determined safely.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user and Tableau request before retrying: " + commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", luid), TableauRequestID: requestID}
}
func userChanges(u UpdateUser, r UpdateRequest) []UpdateChange {
	v := []UpdateChange{}
	add := func(field, before string, after *string) {
		if after != nil && *after != before {
			v = append(v, UpdateChange{field, before, *after})
		}
	}
	add("full_name", u.FullName, r.FullName)
	add("email", u.Email, r.Email)
	add("site_role", u.SiteRole, r.SiteRole)
	add("auth_setting", u.AuthSetting, r.AuthSetting)
	add("identity_pool_name", u.IdentityPoolName, r.IdentityPoolName)
	add("idp_configuration_id", u.IdPConfigurationID, r.IdPConfigurationID)
	add("language", u.Language, r.Language)
	add("locale", u.Locale, r.Locale)
	return v
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateUpdateInput(in UpdateInput) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.UserLUID) == "") == (strings.TrimSpace(in.Username) == "") {
		return updateUsage("selector", "admin user update requires --environment and exactly one of --id or --username; the environment selects the site")
	}
	if in.AuthSetting != nil && in.IdPConfigurationID != nil {
		return updateUsage("auth_setting", "admin user update cannot set auth setting and IdP configuration ID together")
	}
	if in.AuthSetting != nil && !validAuthSetting(*in.AuthSetting) {
		return updateUsage("auth_setting", authSettingGuidance)
	}
	req := UpdateRequest{in.FullName, in.Email, in.SiteRole, in.AuthSetting, in.IdentityPoolName, in.IdPConfigurationID, in.Language, in.Locale}
	if reflect.DeepEqual(req, UpdateRequest{}) {
		return updateUsage("fields", "admin user update requires at least one explicit field")
	}
	return nil
}

// updateUser preserves the original update snapshot, excluding read diagnostics.
func updateUser(v Record) UpdateUser {
	return UpdateUser{LUID: v.LUID, Name: v.Name, FullName: v.FullName, Email: v.Email, SiteRole: v.SiteRole, AuthSetting: v.AuthSetting, IdentityPoolName: v.IdentityPoolName, IdPConfigurationID: v.IdPConfigurationID, Language: v.Language, Locale: v.Locale}
}
