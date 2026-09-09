package create

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"sort"

	"github.com/ahillspace/tadx/internal/errs"
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
	Status           string `json:"status"`
	User             User   `json:"user"`
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
		}{o.Result.Status, o.Result.User.LUID}
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
			return Output{}, outcomeUnknown(in, user.LUID, user.RequestID, err)
		}
		return Output{}, err
	}
	out.Result = &Result{Status: "created", User: user, TableauRequestID: user.RequestID}
	out.Help = []string{commandhint.Environment(in.Environment, "admin", "user", "inspect", "--id", user.LUID)}
	return out, nil
}

func usage(field, message string) error {
	return &errs.Error{ID: "admin.user.create.usage", Kind: errs.KindUsage, Operation: "admin.user.create", Summary: message, Retryable: errs.Bool(false), CorrectiveAction: "Correct the user create input and review a new preview.", Validation: []errs.ValidationDetail{{Field: field, Code: "invalid", Message: message}}}
}
func outcomeUnknown(in Input, luid, requestID string, cause error) error {
	return &errs.Error{ID: "admin.user.create.outcome_unknown", Kind: errs.KindOperation, Operation: "admin.user.create", Resource: luid, Environment: in.Environment, Site: in.Site, Summary: "The user create outcome could not be determined safely.", Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact user and Tableau request before retrying: " + recoveryHint(in, luid), TableauRequestID: requestID}
}
