package group

import (
	"context"
	"errors"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
)

type MembershipRemoveWriter interface {
	RemoveMembership(context.Context, string, string) (MembershipResult, error)
}

func removePlan(ctx context.Context, resolver MembershipResolver, in MembershipInput) (MembershipPlan, error) {
	if in.Username != "" {
		user, err := resolver.ResolveUsername(ctx, in.Username)
		if err != nil {
			return MembershipPlan{}, removeOperationError("user.resolve", in, err)
		}
		in.UserLUID = user.LUID
	}
	group, err := resolver.ResolveMembershipGroup(ctx, in.GroupLUID)
	if err != nil {
		return MembershipPlan{}, removeOperationError("resolve", in, err)
	}
	return MembershipPlan{Mode: "preview", Operation: "admin.group.member.remove", Environment: in.Environment, Site: in.Site, GroupLUID: group.LUID, GroupName: group.Name, UserLUID: in.UserLUID, Username: in.Username, NoOp: !containsMembership(group.Members, in.UserLUID)}, nil
}
func removeApply(ctx context.Context, resolver MembershipResolver, writer MembershipRemoveWriter, plan MembershipPlan) (MembershipResult, error) {
	current, err := resolver.ResolveMembershipGroup(ctx, plan.GroupLUID)
	if err != nil {
		return MembershipResult{}, removeOperationError("revalidate", MembershipInput{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	if !containsMembership(current.Members, plan.UserLUID) {
		return MembershipResult{Status: "unchanged", GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID, Membership: "absent", Evidence: "prewrite_read"}, nil
	}
	result, err := writer.RemoveMembership(ctx, plan.GroupLUID, plan.UserLUID)
	if err != nil {
		return MembershipResult{}, removeOperationError("apply", MembershipInput{Environment: plan.Environment, Site: plan.Site, GroupLUID: plan.GroupLUID, UserLUID: plan.UserLUID}, err)
	}
	result.Membership, result.Evidence = "absent", "mutation_response"
	return result, nil
}
func RemoveMember(ctx context.Context, resolver MembershipResolver, writer MembershipRemoveWriter, in MembershipInput, preview bool) (MembershipOutput, error) {
	plan, err := removePlan(ctx, resolver, in)
	if err != nil {
		return MembershipOutput{}, err
	}
	out := MembershipOutput{Plan: plan, Help: []string{"Run without --preview to remove this exact group member."}}
	if preview {
		return out, nil
	}
	out.Plan.Mode = "execute"
	result, err := removeApply(ctx, resolver, writer, plan)
	if err != nil {
		return MembershipOutput{}, err
	}
	out.Result = &result
	out.Help = []string{commandhint.Environment(plan.Environment, "admin", "group", "inspect", "--id", plan.GroupLUID, "--members")}
	return out, nil
}
func removeUsage(message string) error {
	return &errs.Error{ID: "admin.group.member.remove.usage", Kind: errs.KindUsage, Operation: "admin.group.member.remove", Summary: message, Cause: errors.New(message), Retryable: errs.Bool(false), CorrectiveAction: "Provide an explicit environment, group LUID, and user LUID."}
}
func removeOperationError(stage string, in MembershipInput, cause error) error {
	retry, advice := errs.CompleteRetryAdvice(cause, "Re-read the exact group membership, then retry.")
	return &errs.Error{ID: "admin.group.member.remove." + stage, Kind: errs.KindOperation, Operation: "admin.group.member.remove", Environment: in.Environment, Site: in.Site, Resource: in.GroupLUID, Summary: "Group member remove failed.", Cause: cause, Retryable: retry, CorrectiveAction: advice, TableauRequestID: errs.TableauRequestID(cause)}
}
func ValidateRemoveMemberInput(in MembershipInput) error {
	if message := validateMembershipInput(in); message != "" {
		return removeUsage(message)
	}
	return nil
}
