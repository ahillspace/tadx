package group

import (
	"context"
	"slices"
	"strings"
)

type MembershipInput struct {
	Environment string
	Site        string
	GroupLUID   string
	UserLUID    string
	Username    string
}
type MembershipMember struct {
	LUID string `json:"luid"`
	Name string `json:"name,omitempty"`
}
type MembershipGroup struct {
	LUID    string             `json:"luid"`
	Name    string             `json:"name"`
	Members []MembershipMember `json:"members,omitempty"`
}
type MembershipPlan struct {
	Mode        string `json:"mode"`
	Operation   string `json:"operation"`
	Environment string `json:"environment"`
	Site        string `json:"site"`
	GroupLUID   string `json:"group_luid"`
	GroupName   string `json:"group_name"`
	UserLUID    string `json:"user_luid"`
	Username    string `json:"username,omitempty"`
	NoOp        bool   `json:"no_op"`
}
type MembershipResult struct {
	Status           string `json:"status"`
	GroupLUID        string `json:"group_luid"`
	UserLUID         string `json:"user_luid"`
	Membership       string `json:"membership"`
	Evidence         string `json:"evidence"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type MembershipOutput struct {
	Plan   MembershipPlan    `json:"plan"`
	Result *MembershipResult `json:"result,omitempty"`
	Help   []string          `json:"help"`
}

func (o MembershipOutput) CompactOutput() any { return o }
func (o MembershipOutput) FullOutput() any    { return o }

type MembershipResolver interface {
	ResolveMembershipGroup(context.Context, string) (MembershipGroup, error)
	ResolveUsername(context.Context, string) (MembershipMember, error)
}

func containsMembership(items []MembershipMember, luid string) bool {
	return slices.ContainsFunc(items, func(item MembershipMember) bool { return item.LUID == luid })
}
func validateMembershipInput(in MembershipInput) string {
	if strings.TrimSpace(in.Environment) == "" || strings.TrimSpace(in.GroupLUID) == "" {
		return "--environment and --group-id are required"
	}
	if (in.UserLUID == "") == (in.Username == "") {
		return "provide exactly one of --user-id or --username"
	}
	if strings.TrimSpace(in.UserLUID) != in.UserLUID || strings.TrimSpace(in.Username) != in.Username {
		return "user selectors must not contain surrounding whitespace"
	}
	return ""
}
