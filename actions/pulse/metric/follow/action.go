package follow

import (
	"context"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

type Creator interface {
	CreateSubscription(context.Context, CreateRequest) (CreateResult, error)
}
type Action struct{ creator Creator }

func New(creator Creator) *Action { return &Action{creator: creator} }
func (a *Action) Execute(ctx context.Context, input Input, apply bool) (Output, error) {
	if a == nil || a.creator == nil {
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
	output := Output{Plan: Plan{Mode: "preview", Operation: "pulse.metric.follow", Environment: input.Environment, Site: input.Site, MetricLUID: input.MetricLUID, FollowerType: typeName, FollowerLUID: luid}, Help: []string{"Add --apply to follow this exact Pulse metric."}}
	if !apply {
		return output, nil
	}
	result, err := a.creator.CreateSubscription(ctx, CreateRequest{MetricLUID: input.MetricLUID, FollowerType: typeName, FollowerLUID: luid})
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect the remote follow outcome before retrying.")
		return Output{}, &errs.Error{ID: "pulse.metric.follow.failed", Kind: errs.KindOperation, Operation: "pulse.metric.follow", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric follow failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
	}
	if result.Status != "followed" && result.Status != "already_following" {
		return Output{}, fail("pulse.metric.follow.invalid_response", errs.KindOperation, input, "Tableau returned an unknown Pulse follow status.", nil)
	}
	output.Applied = true
	output.Result = &result
	output.Help = []string{"tadx pulse metric followers --id " + input.MetricLUID}
	return output, nil
}
func fail(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.follow", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact metric and one exact user or group LUID, then review a new preview."}
}
