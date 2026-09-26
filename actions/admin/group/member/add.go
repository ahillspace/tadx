package member

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type AddWriter interface {
	AddGroupUser(context.Context, string, string) (Result, error)
}
type AddAction struct {
	resolver Resolver
	writer   AddWriter
}

func NewAdd(resolver Resolver, writer AddWriter) *AddAction {
	return &AddAction{resolver: resolver, writer: writer}
}

func (a *AddAction) Plan(ctx context.Context, in Input) (Plan, error) {
	if err := ValidateAddInput(in); err != nil {
		return Plan{}, err
	}
	if a == nil || a.resolver == nil || a.writer == nil {
		return Plan{}, addRuntimeError("incremental group membership is not configured")
	}
	if in.Username != "" {
		resolver, ok := a.resolver.(UsernameResolver)
		if !ok {
			return Plan{}, addRuntimeError("exact username resolution is not configured")
		}
		user, err := resolver.ResolveUsername(ctx, in.Username)
		if err != nil {
			return Plan{}, addOperationError("user.resolve", in, err)
		}
		if strings.TrimSpace(user.LUID) == "" || user.Name != in.Username {
			return Plan{}, addRuntimeError("username resolution returned an inconsistent authoritative identity")
		}
		in.UserLUID = user.LUID
	}
	group, err := a.resolver.ResolveGroup(ctx, in.GroupLUID)
	if err != nil {
		return Plan{}, addOperationError("resolve", in, err)
	}
	if err := validateGroup(group, in.GroupLUID); err != nil {
		return Plan{}, addRuntimeError(err.Error())
	}
	return Plan{Mode: "preview", Operation: "admin.group.member.add", Environment: in.Environment, Site: in.Site, GroupLUID: group.LUID, GroupName: group.Name, UserLUID: in.UserLUID, Username: in.Username, NoOp: contains(group.Members, in.UserLUID), planned: true}, nil
}
func (a *AddAction) Apply(ctx context.Context, plan Plan) (Result, error) {
	if a == nil || a.resolver == nil || a.writer == nil || !plan.planned || plan.Operation != "admin.group.member.add" {
		return Result{}, addUsage("group member add requires a plan produced by Plan")
	}
	current, err := a.resolver.ResolveGroup(ctx, plan.GroupLUID)
	if err != nil {
		return Result{}, addOperationError("revalidate", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if err := validateGroup(current, plan.GroupLUID); err != nil {
		return Result{}, addRuntimeError(err.Error())
	}
	if contains(current.Members, plan.UserLUID) {
		return Result{Status: "unchanged", GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID, Membership: "present", Evidence: "prewrite_read"}, nil
	}
	result, err := a.writer.AddGroupUser(ctx, plan.GroupLUID, plan.UserLUID)
	if err != nil {
		return Result{}, addOperationError("apply", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if result.GroupLUID != "" && result.GroupLUID != plan.GroupLUID {
		return Result{}, addRuntimeError("group member add returned a mismatched group identity")
	}
	if result.UserLUID != "" && result.UserLUID != plan.UserLUID {
		return Result{}, addRuntimeError("group member add returned a mismatched user identity")
	}
	result.GroupLUID = plan.GroupLUID
	result.UserLUID = plan.UserLUID
	if result.Status == "" {
		result.Status = "added"
	}
	if result.Status != "added" {
		return Result{}, addRuntimeError("group member add returned an unconfirmed outcome")
	}
	result.Membership, result.Evidence = "present", "mutation_response"
	return result, nil
}
func (a *AddAction) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
	plan, err := a.Plan(ctx, in)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: plan, Help: []string{"Run without --preview to add this exact group member."}}
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
func addUsage(message string) error {
	return &errs.Error{ID: "admin.group.member.add.usage", Kind: errs.KindUsage, Operation: "admin.group.member.add", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide an explicit environment, group LUID, and user LUID."}
}
func addRuntimeError(message string) error {
	return &errs.Error{ID: "admin.group.member.add.runtime", Kind: errs.KindRuntime, Operation: "admin.group.member.add", Summary: message, Retryable: errs.Bool(false)}
}
func addOperationError(stage string, in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Re-read the exact group membership, then retry.")
	return &errs.Error{ID: "admin.group.member.add." + stage, Kind: errs.KindOperation, Operation: "admin.group.member.add", Environment: in.Environment, Site: in.Site, Resource: in.GroupLUID, Summary: "Group member add failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
func ValidateAddInput(in Input) error {
	if message := validateInput(in); message != "" {
		return addUsage(message)
	}
	return nil
}
