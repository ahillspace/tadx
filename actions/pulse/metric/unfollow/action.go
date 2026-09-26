package unfollow

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

type Reader interface {
	ListSubscriptions(context.Context, string) ([]Subscription, error)
}
type Deleter interface {
	DeleteSubscription(context.Context, string) error
}
type Action struct {
	reader  Reader
	deleter Deleter
}

func New(reader Reader, deleter Deleter) *Action { return &Action{reader: reader, deleter: deleter} }
func (a *Action) Execute(ctx context.Context, input Input, preview bool) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil || a.deleter == nil {
		return Output{}, fail("pulse.metric.unfollow.unconfigured", errs.KindRuntime, input, "Pulse metric unfollow is not configured.", nil)
	}
	trim(&input.SubscriptionLUID)
	trim(&input.MetricLUID)
	trim(&input.UserLUID)
	trim(&input.GroupLUID)
	relation := input.SubscriptionLUID == "" && input.MetricLUID != "" && (input.UserLUID != "") != (input.GroupLUID != "")
	plan := Plan{Mode: "preview", Operation: "pulse.metric.unfollow", Environment: input.Environment, Site: input.Site, SubscriptionLUID: input.SubscriptionLUID}
	if relation {
		sub, err := a.resolve(ctx, input)
		if err != nil {
			return Output{}, err
		}
		plan.SubscriptionLUID = sub.LUID
		plan.MetricLUID = input.MetricLUID
		plan.FollowerType = sub.FollowerType
		plan.FollowerLUID = sub.FollowerLUID
	}
	output := Output{Plan: plan, Help: []string{"Run without --preview to remove this exact Pulse subscription."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	if relation {
		current, err := a.resolve(ctx, input)
		if err != nil {
			return Output{}, err
		}
		if current.LUID != plan.SubscriptionLUID {
			return Output{}, fail("pulse.metric.unfollow.changed", errs.KindOperation, input, "The Pulse subscription identity changed during revalidation.", errors.New("subscription LUID changed"))
		}
	}
	if err := a.deleter.DeleteSubscription(ctx, plan.SubscriptionLUID); err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect the remote unfollow outcome before retrying.")
		return Output{}, &errs.Error{ID: "pulse.metric.unfollow.failed", Kind: errs.KindOperation, Operation: "pulse.metric.unfollow", Resource: plan.SubscriptionLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric unfollow failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	output.Result = &Result{Status: "unfollowed", SubscriptionLUID: plan.SubscriptionLUID}
	if plan.MetricLUID != "" {
		output.Help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "followers", "--id", plan.MetricLUID)}
	} else {
		output.Help = nil
	}
	return output, nil
}
func (a *Action) resolve(ctx context.Context, input Input) (Subscription, error) {
	items, err := a.reader.ListSubscriptions(ctx, input.MetricLUID)
	if err != nil {
		return Subscription{}, fail("pulse.metric.unfollow.resolve", errs.KindOperation, input, "Pulse subscription resolution failed.", err)
	}
	typeName, luid := "USER", input.UserLUID
	if input.GroupLUID != "" {
		typeName, luid = "GROUP", input.GroupLUID
	}
	matches := []Subscription{}
	for _, item := range items {
		if item.FollowerType == typeName && item.FollowerLUID == luid {
			matches = append(matches, item)
		}
	}
	if len(matches) != 1 {
		return Subscription{}, fail("pulse.metric.unfollow.resolve", errs.KindOperation, input, "Pulse subscription resolution requires exactly one match.", errors.New("zero or multiple subscriptions matched"))
	}
	if matches[0].LUID == "" {
		return Subscription{}, fail("pulse.metric.unfollow.invalid_response", errs.KindOperation, input, "Tableau returned a subscription without an exact LUID.", nil)
	}
	return matches[0], nil
}
func trim(value *string) { *value = strings.TrimSpace(*value) }
func fail(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.unfollow", Resource: first(input.SubscriptionLUID, input.MetricLUID), Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact subscription or an exact metric and follower pair, then review a new preview.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}
func first(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
