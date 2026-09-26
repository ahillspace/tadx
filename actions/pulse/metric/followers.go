package metric

import (
	"context"
	"errors"
	"strings"

	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/pulsecontract"
	"github.com/ahillspace/tadx/internal/readsource"
)

const followersMaxFollowers = 1000

type FollowersReader interface {
	GetMetric(context.Context, string) (Metric, error)
	ListSubscriptions(context.Context, string) ([]Subscription, error)
}

func Followers(ctx context.Context, reader FollowersReader, input FollowersInput) (FollowersOutput, error) {
	if err := FollowersValidateInput(input); err != nil {
		return FollowersOutput{}, err
	}
	if reader == nil {
		return FollowersOutput{}, followersFail("pulse.metric.followers.unconfigured", errs.KindRuntime, input, "Pulse metric follower listing is not configured.", nil)
	}
	input.MetricLUID = strings.TrimSpace(input.MetricLUID)
	metric, err := reader.GetMetric(ctx, input.MetricLUID)
	if err != nil {
		return FollowersOutput{}, followersReadError(input, err)
	}
	if metric.LUID != input.MetricLUID {
		return FollowersOutput{}, followersFail("pulse.metric.followers.invalid_response", errs.KindOperation, input, "Tableau returned a mismatched Pulse metric.", errors.New("metric identity mismatch"))
	}
	items, err := reader.ListSubscriptions(ctx, input.MetricLUID)
	if err != nil {
		return FollowersOutput{}, followersReadError(input, err)
	}
	if items == nil {
		items = []Subscription{}
	}
	if len(items) > followersMaxFollowers {
		return FollowersOutput{}, followersFail("pulse.metric.followers.invalid_response", errs.KindOperation, input, "Pulse metric follower listing exceeded its bounded output.", errors.New("more than 1000 subscriptions"))
	}
	requestID := metric.RequestID
	seen := map[string]bool{}
	for _, item := range items {
		if item.LUID == "" || seen[item.LUID] || item.MetricLUID != input.MetricLUID || item.FollowerLUID == "" || (item.FollowerType != "USER" && item.FollowerType != "GROUP") {
			return FollowersOutput{}, followersFail("pulse.metric.followers.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete or mismatched subscription.", errors.New("subscription identity mismatch"))
		}
		seen[item.LUID] = true
		if requestID == "" {
			requestID = item.RequestID
		}
	}
	return FollowersOutput{Status: "listed", Environment: input.Environment, Site: input.Site, MetricLUID: input.MetricLUID, Count: len(items), Subscriptions: items, RequestID: requestID, Help: []string{commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", input.MetricLUID)}}, nil
}

func followersReadError(input FollowersInput, err error) error {
	var structured *errs.Error
	if input.Cache && errors.As(err, &structured) {
		return err
	}
	retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact metric LUID and selected site, then retry.")
	return &errs.Error{ID: "pulse.metric.followers.failed", Kind: errs.KindOperation, Operation: "pulse.metric.followers", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric follower listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
}

func followersFail(id string, kind errs.Kind, input FollowersInput, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.followers", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact Pulse metric LUID and review its subscriptions."}
}

type FollowersInput struct {
	Environment string
	Site        string
	MetricLUID  string
	Cache       bool
}
type FollowersOutput struct {
	Status        string               `json:"status"`
	Environment   string               `json:"environment,omitempty"`
	Site          string               `json:"site,omitempty"`
	MetricLUID    string               `json:"metric_luid"`
	Count         int                  `json:"count"`
	Warnings      []string             `json:"warnings,omitempty"`
	Subscriptions []Subscription       `json:"subscriptions"`
	RequestID     string               `json:"tableau_request_id,omitempty"`
	Help          []string             `json:"help"`
	Source        *readsource.Metadata `json:"source,omitempty"`
}

type FollowersCompactSubscription struct {
	LUID         string `json:"luid"`
	MetricLUID   string `json:"metric_luid"`
	FollowerType string `json:"follower_type"`
	FollowerLUID string `json:"follower_luid"`
	FollowerName string `json:"follower_name"`
}

type FollowersCompactResult struct {
	Status        string                         `json:"status"`
	Environment   string                         `json:"environment,omitempty"`
	Site          string                         `json:"site,omitempty"`
	MetricLUID    string                         `json:"metric_luid"`
	Count         int                            `json:"count"`
	Warnings      []string                       `json:"warnings,omitempty"`
	Subscriptions []FollowersCompactSubscription `json:"subscriptions"`
	Help          []string                       `json:"help"`
	Source        *readsource.Metadata           `json:"source,omitempty"`
}

func (o FollowersOutput) CompactOutput() any {
	items := make([]FollowersCompactSubscription, len(o.Subscriptions))
	for i, item := range o.Subscriptions {
		items[i] = FollowersCompactSubscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID, FollowerName: item.FollowerName}
	}
	return FollowersCompactResult{Status: o.Status, Environment: o.Environment, Site: o.Site, MetricLUID: o.MetricLUID, Count: o.Count, Warnings: o.Warnings, Subscriptions: items, Help: o.Help, Source: o.Source}
}
func (o FollowersOutput) FullOutput() any { return o }

// ValidateInput checks local selectors without resolving a site or contacting Tableau.
func FollowersValidateInput(input FollowersInput) error {
	if strings.TrimSpace(input.MetricLUID) == "" {
		return followersFail("pulse.metric.followers.usage", errs.KindUsage, input, "Pulse metric followers requires an exact LUID.", nil)
	}
	if err := pulsecontract.ValidateLUIDShape("metric", input.MetricLUID); err != nil {
		return followersFail("pulse.metric.followers.usage", errs.KindUsage, input, "Pulse metric followers requires a well-formed exact metric LUID.", err)
	}
	return nil
}
