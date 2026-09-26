package delete

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"reflect"
	"strings"
)

type Input struct {
	TargetResolved                        bool
	Environment, Site, UserLUID, Username string
}
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
	RemovalStatus    string `json:"removal_status,omitempty"`
	LicenseStatus    string `json:"license_status,omitempty"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Result  *Result  `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}
type CompactMutationResult struct {
	Status        string `json:"status"`
	UserLUID      string `json:"user_luid"`
	RemovalStatus string `json:"removal_status,omitempty"`
	LicenseStatus string `json:"license_status,omitempty"`
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
		result = &CompactMutationResult{Status: o.Result.Status, UserLUID: o.Result.UserLUID, RemovalStatus: o.Result.RemovalStatus, LicenseStatus: o.Result.LicenseStatus}
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
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	if in.Site == "" && !in.TargetResolved {
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
		return a.failedOutput(ctx, in, user, out, result, err)
	}
	if result.UserLUID == "" {
		result.UserLUID = user.LUID
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "list")}
	return out, nil
}

type upstreamCodeCarrier interface {
	TableauCode() string
}

func (a *Action) failedOutput(ctx context.Context, in Input, user User, out Output, result Result, cause error) (Output, error) {
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
		observed, readErr := a.resolver.ResolveUser(ctx, luid)
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
	var carrier upstreamCodeCarrier
	if errors.As(err, &carrier) && carrier.TableauCode() == "409003" {
		return true
	}
	var structured *errs.Error
	return errors.As(err, &structured) && structured.UpstreamCode == "409003"
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || (strings.TrimSpace(in.UserLUID) == "") == (strings.TrimSpace(in.Username) == "") {
		return &errs.Error{ID: "admin.user.delete.usage", Kind: errs.KindUsage, Operation: "admin.user.delete", Summary: "admin user delete requires explicit environment and exactly one of user LUID or username", Retryable: errs.Bool(false), CorrectiveAction: "Provide an exact environment and exactly one of --id or --username.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "required", Message: "admin user delete requires explicit environment and exactly one of --id or --username"}}}
	}
	return nil
}
