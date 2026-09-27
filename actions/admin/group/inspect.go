package group

import (
	"context"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/readsource"
)

const memberLimit = 100

type Selector struct{ LUID, Name string }
type InspectInput struct {
	Environment, Site string
	Selector          Selector
	IncludeMembers    bool
	Cache             bool
}

type Member struct {
	LUID     string `json:"luid"`
	Name     string `json:"name"`
	SiteRole string `json:"site_role,omitempty"`
}
type Record struct {
	LUID                     string   `json:"luid"`
	Name                     string   `json:"name"`
	Domain                   string   `json:"domain,omitempty"`
	MinimumSiteRole          string   `json:"minimum_site_role,omitempty"`
	GrantLicenseMode         string   `json:"grant_license_mode,omitempty"`
	ExternalUserEnabled      *bool    `json:"external_user_enabled,omitempty"`
	ExternalUserEnabledState string   `json:"external_user_enabled_state,omitempty"`
	Members                  []Member `json:"members"`
	MembersOmitted           int      `json:"members_omitted,omitempty"`
	RequestID                string   `json:"-"`
	MutationStatus           string   `json:"-"`
}
type InspectOutput struct {
	Status, Environment, Site string
	Group                     Record
	membersRequested          bool
	RequestID                 string
	Help                      []string
	Source                    *readsource.Metadata
}
type InspectCompactGroup struct {
	LUID        string    `json:"luid"`
	Name        string    `json:"name"`
	Domain      string    `json:"domain,omitempty"`
	MemberCount int       `json:"member_count,omitempty"`
	Members     *[]Member `json:"members,omitempty"`
}
type InspectCompactResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Group       InspectCompactGroup  `json:"group"`
	Details     string               `json:"details"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}
type InspectFullResult struct {
	Status      string               `json:"status"`
	Environment string               `json:"environment,omitempty"`
	Site        string               `json:"site,omitempty"`
	Group       Record               `json:"group"`
	RequestID   string               `json:"tableau_request_id,omitempty"`
	Help        []string             `json:"help"`
	Source      *readsource.Metadata `json:"source,omitempty"`
}

func (o InspectOutput) CompactOutput() any {
	group := InspectCompactGroup{LUID: o.Group.LUID, Name: o.Group.Name, Domain: o.Group.Domain, MemberCount: len(o.Group.Members)}
	if o.membersRequested {
		members := append([]Member{}, o.Group.Members...)
		group.Members = &members
	}
	return InspectCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Group: group, Details: "--full", Help: o.Help, Source: o.Source}
}
func (o InspectOutput) FullOutput() any {
	g := o.Group
	g.Members = append([]Member(nil), g.Members...)
	if o.membersRequested {
		g.Members = append([]Member{}, g.Members...)
	}
	if g.ExternalUserEnabled == nil {
		g.ExternalUserEnabledState = "not_reported"
	}
	if !o.membersRequested && len(g.Members) > memberLimit {
		g.MembersOmitted = len(g.Members) - memberLimit
		g.Members = g.Members[:memberLimit]
	}
	return InspectFullResult{Status: o.Status, Environment: o.Environment, Site: o.Site, Group: g, RequestID: o.RequestID, Help: o.Help, Source: o.Source}
}

type Resolver interface {
	ResolveGroup(context.Context, Selector, bool) (Record, error)
}

func Inspect(ctx context.Context, resolver Resolver, in InspectInput) (InspectOutput, error) {
	g, err := resolver.ResolveGroup(ctx, in.Selector, in.IncludeMembers)
	if err != nil {
		return InspectOutput{}, err
	}
	var help []string
	if !in.IncludeMembers {
		help = []string{commandhint.Environment(in.Environment, "admin", "group", "inspect", "--id", g.LUID, "--members")}
	}
	return InspectOutput{Status: "found", Environment: in.Environment, Site: in.Site, Group: g, membersRequested: in.IncludeMembers, RequestID: g.RequestID, Help: help}, nil
}

// ValidateInput checks one exact selector before authentication.
func ValidateInspectInput(input InspectInput) error {
	id, name := strings.TrimSpace(input.Selector.LUID), strings.TrimSpace(input.Selector.Name)
	if (id == "") == (name == "") {
		return &errs.Error{ID: "admin.group.inspect.usage", Kind: errs.KindUsage, Operation: "admin.group.inspect", Summary: "Provide exactly one authoritative LUID or exact group name.", Retryable: errs.Bool(false), CorrectiveAction: "Use --id or --name, but not both.", Validation: []errs.ValidationDetail{{Field: "selector", Code: "invalid", Message: "exactly one selector is required"}}}
	}
	return nil
}
