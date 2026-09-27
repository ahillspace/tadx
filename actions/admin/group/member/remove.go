package member

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type RemoveWriter interface {
	RemoveGroupUser(context.Context, string, string) (Result, error)
}

func removePlan(ctx context.Context, resolver Resolver, in Input) (Plan, error) {
	if in.Username != "" {
		user, err := resolver.ResolveUsername(ctx, in.Username)
		if err != nil {
			return Plan{}, removeOperationError("user.resolve", in, err)
		}
		in.UserLUID = user.LUID
	}
	group, err := resolver.ResolveGroup(ctx, in.GroupLUID)
	if err != nil {
		return Plan{}, removeOperationError("resolve", in, err)
	}
	return Plan{Mode: "preview", Operation: "admin.group.member.remove", Environment: in.Environment, Site: in.Site, GroupLUID: group.LUID, GroupName: group.Name, UserLUID: in.UserLUID, Username: in.Username, NoOp: !contains(group.Members, in.UserLUID)}, nil
}
func removeApply(ctx context.Context, resolver Resolver, writer RemoveWriter, plan Plan) (Result, error) {
	current, err := resolver.ResolveGroup(ctx, plan.GroupLUID)
	if err != nil {
		return Result{}, removeOperationError("revalidate", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if !contains(current.Members, plan.UserLUID) {
		return Result{Status: "unchanged", GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID, Membership: "absent", Evidence: "prewrite_read"}, nil
	}
	result, err := writer.RemoveGroupUser(ctx, plan.GroupLUID, plan.UserLUID)
	if err != nil {
		return Result{}, removeOperationError("apply", Input{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	result.Membership, result.Evidence = "absent", "mutation_response"
	return result, nil
}
func Remove(ctx context.Context, resolver Resolver, writer RemoveWriter, in Input, preview bool) (Output, error) {
	plan, err := removePlan(ctx, resolver, in)
	if err != nil {
		return Output{}, err
	}
	out := Output{Plan: plan, Help: []string{"Run without --preview to remove this exact group member."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	result, err := removeApply(ctx, resolver, writer, plan)
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
