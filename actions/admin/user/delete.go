package user

import (
	"context"
	"errors"
	"reflect"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type DeleteInput struct {
	Environment, Site, UserLUID, Username string
}
type DeleteUser struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type DeletePlan struct {
	Mode                  string     `json:"mode"`
	Operation             string     `json:"operation"`
	Environment           string     `json:"environment"`
	Site                  string     `json:"site"`
	Target                DeleteUser `json:"target"`
	OwnershipReassignment bool       `json:"ownership_reassignment"`
}
type DeleteResult struct {
	Status           string `json:"status"`
	UserLUID         string `json:"user_luid"`
	RemovalStatus    string `json:"removal_status,omitempty"`
	LicenseStatus    string `json:"license_status,omitempty"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type DeleteOutput struct {
	Plan   DeletePlan    `json:"plan"`
	Result *DeleteResult `json:"result,omitempty"`
	Help   []string      `json:"help"`
}
type DeleteCompactMutationResult struct {
	Status        string `json:"status"`
	UserLUID      string `json:"user_luid"`
	RemovalStatus string `json:"removal_status,omitempty"`
	LicenseStatus string `json:"license_status,omitempty"`
}
type DeleteCompactResult struct {
	Plan    DeletePlan                   `json:"plan"`
	Result  *DeleteCompactMutationResult `json:"result,omitempty"`
	Details string                       `json:"details"`
	Help    []string                     `json:"help"`
}

func (o DeleteOutput) CompactOutput() any {
	var result *DeleteCompactMutationResult
	if o.Result != nil {
		result = &DeleteCompactMutationResult{Status: o.Result.Status, UserLUID: o.Result.UserLUID, RemovalStatus: o.Result.RemovalStatus, LicenseStatus: o.Result.LicenseStatus}
	}
	return DeleteCompactResult{Plan: o.Plan, Result: result, Details: "--full", Help: o.Help}
}
func (o DeleteOutput) FullOutput() any {
	return o
}

type DeleteWriter interface {
	DeleteUser(context.Context, string) (DeleteResult, error)
}

func Delete(ctx context.Context, resolver Resolver, deleter DeleteWriter, in DeleteInput, preview bool) (DeleteOutput, error) {
	record, err := resolver.ResolveUser(ctx, Selector{LUID: in.UserLUID})
	if err != nil {
		return DeleteOutput{}, err
	}
	user := deleteUser(record)
	out := DeleteOutput{Plan: DeletePlan{Mode: "preview", Operation: "admin.user.delete", Environment: in.Environment, Site: in.Site, Target: user, OwnershipReassignment: false}, Help: []string{"Run without --preview to remove this exact site user without ownership reassignment."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := resolver.ResolveUser(ctx, Selector{LUID: in.UserLUID})
	if err != nil {
		return DeleteOutput{}, err
	}
	if !reflect.DeepEqual(deleteUser(current), user) {
		return DeleteOutput{}, errors.New("the user delete target changed during revalidation")
	}
	result, err := deleter.DeleteUser(ctx, user.LUID)
	if err != nil {
		return failedDeleteOutput(ctx, resolver, in, user, out, result, err)
	}
	if result.UserLUID == "" {
		result.UserLUID = user.LUID
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "list")}
	return out, nil
}

type upstreamCodeCarrier interface {
	error
	TableauCode() string
}

func failedDeleteOutput(ctx context.Context, resolver Resolver, in DeleteInput, user DeleteUser, out DeleteOutput, result DeleteResult, cause error) (DeleteOutput, error) {
	luid := result.UserLUID
	if luid == "" {
		luid = user.LUID
	}
	result.UserLUID = luid
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", luid, "--full")}
	if isAssetConflict(cause) {
		result.Status = "refused"
		result.RemovalStatus = "refused"
		result.LicenseStatus = "unknown"
		observed, readErr := resolver.ResolveUser(ctx, Selector{LUID: luid})
		if readErr == nil && observed.LUID == luid && strings.EqualFold(observed.SiteRole, "Unlicensed") {
			result.LicenseStatus = "unlicensed"
			result.Status = "unlicensed"
		}
		out.Result = &result
		outcome := errs.OutcomeUnknown
		summary := "Tableau refused user removal, and the resulting site license status could not be confirmed."
		if result.LicenseStatus == "unlicensed" {
			outcome = errs.OutcomeConfirmed
			summary = "Tableau refused user removal after the user became unlicensed."
		}
		return out, &errs.Error{ID: "admin.user.delete.partial", Kind: errs.KindOperation, Operation: "admin.user.delete", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user before retrying: " + out.Help[0] + "; reassign owned content before attempting removal again.", TableauRequestID: result.TableauRequestID, Phase: errs.PhaseVerification, Outcome: outcome}
	}
	if result.Status == "" {
		result.Status = "unknown"
	}
	out.Result = &result
	return out, &errs.Error{ID: "admin.user.delete.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.user.delete", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The user delete outcome could not be determined safely.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user and Tableau request before retrying: " + out.Help[0], TableauRequestID: result.TableauRequestID, Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
}

func isAssetConflict(err error) bool {
	if carrier, ok := errors.AsType[upstreamCodeCarrier](err); ok && carrier.TableauCode() == "409003" {
		return true
	}
	structured, ok := errors.AsType[*errs.Error](err)
	return ok && structured.UpstreamCode == "409003"
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateDeleteInput(in DeleteInput) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.UserLUID) == "") == (strings.TrimSpace(in.Username) == "") {
		return &errs.Error{ID: "admin.user.delete.usage", Kind: errs.KindUsage, Operation: "admin.user.delete", Summary: "admin user delete requires explicit environment and exactly one of user LUID or username", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment and exactly one of --id or --username.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin user delete requires explicit environment and exactly one of --id or --username"}}}
	}
	return nil
}

func deleteUser(v Record) DeleteUser {
	return DeleteUser{LUID: v.LUID, Name: v.Name, SiteRole: v.SiteRole}
}
