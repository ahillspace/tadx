// Package remove implements admin.group.member.remove.
package remove

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
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
	planned     bool
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
}

// UsernameResolver resolves only an exact site username to an authoritative user.
type UsernameResolver interface {
	ResolveUsername(context.Context, string) (Member, error)
}
type Writer interface {
	RemoveGroupUser(context.Context, string, string) (Result, error)
}
type Action struct {
	resolver Resolver
	writer   Writer
}

func New(resolver Resolver, writer Writer) *Action {
	return &Action{resolver: resolver, writer: writer}
}

func (a *Action) Plan(ctx context.Context, in Input) (Plan, error) {
	if err := ValidateInput(in); err != nil {
		return Plan{}, err
	}
	if a == nil || a.resolver == nil || a.writer == nil {
		return Plan{}, runtimeError("incremental group membership is not configured")
	}
	if in.Username != "" {
		resolver, ok := a.resolver.(UsernameResolver)
		if !ok {
			return Plan{}, runtimeError("exact username resolution is not configured")
		}
		user, err := resolver.ResolveUsername(ctx, in.Username)
		if err != nil {
			return Plan{}, operationError("user.resolve", in, err)
		}
		if strings.TrimSpace(user.LUID) == "" || user.Name != in.Username {
			return Plan{}, runtimeError("username resolution returned an inconsistent authoritative identity")
		}
		in.UserLUID = user.LUID
	}
	group, err := a.resolver.ResolveGroup(ctx, in.GroupLUID)
	if err != nil {
		return Plan{}, operationError("resolve", in, err)
	}
	if err := validateGroup(group, in.GroupLUID); err != nil {
		return Plan{}, runtimeError(err.Error())
	}
	return Plan{Mode: "preview", Operation: "admin.group.member.remove", Environment: in.Environment, Site: in.Site, GroupLUID: group.LUID, GroupName: group.Name, UserLUID: in.UserLUID, Username: in.Username, NoOp: !contains(group.Members, in.UserLUID), planned: true}, nil
}
func (a *Action) Apply(ctx context.Context, plan Plan) (Result, error) {
	if a == nil || a.resolver == nil || a.writer == nil || !plan.planned || plan.Operation != "admin.group.member.remove" {
		return Result{}, usage("group member remove requires a plan produced by Plan")
	}
	current, err := a.resolver.ResolveGroup(ctx, plan.GroupLUID)
	if err != nil {
		return Result{}, operationError("revalidate", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if err := validateGroup(current, plan.GroupLUID); err != nil {
		return Result{}, runtimeError(err.Error())
	}
	if !contains(current.Members, plan.UserLUID) {
		return Result{Status: "unchanged", GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID, Membership: "absent", Evidence: "prewrite_read"}, nil
	}
	result, err := a.writer.RemoveGroupUser(ctx, plan.GroupLUID, plan.UserLUID)
	if err != nil {
		return Result{}, operationError("apply", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if result.GroupLUID != "" && result.GroupLUID != plan.GroupLUID {
		return Result{}, runtimeError("group member remove returned a mismatched group identity")
	}
	if result.UserLUID != "" && result.UserLUID != plan.UserLUID {
		return Result{}, runtimeError("group member remove returned a mismatched user identity")
	}
	result.GroupLUID = plan.GroupLUID
	result.UserLUID = plan.UserLUID
	if result.Status == "" {
		result.Status = "removed"
	}
	if result.Status != "removed" && result.Status != "deleted" {
		return Result{}, runtimeError("group member remove returned an unconfirmed outcome")
	}
	result.Membership, result.Evidence = "absent", "mutation_response"
	return result, nil
}
func (a *Action) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	plan, err := a.Plan(ctx, in)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: plan, Help: []string{"Run without --preview to remove this exact group member."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	result, err := a.Apply(ctx, plan)
	if err != nil {
		return Output{}, err
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(plan.Environment, "admin", "group", "inspect", "--id", plan.GroupLUID, "--members")}
	return out, nil
}
func validateGroup(group Group, expected string) error {
	if group.LUID != expected || strings.TrimSpace(group.Name) == "" {
		return errors.New("group resolution returned an inconsistent authoritative identity")
	}
	seen := map[string]bool{}
	for _, m := range group.Members {
		if strings.TrimSpace(m.LUID) == "" || seen[m.LUID] {
			return errors.New("group membership returned an incomplete or duplicate identity")
		}
		seen[m.LUID] = true
	}
	return nil
}
func contains(items []Member, luid string) bool {
	for _, item := range items {
		if item.LUID == luid {
			return true
		}
	}
	return false
}
func usage(message string) error {
	return &errs.Error{ID: "admin.group.member.remove.usage", Kind: errs.KindUsage, Operation: "admin.group.member.remove", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide an explicit environment, group LUID, and user LUID."}
}
func runtimeError(message string) error {
	return &errs.Error{ID: "admin.group.member.remove.runtime", Kind: errs.KindRuntime, Operation: "admin.group.member.remove", Summary: message, Retryable: errs.Bool(false)}
}
func operationError(stage string, in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Re-read the exact group membership, then retry.")
	return &errs.Error{ID: "admin.group.member.remove." + stage, Kind: errs.KindOperation, Operation: "admin.group.member.remove", Environment: in.Environment, Site: in.Site, Resource: in.GroupLUID, Summary: "Group member remove failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
