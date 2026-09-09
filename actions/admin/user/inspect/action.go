package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/readsource"
)

type Selector struct{ LUID, NameOrEmail string }
type Input struct {
	Environment, Site string
	Selector          Selector
	Catalog           bool
}

func (i *Input) SetSelector(luid, nameOrEmail string) {
	i.Selector = Selector{LUID: luid, NameOrEmail: nameOrEmail}
}

type User struct {
	LUID               string `json:"luid"`
	Name               string `json:"name"`
	FullName           string `json:"full_name,omitempty"`
	Email              string `json:"email,omitempty"`
	SiteRole           string `json:"site_role,omitempty"`
	LastLogin          string `json:"last_login,omitempty"`
	ExternalAuthUserID string `json:"external_auth_user_id,omitempty"`
	AuthSetting        string `json:"auth_setting,omitempty"`
	IdentityPoolName   string `json:"identity_pool_name,omitempty"`
	IdPConfigurationID string `json:"idp_configuration_id,omitempty"`
	Language           string `json:"language,omitempty"`
	Locale             string `json:"locale,omitempty"`
	Domain             string `json:"domain,omitempty"`
	RequestID          string `json:"-"`
}
type Output struct {
	Status, Environment, Site string
	User                      User
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type CompactUser struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	User        CompactUser          `json:"user"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	User        User                 `json:"user"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, User: CompactUser{LUID: o.User.LUID, Name: o.User.Name, SiteRole: o.User.SiteRole}, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any {
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, User: o.User, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type Resolver interface {
	ResolveUser(context.Context, Selector) (User, error)
}
type Action struct{ resolver Resolver }

func New(r Resolver) *Action { return &Action{resolver: r} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, errors.New("admin user inspect resolver is not configured")
	}
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	user, err := a.resolver.ResolveUser(ctx, input.Selector)
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "found", Environment: input.Environment, Site: input.Site, User: user, RequestID: user.RequestID, Help: []string{commandhint.Environment(input.Environment, "admin", "user", "inspect", "--id", user.LUID, "--full")}}, nil
}
