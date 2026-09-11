package inspect

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"

	"github.com/ahillspace/tadx/internal/readsource"
)

const memberLimit = 100

type Selector struct{ LUID, Name string }
type Input struct {
	Environment, Site string
	Selector          Selector
	IncludeMembers    bool
	Cache             bool
}

func (i *Input) SetSelector(luid, name string) { i.Selector = Selector{LUID: luid, Name: name} }

type Member struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type Group struct {
	LUID                string   `json:"luid"`
	Name                string   `json:"name"`
	Domain              string   `json:"domain,omitempty"`
	MinimumSiteRole     string   `json:"minimum_site_role,omitempty"`
	GrantLicenseMode    string   `json:"grant_license_mode,omitempty"`
	ExternalUserEnabled *bool    `json:"external_user_enabled,omitempty"`
	Members             []Member `json:"members,omitempty"`
	MembersOmitted      int      `json:"members_omitted,omitempty"`
	RequestID           string   `json:"-"`
}
type Output struct {
	Status, Environment, Site string
	Group                     Group
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type CompactGroup struct {
	LUID        string `json:"luid"`
	Name        string `json:"name"`
	Domain      string `json:"domain,omitempty"`
	MemberCount int    `json:"member_count,omitempty"`
}
type CompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Group       CompactGroup         `json:"group"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type FullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Group       Group                `json:"group"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o Output) CompactOutput() any {
	return CompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Group: CompactGroup{LUID: o.Group.LUID, Name: o.Group.Name, Domain: o.Group.Domain, MemberCount: len(o.Group.Members)}, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o Output) FullOutput() any {
	g := o.Group
	g.Members = append([]Member(nil), g.Members...)
	if len(g.Members) > memberLimit {
		g.MembersOmitted = len(g.Members) - memberLimit
		g.Members = g.Members[:memberLimit]
	}
	return FullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Group: g, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type Resolver interface {
	ResolveGroup(context.Context, Selector, bool) (Group, error)
}
type Action struct{ resolver Resolver }

func New(r Resolver) *Action { return &Action{resolver: r} }
func (a *Action) Execute(ctx context.Context, in Input) (Output, error) {
	if a == nil || a.resolver == nil {
		return Output{}, errors.New("admin group inspect resolver is not configured")
	}
	if err := ValidateInput(in); err != nil {
		return Output{}, err
	}
	g, err := a.resolver.ResolveGroup(ctx, in.Selector, in.IncludeMembers)
	if err != nil {
		return Output{}, err
	}
	return Output{Status: "found", Environment: in.Environment, Site: in.Site, Group: g, RequestID: g.RequestID, Help: []string{commandhint.Environment(in.Environment, "admin", "group", "inspect", "--id", g.LUID, "--members", "--full")}}, nil
}
