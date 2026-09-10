package follow

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

type Creator interface {
	CreateSubscription(context.Context, CreateRequest) (CreateResult, error)
}
type Action struct {
	resolver Resolver
	creator  Creator
}

func New(resolver Resolver, creator Creator) *Action {
	return &Action{resolver: resolver, creator: creator}
}
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.resolver == nil || a.creator == nil {
		return Output{}, fail("pulse.metric.follow.unconfigured", errs.KindRuntime, input, "Pulse metric follow is not configured.", nil)
	}
	input.MetricLUID = strings.TrimSpace(input.MetricLUID)
	input.UserLUID = strings.TrimSpace(input.UserLUID)
	input.GroupLUID = strings.TrimSpace(input.GroupLUID)
	if input.MetricLUID == "" || (input.UserLUID == "") == (input.GroupLUID == "") {
		return Output{}, fail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires an exact metric and exactly one user or group LUID.", nil)
	}
	typeName, luid := "USER", input.UserLUID
	if input.GroupLUID != "" {
		typeName, luid = "GROUP", input.GroupLUID
	}
	metric, err := a.resolver.ResolveMetric(ctx, input.MetricLUID)
	if err != nil {
		return Output{}, resolveFailure("metric", input, err)
	}
	if metric.LUID != input.MetricLUID {
		return Output{}, resolveFailure("metric", input, errors.New("metric resolution returned an inconsistent authoritative identity"))
	}
	if typeName == "USER" {
		user, err := a.resolver.ResolveUser(ctx, luid)
		if err != nil {
			return Output{}, resolveFailure("user", input, err)
		}
		if user.LUID != luid {
			return Output{}, resolveFailure("user", input, errors.New("user resolution returned an inconsistent authoritative identity"))
		}
	} else {
		group, err := a.resolver.ResolveGroup(ctx, luid)
		if err != nil {
			return Output{}, resolveFailure("group", input, err)
		}
		if group.LUID != luid {
			return Output{}, resolveFailure("group", input, errors.New("group resolution returned an inconsistent authoritative identity"))
		}
	}
	output := Output{Plan: Plan{Mode: "preview", Operation: "pulse.metric.follow", Environment: input.Environment, Site: input.Site, MetricLUID: input.MetricLUID, FollowerType: typeName, FollowerLUID: luid}, Help: []string{"Run without --preview to follow this exact Pulse metric."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	result, err := a.creator.CreateSubscription(ctx, CreateRequest{MetricLUID: input.MetricLUID, FollowerType: typeName, FollowerLUID: luid})
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect the remote follow outcome before retrying.")
		return Output{}, &errs.Error{ID: "pulse.metric.follow.failed", Kind: errs.KindOperation, Operation: "pulse.metric.follow", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric follow failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if result.Status != "followed" && result.Status != "already_following" {
		return Output{}, fail("pulse.metric.follow.invalid_response", errs.KindOperation, input, "Tableau returned an unknown Pulse follow status.", nil)
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "followers", "--id", input.MetricLUID)}
	return output, nil
}
func resolveFailure(target string, input Input, cause error) error {
	retryable, corrective := errs.CompleteRetryAdvice(cause, "Review the exact Pulse metric and selected follower identity, then retry.")
	resource, summary := input.MetricLUID, "The exact Pulse metric must exist."
	if target == "user" {
		resource, summary = input.UserLUID, "The exact follower user must exist."
	}
	if target == "group" {
		resource, summary = input.GroupLUID, "The exact follower group must exist."
	}
	return &errs.Error{ID: "pulse.metric.follow." + target + ".resolve", Kind: errs.KindOperation, Operation: "pulse.metric.follow", Resource: resource, Environment: input.Environment, Site: input.Site, Summary: "Pulse follow identity resolution failed.", Cause: cause, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(cause), Phase: errs.PhaseVerification, Outcome: errs.OutcomeNotAttempted, Prerequisite: &errs.Prerequisite{Kind: target, Resource: resource, Summary: summary}}
}
func fail(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.follow", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact metric and one exact user or group LUID, then review a new preview."}
}
