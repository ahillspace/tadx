package create

import (
	"context"
	"errors"
	"sort"
)

type Input struct{ Environment, Site, Name, SiteRole, AuthSetting, IdentityPoolName, IdPConfigurationID, Email, Language, Locale string }
type User struct {
	LUID               string `json:"luid"`
	Name               string `json:"name"`
	SiteRole           string `json:"site_role,omitempty"`
	AuthSetting        string `json:"auth_setting,omitempty"`
	IdPConfigurationID string `json:"idp_configuration_id,omitempty"`
	RequestID          string `json:"-"`
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
}
type Result struct {
	Status           string `json:"status"`
	User             User   `json:"user"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan    Plan     `json:"plan"`
	Applied bool     `json:"applied"`
	Result  *Result  `json:"result,omitempty"`
	Help    []string `json:"help"`
}
type CompactResult struct {
	Plan    Plan `json:"plan"`
	Applied bool `json:"applied"`
	Result  *struct {
		Status   string `json:"status"`
		UserLUID string `json:"user_luid"`
	} `json:"result,omitempty"`
	Details string   `json:"details"`
	Help    []string `json:"help"`
}

func (o Output) CompactOutput() any {
	v := CompactResult{Plan: o.Plan, Applied: o.Applied, Details: "--full", Help: o.Help}
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
func (a *Action) Execute(ctx context.Context, in Input, apply bool) (Output, error) {
	if a == nil || a.finder == nil || a.creator == nil {
		return Output{}, errors.New("admin user create is not configured")
	}
	if in.Environment == "" || in.Site == "" || in.Name == "" || in.SiteRole == "" {
		return Output{}, errors.New("admin user create requires explicit environment, site, name, and site role")
	}
	if (in.AuthSetting == "") == (in.IdPConfigurationID == "") {
		return Output{}, errors.New("admin user create requires exactly one explicit auth setting or IdP configuration ID")
	}
	found, err := a.finder.FindUsers(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	sort.Slice(found, func(i, j int) bool { return found[i].LUID < found[j].LUID })
	if len(found) > 0 {
		return Output{}, errors.New("an exact user name or email collision exists")
	}
	plan := Plan{Mode: "preview", Operation: "admin.user.create", Environment: in.Environment, Site: in.Site, Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdPConfigurationID: in.IdPConfigurationID}
	out := Output{Plan: plan, Help: []string{"Add --apply to create this exact user."}}
	if !apply {
		return out, nil
	}
	found, err = a.finder.FindUsers(ctx, in.Name)
	if err != nil {
		return Output{}, err
	}
	if len(found) > 0 {
		return Output{}, errors.New("the user create target changed after preview")
	}
	user, err := a.creator.CreateUser(ctx, Request{Name: in.Name, SiteRole: in.SiteRole, AuthSetting: in.AuthSetting, IdentityPoolName: in.IdentityPoolName, IdPConfigurationID: in.IdPConfigurationID, Email: in.Email, Language: in.Language, Locale: in.Locale})
	if err != nil {
		return Output{}, err
	}
	out.Applied = true
	out.Result = &Result{Status: "created", User: user, TableauRequestID: user.RequestID}
	out.Help = []string{"tadx admin user get --id " + user.LUID}
	return out, nil
}
