package user

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

type Selector struct{ LUID, Username string }
type InspectInput struct {
	Environment, Site string
	Selector          Selector
	Cache             bool
}

type Record struct {
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
	MutationStatus     string `json:"-"`
}
type InspectOutput struct {
	Status, Environment, Site string
	User                      Record
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type InspectCompactUser struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type InspectCompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	User        InspectCompactUser   `json:"user"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	User        Record               `json:"user"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o InspectOutput) CompactOutput() any {
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, User: InspectCompactUser{LUID: o.User.LUID, Name: o.User.Name, SiteRole: o.User.SiteRole}, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o InspectOutput) FullOutput() any {
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, User: o.User, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type Resolver interface {
	ResolveUser(context.Context, Selector) (Record, error)
}

func Inspect(ctx context.Context, resolver Resolver, input InspectInput) (InspectOutput, error) {
	user, err := resolver.ResolveUser(ctx, input.Selector)
	if err != nil {
		return InspectOutput{}, err
	}
	return InspectOutput{Status: "found", Environment: input.Environment, Site: input.Site, User: user, RequestID: user.RequestID, Help: []string{commandhint.Environment(input.Environment, "admin", "user", "inspect", "--id", user.LUID, "--full")}}, nil
}

// ValidateInput checks one exact selector before authentication.
func ValidateInspectInput(input InspectInput) error {
	id, name := strings.TrimSpace(input.Selector.LUID), strings.TrimSpace(input.Selector.Username)
	if (id == "") == (name == "") {
		return &errs.Error{ID: "admin.user.inspect.usage", Kind: errs.KindUsage, Operation: "admin.user.inspect", Summary: "Provide exactly one authoritative LUID or exact user name.", Retryable: errs.Bool(false), CorrectiveAction: "Use --id or --name, but not both.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "invalid", Message: "exactly one selector is required"}}}
	}
	return nil
}
