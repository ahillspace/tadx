package update

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"reflect"

	"github.com/ahillspace/tadx/internal/errs"
)

type Input struct {
	// TargetResolved confirms authenticated target selection, including the Default site.
	TargetResolved                                                                                 bool
	Environment, Site, UserLUID                                                                    string
	FullName, Email, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID, Language, Locale *string
}
type User struct {
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
type Request struct{ FullName, Email, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID, Language, Locale *string }
type Change struct {
	Field  string `json:"field"`
	Before string `json:"before,omitempty"`
	After  string `json:"after,omitempty"`
}
type Plan struct {
	Mode        string   `json:"mode"`
	Operation   string   `json:"operation"`
	Environment string   `json:"environment"`
	Site        string   `json:"site"`
	Target      User     `json:"target"`
	Changes     []Change `json:"changes"`
	NoOp        bool     `json:"no_op"`
}
type Result struct {
	Status           string `json:"status"`
	UserLUID         string `json:"user_luid"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type CompactResult struct {
	Plan   Plan `json:"plan"`
	Result *struct {
		Status   string `json:"status"`
		UserLUID string `json:"user_luid"`
	} `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}

func (o Output) CompactOutput() any {
	v := CompactResult{Plan: o.Plan, Details: "--full", Help: o.Help}
	if o.Result != nil {
		v.Result = &struct {
			Status   string `json:"status"`
			UserLUID string `json:"user_luid"`
		}{o.Result.Status, o.Result.UserLUID}
	}
	return v
}
func (o Output) FullOutput() any { return o }

type Resolver interface {
	ResolveUser(context.Context, string) (User, error)
}
type Updater interface {
	UpdateUser(context.Context, string, Request) (User, error)
}
type Action struct {
	resolver Resolver
	updater  Updater
}

func New(r Resolver, u Updater) *Action { return &Action{resolver: r, updater: u} }
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if a == nil || a.resolver == nil || a.updater == nil {
		return Output{}, errors.New("admin user update is not configured")
	}
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	if in.Site == "" && !in.TargetResolved {
		return Output{}, usage("selector", "admin user update requires --environment and --id; the environment selects the site")
	}
	req := Request{in.FullName, in.Email, in.SiteRole, in.AuthSetting, in.IdentityPoolName, in.IdPConfigurationID, in.Language, in.Locale}
	user, err := a.resolver.ResolveUser(ctx, in.UserLUID)
	if err != nil {
		return Output{}, err
	}
	changes := userChanges(user, req)
	plan := Plan{Mode: "preview", Operation: "admin.user.update", Environment: in.Environment, Site: in.Site, Target: user, Changes: changes, NoOp: len(changes) == 0}
	out := Output{Plan: plan, Help: []string{"Run without --preview to update this exact user."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	current, err := a.resolver.ResolveUser(ctx, in.UserLUID)
	if err != nil {
		return Output{}, err
	}
	if !reflect.DeepEqual(current, user) {
		return Output{}, errors.New("the user update target changed during revalidation")
	}
	if plan.NoOp {
		out.Result = &Result{Status: "unchanged", UserLUID: user.LUID}
		return out, nil
	}
	updated, err := a.updater.UpdateUser(ctx, user.LUID, req)
	if err != nil {
		if updated.MutationStatus == "unknown" {
			luid := updated.LUID
			if luid == "" {
				luid = user.LUID
			}
			return Output{}, outcomeUnknown(in, luid, updated.RequestID, err)
		}
		return Output{}, err
	}
	out.Result = &Result{Status: "updated", UserLUID: updated.LUID, TableauRequestID: updated.RequestID}
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", updated.LUID)}
	return out, nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "admin.user.update.usage", Kind: errs.KindUsage, Operation: "admin.user.update", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the user update input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func outcomeUnknown(in Input, luid, requestID string, cause error) error {
	return &errs.Error{ID: "admin.user.update.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.user.update", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The user update outcome could not be determined safely.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user and Tableau request before retrying: " + commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", luid), TableauRequestID: requestID}
}
func userChanges(u User, r Request) []Change {
	v := []Change{}
	add := func(field, before string, after *string) {
		if after != nil && *after != before {
			v = append(v, Change{field, before, *after})
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
