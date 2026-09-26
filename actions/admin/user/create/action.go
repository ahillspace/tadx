package create

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"sort"
	"strings"
)

type Input struct {
	TargetResolved                                                                                                bool
	Environment, Site, Name, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID, Email, Language, Locale string
}
type User struct {
	LUID               string `json:"luid"`
	Name               string `json:"name"`
	SiteRole           string `json:"site_role,omitempty"`
	AuthSetting        string `json:"auth_setting,omitempty"`
	IdPConfigurationID string `json:"idp_configuration_id,omitempty"`
	IdentityPoolName   string `json:"identity_pool_name,omitempty"`
	Email              string `json:"email,omitempty"`
	Language           string `json:"language,omitempty"`
	Locale             string `json:"locale,omitempty"`
	RequestID          string `json:"-"`
	MutationStatus     string `json:"-"`
}
type Request struct{ Name, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID, Email, Language, Locale string }
type Plan struct {
	Mode               string `json:"mode"`
	Operation          string `json:"operation"`
	Environment        string `json:"environment"`
	Site               string `json:"site"`
	Name               string `json:"name"`
	SiteRole           string `json:"site_role"`
	AuthSetting        string `json:"auth_setting,omitempty"`
	IdPConfigurationID string `json:"idp_configuration_id,omitempty"`
	IdentityPoolName   string `json:"identity_pool_name,omitempty"`
	Email              string `json:"email,omitempty"`
	Language           string `json:"language,omitempty"`
	Locale             string `json:"locale,omitempty"`
}
type Result struct {
	UnverifiedSettings []string `json:"unverified_settings,omitempty"`
	Status             string   `json:"status"`
	User               User     `json:"user"`
	TableauRequestID   string   `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}
type CompactMutationResult struct {
	Status             string   `json:"status"`
	UserLUID           string   `json:"user_luid"`
	SiteRole           string   `json:"site_role,omitempty"`
	AuthSetting        string   `json:"auth_setting,omitempty"`
	IdPConfigurationID string   `json:"idp_configuration_id,omitempty"`
	IdentityPoolName   string   `json:"identity_pool_name,omitempty"`
	Email              string   `json:"email,omitempty"`
	Language           string   `json:"language,omitempty"`
	Locale             string   `json:"locale,omitempty"`
	UnverifiedSettings []string `json:"unverified_settings,omitempty"`
}
type CompactResult struct {
	Plan    Plan                   `json:"plan"`
	Result  *CompactMutationResult `json:"result,omitempty"`
	Details string                 `json:"details"`
	Help    []string               `json:"help"`
}

func (o Output) CompactOutput() any {
	v := CompactResult{Plan: o.Plan, Details: "--full", Help: o.Help}
	if o.Result != nil {
		u := o.Result.User
		v.Result = &CompactMutationResult{Status: o.Result.Status, UserLUID: u.LUID, SiteRole: u.SiteRole, AuthSetting: u.AuthSetting, IdPConfigurationID: u.IdPConfigurationID, IdentityPoolName: u.IdentityPoolName, Email: u.Email, Language: u.Language, Locale: u.Locale, UnverifiedSettings: o.Result.UnverifiedSettings}
	}
	return v
}
func (o Output) FullOutput() any { return o }

type Finder interface {
	FindUsers(context.Context, string) ([]User, error)
}
type Creator interface {
	CreateUser(context.Context, Request) (User, error)
}
type Action struct {
	finder  Finder
	creator Creator
}

func New(f Finder, c Creator) *Action { return &Action{finder: f, creator: c} }
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	if a == nil || a.finder == nil || a.creator == nil {
		return Output{}, errors.New("admin user create is not configured")
	}
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	if in.Site == "" && !in.TargetResolved {
		return Output{}, usage("selector", "admin user create requires --environment, --name, and --site-role; the environment selects the site")
	}
	found, err := a.finder.FindUsers(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	sort.Slice(found, func(i, j int) bool { return found[i].LUID < found[j].LUID })
	if len(found) > 0 {
		return Output{}, errors.New("an exact user name or email collision exists")
	}
	plan := Plan{Mode: "preview", Operation: "admin.user.create", Environment: in.Environment, Site: in.Site, Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdPConfigurationID: in.IdPConfigurationID, IdentityPoolName: in.IdentityPoolName, Email: in.Email, Language: in.Language, Locale: in.Locale}
	out := Output{Plan: plan, Help: []string{"Run without --preview to create this exact user."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	out.Help = nil
	found, err = a.finder.FindUsers(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	if len(found) > 0 {
		return Output{}, errors.New("the user create target changed during revalidation")
	}
	user, err := a.creator.CreateUser(ctx, Request{Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdentityPoolName: in.IdentityPoolName, IdPConfigurationID: in.IdPConfigurationID, Email: in.Email, Language: in.Language, Locale: in.Locale})
	if err != nil {
		if user.MutationStatus == "unknown" {
			out.Help = []string{recoveryHint(in, user.LUID)}
			if user.LUID != "" {
				out.Result = &Result{Status: "unknown", User: user, TableauRequestID: user.RequestID}
			}
			return out, outcomeUnknown(in, user.LUID, user.RequestID, err)
		}
		return Output{}, err
	}
	out.Result = &Result{Status: "created", User: user, TableauRequestID: user.RequestID}
	for _, field := range []struct{ name, requested, observed string }{
		{"site_role", in.SiteRole, user.SiteRole}, {"auth_setting", in.AuthSetting, user.AuthSetting},
		{"identity_pool_name", in.IdentityPoolName, user.IdentityPoolName}, {"idp_configuration_id", in.IdPConfigurationID, user.IdPConfigurationID},
		{"email", in.Email, user.Email}, {"language", in.Language, user.Language}, {"locale", in.Locale, user.Locale},
	} {
		if field.requested != "" && field.observed == "" {
			out.Result.UnverifiedSettings = append(out.Result.UnverifiedSettings, field.name)
		}
	}
	return out, nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "admin.user.create.usage", Kind: errs.KindUsage, Operation: "admin.user.create", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the user create input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func outcomeUnknown(in Input, luid, requestID string, cause error) error {
	resource := luid
	if resource == "" {
		resource = in.Name
	}
	return &errs.Error{ID: "admin.user.create.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.user.create", Resource: resource, Environment: in.Environment, Site: in.Site, Summary: "The user create outcome could not be determined safely.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user and Tableau request before retrying: " + recoveryHint(in, luid), TableauRequestID: requestID, Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
}

func recoveryHint(input Input, id string) string {
	if id != "" {
		return commandhint.Environment(input.Environment, "admin", "user", "inspect", "--id", id)
	}
	return commandhint.Environment(input.Environment, "admin", "user", "inspect", "--name", input.Name)
}

// ValidateInput checks local options without requiring a resolved site or remote session.
func ValidateInput(in Input) error {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.Name) == "" || strings.TrimSpace(in.SiteRole) == "" {
		return usage("selector", "admin user create requires --environment, --name, and --site-role; the environment selects the site")
	}
	if (in.AuthSetting == "") == (in.IdPConfigurationID == "") {
		return usage("auth_setting", "admin user create requires exactly one explicit auth setting or IdP configuration ID")
	}
	if in.AuthSetting != "" {
		switch in.AuthSetting {
		case "ServerDefault", "SAML", "OpenID", "TableauIDWithMFA":
		default:
			return usage("auth_setting", "Supported --auth-setting values: ServerDefault, SAML, OpenID, TableauIDWithMFA. Use --idp-configuration-id for an exact authentication configuration.")
		}
	}
	return nil
}
