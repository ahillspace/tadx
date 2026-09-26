package member

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

type RemoveWriter interface {
	RemoveGroupUser(context.Context, string, string) (Result, error)
}
type RemoveAction struct {
	resolver Resolver
	writer   RemoveWriter
}

func NewRemove(resolver Resolver, writer RemoveWriter) *RemoveAction {
	return &RemoveAction{resolver: resolver, writer: writer}
}

func (a *RemoveAction) Plan(ctx context.Context, in Input) (Plan, error) {
	if err := ValidateRemoveInput(in); err != nil {
		return Plan{}, err
	}
	if a == nil || a.resolver == nil || a.writer == nil {
		return Plan{}, removeRuntimeError("incremental group membership is not configured")
	}
	if in.Username != "" {
		resolver, ok := a.resolver.(UsernameResolver)
		if !ok {
			return Plan{}, removeRuntimeError("exact username resolution is not configured")
		}
		user, err := resolver.ResolveUsername(ctx, in.Username)
		if err != nil {
			return Plan{}, removeOperationError("user.resolve", in, err)
		}
		if strings.TrimSpace(user.LUID) == "" || user.Name != in.Username {
			return Plan{}, removeRuntimeError("username resolution returned an inconsistent authoritative identity")
		}
		in.UserLUID = user.LUID
	}
	group, err := a.resolver.ResolveGroup(ctx, in.GroupLUID)
	if err != nil {
		return Plan{}, removeOperationError("resolve", in, err)
	}
	if err := validateGroup(group, in.GroupLUID); err != nil {
		return Plan{}, removeRuntimeError(err.Error())
	}
	return Plan{Mode: "preview", Operation: "admin.group.member.remove", Environment: in.Environment, Site: in.Site, GroupLUID: group.LUID, GroupName: group.Name, UserLUID: in.UserLUID, Username: in.Username, NoOp: !contains(group.Members, in.UserLUID), planned: true}, nil
}
func (a *RemoveAction) Apply(ctx context.Context, plan Plan) (Result, error) {
	if a == nil || a.resolver == nil || a.writer == nil || !plan.planned || plan.Operation != "admin.group.member.remove" {
		return Result{}, removeUsage("group member remove requires a plan produced by Plan")
	}
	current, err := a.resolver.ResolveGroup(ctx, plan.GroupLUID)
	if err != nil {
		return Result{}, removeOperationError("revalidate", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if err := validateGroup(current, plan.GroupLUID); err != nil {
		return Result{}, removeRuntimeError(err.Error())
	}
	if !contains(current.Members, plan.UserLUID) {
		return Result{Status: "unchanged", GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID, Membership: "absent", Evidence: "prewrite_read"}, nil
	}
	result, err := a.writer.RemoveGroupUser(ctx, plan.GroupLUID, plan.UserLUID)
	if err != nil {
		return Result{}, removeOperationError("apply", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if result.GroupLUID != "" && result.GroupLUID != plan.GroupLUID {
		return Result{}, removeRuntimeError("group member remove returned a mismatched group identity")
	}
	if result.UserLUID != "" && result.UserLUID != plan.UserLUID {
		return Result{}, removeRuntimeError("group member remove returned a mismatched user identity")
	}
	result.GroupLUID = plan.GroupLUID
	result.UserLUID = plan.UserLUID
	if result.Status == "" {
		result.Status = "removed"
	}
	if result.Status != "removed" && result.Status != "deleted" {
		return Result{}, removeRuntimeError("group member remove returned an unconfirmed outcome")
	}
	result.Membership, result.Evidence = "absent", "mutation_response"
	return result, nil
}
func (a *RemoveAction) Execute(ctx context.Context, in Input, preview bool) (Output, error) {
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
func removeUsage(message string) error {
	return &errs.Error{ID: "admin.group.member.remove.usage", Kind: errs.KindUsage, Operation: "admin.group.member.remove", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide an explicit environment, group LUID, and user LUID."}
}
func removeRuntimeError(message string) error {
	return &errs.Error{ID: "admin.group.member.remove.runtime", Kind: errs.KindRuntime, Operation: "admin.group.member.remove", Summary: message, Retryable: errs.Bool(false)}
}
func removeOperationError(stage string, in Input, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Re-read the exact group membership, then retry.")
	return &errs.Error{ID: "admin.group.member.remove." + stage, Kind: errs.KindOperation, Operation: "admin.group.member.remove", Environment: in.Environment, Site: in.Site, Resource: in.GroupLUID, Summary: "Group member remove failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
func ValidateRemoveInput(in Input) error {
	if message := validateInput(in); message != "" {
		return removeUsage(message)
	}
	return nil
}
