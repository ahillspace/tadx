package member

import (
	"context"
	"slices"
	"strings"
)

type Input struct {
	Environment string
	Site        string
	GroupLUID   string
	UserLUID    string
	Username    string
}
type Member struct {
	LUID string `json:"luid"`
	Name string `json:"name,omitempty"`
}
type Group struct {
	LUID    string   `json:"luid"`
	Name    string   `json:"name"`
	Members []Member `json:"members,omitempty"`
}
type Plan struct {
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
type Result struct {
	Status           string `json:"status"`
	GroupLUID        string `json:"group_luid"`
	UserLUID         string `json:"user_luid"`
	Membership       string `json:"membership"`
	Evidence         string `json:"evidence"`
	TableauRequestID string `json:"tableau_request_id,omitempty"`
}
type Output struct {
	Plan   Plan     `json:"plan"`
	Result *Result  `json:"result,omitempty"`
	Help   []string `json:"help"`
}

func (o Output) CompactOutput() any { return o }
func (o Output) FullOutput() any    { return o }

type Resolver interface {
	ResolveGroup(context.Context, string) (Group, error)
	ResolveUsername(context.Context, string) (Member, error)
}

func contains(items []Member, luid string) bool {
	return slices.ContainsFunc(items, func(item Member) bool { return item.LUID == luid })
}
func validateInput(in Input) string {
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
