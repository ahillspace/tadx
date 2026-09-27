package metric

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
	"github.com/ahillspace/tadx/internal/value"
)

type FollowCreator interface {
	CreateSubscription(context.Context, FollowCreateRequest) (FollowCreateResult, error)
}

func Follow(ctx context.Context, resolver FollowResolver, creator FollowCreator, input FollowInput, preview bool) (FollowOutput, error) {
	input.MetricLUID = strings.TrimSpace(input.MetricLUID)
	input.UserLUID = strings.TrimSpace(input.UserLUID)
	input.GroupLUID = strings.TrimSpace(input.GroupLUID)
	typeName, luid := "USER", input.UserLUID
	if input.GroupLUID != "" {
		typeName, luid = "GROUP", input.GroupLUID
	}
	metric, err := resolver.ResolveMetric(ctx, input.MetricLUID)
	if err != nil {
		return FollowOutput{}, followResolveFailure("metric", input, err)
	}
	if metric.LUID != input.MetricLUID {
		return FollowOutput{}, followResolveFailure("metric", input, errors.New("metric resolution returned an inconsistent authoritative identity"))
	}
	if typeName == "USER" {
		user, err := resolver.ResolveUser(ctx, luid)
		if err != nil {
			return FollowOutput{}, followResolveFailure("user", input, err)
		}
		if user.LUID != luid {
			return FollowOutput{}, followResolveFailure("user", input, errors.New("user resolution returned an inconsistent authoritative identity"))
		}
	} else {
		group, err := resolver.ResolveGroup(ctx, luid)
		if err != nil {
			return FollowOutput{}, followResolveFailure("group", input, err)
		}
		if group.LUID != luid {
			return FollowOutput{}, followResolveFailure("group", input, errors.New("group resolution returned an inconsistent authoritative identity"))
		}
	}
	output := FollowOutput{Plan: FollowPlan{Mode: "preview", Operation: "pulse.metric.follow", Environment: input.Environment, Site: input.Site, MetricLUID: input.MetricLUID, FollowerType: typeName, FollowerLUID: luid}, Help: []string{"Run without --preview to follow this exact Pulse metric."}}
	if preview {
		return output, nil
	}
	output.Plan.Mode = "execute"
	result, err := creator.CreateSubscription(ctx, FollowCreateRequest{MetricLUID: input.MetricLUID, FollowerType: typeName, FollowerLUID: luid})
	if err != nil {
		retryable, corrective := errs.CompleteRetryAdvice(err, "Inspect the remote follow outcome before retrying.")
		return FollowOutput{}, &errs.Error{ID: "pulse.metric.follow.failed", Kind: errs.KindOperation, Operation: "pulse.metric.follow", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric follow failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err), Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	if result.Status != "followed" && result.Status != "already_following" {
		return FollowOutput{}, &errs.Error{ID: "pulse.metric.follow.invalid_response", Kind: errs.KindOperation, Operation: "pulse.metric.follow", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Tableau returned an unknown Pulse follow status.", Retryable: errs.Bool(false), CorrectiveAction: "Inspect the exact Pulse metric followers before retrying; do not replay an uncertain follow.", Phase: errs.PhaseSubmission, Outcome: errs.OutcomeUnknown}
	}
	output.Result = &result
	output.Help = []string{commandhint.Environment(input.Environment, "pulse", "metric", "followers", "--id", input.MetricLUID)}
	return output, nil
}
func followResolveFailure(target string, input FollowInput, cause error) error {
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
func followFail(id string, kind errs.Kind, input FollowInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.follow", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact metric and one exact user or group LUID, then review a new preview.", Phase: errs.PhaseValidation, Outcome: errs.OutcomeNotAttempted}
}

type FollowInput struct {
	Environment string
	Site        string
	MetricLUID  string
	UserLUID    string
	GroupLUID   string
}
type FollowCreateRequest = value.PulseSubscriptionRequest
type FollowCreateResult struct {
	Status           string `json:"status"`
	SubscriptionLUID string `json:"subscription_luid,omitempty"`
	RequestID        string `json:"tableau_request_id,omitempty"`
}

// Metric, User, and Group are the minimal authoritative identities required
// before a Pulse subscription can be previewed or created.
type FollowMetric struct{ LUID string }
type FollowUser struct{ LUID string }
type FollowGroup struct{ LUID string }

// Resolver performs exact, type-specific live identity checks.
// Implementations must not substitute or enumerate principals.
type FollowResolver interface {
	ResolveMetric(context.Context, string) (FollowMetric, error)
	ResolveUser(context.Context, string) (FollowUser, error)
	ResolveGroup(context.Context, string) (FollowGroup, error)
}
type FollowPlan struct {
	Mode         string `json:"mode"`
	Operation    string `json:"operation"`
	Environment  string `json:"environment,omitempty"`
	Site         string `json:"site,omitempty"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type"`
	FollowerLUID string `json:"follower_luid"`
}
type FollowOutput struct {
	Plan   FollowPlan          `json:"plan"`
	Result *FollowCreateResult `json:"result,omitempty"`
	Help   []string            `json:"help"`
}

func (o FollowOutput) CompactOutput() any {
	value := o
	if value.Result != nil {
		copy := *value.Result
		copy.RequestID = ""
		value.Result = &copy
	}
	return value
}
func (o FollowOutput) FullOutput() any { return o }

// ValidateInput checks exact metric/follower selection before authentication.
func FollowValidateInput(input FollowInput) error {
	if strings.TrimSpace(input.MetricLUID) == "" || (strings.TrimSpace(input.UserLUID) == "") == (strings.TrimSpace(input.GroupLUID) == "") {
		return followFail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires an exact metric and exactly one user or group LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
		return followFail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires a well-formed exact metric LUID.", err)
	}
	if strings.TrimSpace(input.UserLUID) != "" {
		if err := pulsecontract.ValidateLUIDShape("user", input.UserLUID); err != nil {
			return followFail("pulse.metric.follow.usage", errs.KindUsage, input, "Pulse metric follow requires a well-formed exact user LUID.", err)
		}
	}
	return nil
}
