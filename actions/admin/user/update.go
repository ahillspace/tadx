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
	Status           string              `json:"status"`
	UserLUID         string              `json:"user_luid"`
	User             *UpdateUser         `json:"user,omitempty"`
	TableauRequestID string              `json:"tableau_request_id,omitempty"`
	FieldResults     []UpdateFieldResult `json:"field_results,omitempty"`
}
type UpdateFieldResult struct {
	Field     string  `json:"field"`
	Status    string  `json:"status"`
	Requested string  `json:"requested"`
	Actual    *string `json:"actual,omitempty"`
}
type UpdateOutput struct {
	Plan   UpdatePlan    `json:"plan"`
	Result *UpdateResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type UpdateCompactResult struct {
	Plan   UpdatePlan `json:"plan"`
	Result *struct {
		Status       string              `json:"status"`
		UserLUID     string              `json:"user_luid"`
		User         *UpdateUser         `json:"user,omitempty"`
		FieldResults []UpdateFieldResult `json:"field_results,omitempty"`
	} `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}

func (o UpdateOutput) CompactOutput() any {
	v := UpdateCompactResult{Plan: o.Plan, Details: "--full", Help: o.Help}
	if o.Result != nil {
		v.Result = &struct {
			Status       string              `json:"status"`
			UserLUID     string              `json:"user_luid"`
			User         *UpdateUser         `json:"user,omitempty"`
			FieldResults []UpdateFieldResult `json:"field_results,omitempty"`
		}{o.Result.Status, o.Result.UserLUID, o.Result.User, o.Result.FieldResults}
	}
	return v
}
func (o UpdateOutput) FullOutput() any { return o }

type UpdateWriter interface {
	UpdateValidator
	UpdateUser(context.Context, string, UpdateRequest) (Record, error)
}
type UpdateValidator interface {
	ValidateUpdate(context.Context, UpdateRequest) error
}

func Update(ctx context.Context, resolver Resolver, updater UpdateWriter, in UpdateInput, preview bool) (UpdateOutput, error) {
	req := UpdateRequest{in.FullName, in.Email, in.SiteRole, in.AuthSetting, in.IdentityPoolName, in.IdPConfigurationID, in.Language, in.Locale}
	record, err := resolver.ResolveUser(ctx, Selector{LUID: in.UserLUID})
	if err != nil {
		return UpdateOutput{}, err
	}
	user := updateUser(record)
	changes := userChanges(user, record.PresentFields, req)
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
	if !reflect.DeepEqual(updateUser(current), user) || !reflect.DeepEqual(userChanges(updateUser(current), current.PresentFields, req), changes) {
		return UpdateOutput{}, errors.New("the user update target changed during revalidation")
	}
	if plan.NoOp {
		out.Result = &UpdateResult{Status: "unchanged", UserLUID: user.LUID, User: new(user)}
		return out, nil
	}
	validationRequest := req
	validationRequest.FullName = nil
	for _, change := range changes {
		if change.Field == "full_name" {
			validationRequest.FullName = req.FullName
			break
		}
	}
	if err := updater.ValidateUpdate(ctx, validationRequest); err != nil {
		return out, err
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
			out.Result = &UpdateResult{Status: "unknown", UserLUID: luid, TableauRequestID: updated.RequestID}
			out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", luid)}
			return out, updateOutcomeUnknown(in, luid, updated.RequestID, err)
		}
		return UpdateOutput{}, err
	}
	fieldResults, confirmed, failed, unknown := verifyUserUpdate(updatedRecord, req, changes)
	status := "updated"
	if len(failed)+len(unknown) > 0 {
		status = "unverified"
		if len(confirmed) > 0 {
			status = "partial"
		} else if len(unknown) == 0 {
			status = "not_applied"
		}
	}
	out.Result = &UpdateResult{Status: status, UserLUID: updated.LUID, User: new(updated), TableauRequestID: updated.RequestID, FieldResults: fieldResults}
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", updated.LUID)}
	if status != "updated" {
		firstFailed := ""
		if len(failed) > 0 {
			firstFailed = failed[0]
		} else {
			firstFailed = unknown[0]
		}
		outcome := errs.OutcomeUnknown
		if len(confirmed) > 0 && len(unknown) == 0 {
			outcome = errs.OutcomeConfirmed
		}
		return out, &errs.Error{
			ID: "admin.user.update.partial", Kind: errs.KindOperation, Operation: "admin.user.update",
			Resource: updated.LUID, Environment: in.Environment, Site: in.Site,
			Summary:   "The user update response did not confirm every requested change.",
			Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user and Tableau request before reviewing a new update plan: " + out.Help[0],
			TableauRequestID: updated.RequestID, Completed: confirmed, Failed: firstFailed,
			Phase: errs.PhaseVerification, Outcome: outcome,
		}
	}
	return out, nil
}

func verifyUserUpdate(updated Record, req UpdateRequest, changes []UpdateChange) ([]UpdateFieldResult, []string, []string, []string) {
	requested := []struct {
		field string
		value *string
	}{
		{"full_name", req.FullName}, {"email", req.Email}, {"site_role", req.SiteRole}, {"auth_setting", req.AuthSetting},
		{"identity_pool_name", req.IdentityPoolName}, {"idp_configuration_id", req.IdPConfigurationID}, {"language", req.Language}, {"locale", req.Locale},
	}
	results := make([]UpdateFieldResult, 0, len(requested))
	var confirmed, failed, unknown []string
	for _, item := range requested {
		if item.value == nil {
			continue
		}
		attribute, actual := userField(updated, item.field)
		result := UpdateFieldResult{Field: item.field, Requested: *item.value, Status: "unknown"}
		if updated.PresentFields[attribute] {
			result.Actual = new(actual)
			if userFieldMatches(item.field, actual, *item.value) {
				result.Status = "matched"
				for _, change := range changes {
					if change.Field == item.field {
						result.Status = "confirmed"
						confirmed = append(confirmed, item.field)
						break
					}
				}
			} else {
				result.Status = "mismatch"
				failed = append(failed, item.field)
			}
		} else {
			unknown = append(unknown, item.field)
		}
		results = append(results, result)
	}
	return results, confirmed, failed, unknown
}

func userField(u Record, field string) (string, string) {
	switch field {
	case "full_name":
		return "fullName", u.FullName
	case "email":
		return "email", u.Email
	case "site_role":
		return "siteRole", u.SiteRole
	case "auth_setting":
		return "authSetting", u.AuthSetting
	case "identity_pool_name":
		return "identityPoolName", u.IdentityPoolName
	case "idp_configuration_id":
		return "idpConfigurationId", u.IdPConfigurationID
	case "language":
		return "language", u.Language
	case "locale":
		return "locale", u.Locale
	default:
		return "", ""
	}
}

func userFieldMatches(field, actual, requested string) bool {
	if field == "auth_setting" {
		// Tableau documents both spellings for the same authentication setting.
		if actual == "TableauIdWithMFA" {
			actual = "TableauIDWithMFA"
		}
		if requested == "TableauIdWithMFA" {
			requested = "TableauIDWithMFA"
		}
	}
	return actual == requested
}

func updateUsage(field, message string) error {
	return &errs.Error{ID: "admin.user.update.usage", Kind: errs.KindUsage, Operation: "admin.user.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the user update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func updateOutcomeUnknown(in UpdateInput, luid, requestID string, cause error) error {
	return &errs.Error{
		ID: "admin.user.update.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.user.update",
		Resource: luid, Environment: in.Environment, Site: in.Site,
		Summary: "The user update outcome could not be determined safely.", Cause: cause,
		Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user and Tableau request before retrying: " + commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", luid),
		TableauRequestID: requestID, Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown,
	}
}
func userChanges(u UpdateUser, present map[string]bool, r UpdateRequest) []UpdateChange {
	v := []UpdateChange{}
	add := func(field, before string, after *string) {
		attribute, _ := userField(Record{}, field)
		if after != nil && (!userFieldMatches(field, before, *after) || (*after == "" && !present[attribute])) {
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
