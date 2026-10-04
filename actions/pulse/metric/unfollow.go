package metric

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
)

type UnfollowReader interface {
	ListSubscriptions(context.Context, string) ([]Subscription, error)
}
type UnfollowDeleter interface {
	DeleteSubscription(context.Context, string) error
}

func runUnfollow(ctx context.Context, reader UnfollowReader, deleter UnfollowDeleter, input UnfollowInput, preview bool) (UnfollowOutput, error) {
	input.SubscriptionLUID = strings.TrimSpace(input.SubscriptionLUID)
	input.MetricLUID = strings.TrimSpace(input.MetricLUID)
	input.UserLUID = strings.TrimSpace(input.UserLUID)
	input.GroupLUID = strings.TrimSpace(input.GroupLUID)
	relation := input.SubscriptionLUID == "" && input.MetricLUID != "" && (input.UserLUID != "") != (input.GroupLUID != "")
	plan := UnfollowPlan{Mode: "preview", Operation: "pulse.metric.unfollow", Environment: input.Environment, Site: input.Site, SubscriptionLUID: input.SubscriptionLUID}
	if relation {
		sub, err := unfollowResolve(ctx, reader, input)
		if err != nil {
			return UnfollowOutput{}, err
		}
		plan.SubscriptionLUID = sub.LUID
		plan.MetricLUID = input.MetricLUID
		plan.FollowerType = sub.FollowerType
		plan.FollowerLUID = sub.FollowerLUID
	}
	output := UnfollowOutput{Plan: plan, Help: []string{"Run without --preview to remove this exact Pulse subscription."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	if relation {
		current, err := unfollowResolve(ctx, reader, input)
		if err != nil {
			return UnfollowOutput{}, err
		}
		if current.LUID != plan.SubscriptionLUID {
			return UnfollowOutput{}, unfollowFail("pulse.metric.unfollow.changed", errs.KindOperation, input, "The Pulse subscription identity changed during revalidation.", errors.New("subscription LUID changed"))
		}
	}
	if err := deleter.DeleteSubscription(ctx, plan.SubscriptionLUID); err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect the remote unfollow outcome before retrying.")
		return UnfollowOutput{}, &errs.Error{ID: "pulse.metric.unfollow.failed", Kind: errs.KindOperation, Operation: "pulse.metric.unfollow", Resource: plan.SubscriptionLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric unfollow failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	output.Result = &UnfollowResult{Status: "unfollowed", SubscriptionLUID: plan.SubscriptionLUID}
	if plan.MetricLUID != "" {
		output.Help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "followers", "--id", plan.MetricLUID)}
	} else {
		output.Help = nil
	}
	return output, nil
}
func unfollowResolve(ctx context.Context, reader UnfollowReader, input UnfollowInput) (Subscription, error) {
	items, err := reader.ListSubscriptions(ctx, input.MetricLUID)
	if err != nil {
		return Subscription{}, unfollowFail("pulse.metric.unfollow.resolve", errs.KindOperation, input, "Pulse subscription resolution failed.", err)
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
		return Subscription{}, unfollowFail("pulse.metric.unfollow.resolve", errs.KindOperation, input, "Pulse subscription resolution requires exactly one match.", errors.New("zero or multiple subscriptions matched"))
	}
	if matches[0].LUID == "" {
		return Subscription{}, unfollowFail("pulse.metric.unfollow.invalid_response", errs.KindOperation, input, "Tableau returned a subscription without an exact LUID.", nil)
	}
	return matches[0], nil
}
func unfollowFail(id string, kind errs.Kind, input UnfollowInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.unfollow", Resource: unfollowFirst(input.SubscriptionLUID, input.MetricLUID), Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact subscription or an exact metric and follower pair, then review a new preview.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}
func unfollowFirst(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

type UnfollowInput struct {
	Environment      string
	Site             string
	SubscriptionLUID string
	MetricLUID       string
	UserLUID         string
	GroupLUID        string
}
type UnfollowPlan struct {
	Mode             string `json:"mode"`
	Operation        string `json:"operation"`
	Environment      string `json:"environment,omitempty"`
	Site             string `json:"site,omitempty"`
	SubscriptionLUID string `json:"subscription_luid"`
	MetricLUID       string `json:"metric_luid,omitempty"`
	FollowerType     string `json:"follower_type,omitempty"`
	FollowerLUID     string `json:"follower_luid,omitempty"`
}
type UnfollowResult struct {
	Status           string `json:"status"`
	SubscriptionLUID string `json:"subscription_luid"`
}
type UnfollowOutput struct {
	Plan   UnfollowPlan    `json:"plan"`
	Result *UnfollowResult `json:"result,omitempty"`
	Help   []string        `json:"help,omitempty"`
}

func (o UnfollowOutput) CompactOutput() any { return o }
func (o UnfollowOutput) FullOutput() any    { return o }

// ValidateInput checks subscription-or-relationship selection without remote reads.
func unfollowValidateInput(input UnfollowInput) error {
	subscription, metric, user, group := strings.TrimSpace(input.SubscriptionLUID), strings.TrimSpace(input.MetricLUID), strings.TrimSpace(input.UserLUID), strings.TrimSpace(input.GroupLUID)
	direct := subscription != "" && metric == "" && user == "" && group == ""
	relation := subscription == "" && metric != "" && (user != "") != (group != "")
	if !direct && !relation {
		return unfollowFail("pulse.metric.unfollow.usage", errs.KindUsage, input, "Use either one exact subscription LUID or one exact metric and follower pair.", nil)
	}
	if relation {
		if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
			return unfollowFail("pulse.metric.unfollow.usage", errs.KindUsage, input, "Pulse metric unfollow requires a well-formed exact metric LUID.", err)
		}
		if user != "" {
			if err := pulsecontract.ValidateLUIDShape("user", input.UserLUID); err != nil {
				return unfollowFail("pulse.metric.unfollow.usage", errs.KindUsage, input, "Pulse metric unfollow requires a well-formed exact user LUID.", err)
			}
		}
	}
	return nil
}
