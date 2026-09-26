package followers

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/commandhint"
	"github.com/ahillspace/tadx/internal/errs"
	"strings"
)

const maxFollowers = 1000

type Reader interface {
	GetMetric(context.Context, string) (Metric, error)
	ListSubscriptions(context.Context, string) ([]Subscription, error)
}
type Action struct{ reader Reader }

func New(reader Reader) *Action { return &Action{reader: reader} }
func (a *Action) Execute(ctx context.Context, input Input) (Output, error) {
	if err := ValidateInput(input); err != nil {
		return Output{}, err
	}
	if a == nil || a.reader == nil {
		return Output{}, fail("pulse.metric.followers.unconfigured", errs.KindRuntime, input, "Pulse metric follower listing is not configured.", nil)
	}
	input.MetricLUID = strings.TrimSpace(input.MetricLUID)
	metric, err := a.reader.GetMetric(ctx, input.MetricLUID)
	if err != nil {
		return Output{}, readError(input, err)
	}
	if metric.LUID != input.MetricLUID {
		return Output{}, fail("pulse.metric.followers.invalid_response", errs.KindOperation, input, "Tableau returned a mismatched Pulse metric.", errors.New("metric identity mismatch"))
	}
	items, err := a.reader.ListSubscriptions(ctx, input.MetricLUID)
	if err != nil {
		return Output{}, readError(input, err)
	}
	if items == nil {
		items = []Subscription{}
	}
	if len(items) > maxFollowers {
		return Output{}, fail("pulse.metric.followers.invalid_response", errs.KindOperation, input, "Pulse metric follower listing exceeded its bounded output.", errors.New("more than 1000 subscriptions"))
	}
	requestID := metric.RequestID
	seen := map[string]bool{}
	for _, item := range items {
		if item.LUID == "" || seen[item.LUID] || item.MetricLUID != input.MetricLUID || item.FollowerLUID == "" || (item.FollowerType != "USER" && item.FollowerType != "GROUP") {
			return Output{}, fail("pulse.metric.followers.invalid_response", errs.KindOperation, input, "Tableau returned an incomplete or mismatched subscription.", errors.New("subscription identity mismatch"))
		}
		seen[item.LUID] = true
		if requestID == "" {
			requestID = item.RequestID
		}
	}
	return Output{Status: "listed", Environment: input.Environment, Site: input.Site, MetricLUID: input.MetricLUID, Count: len(items), Subscriptions: items, RequestID: requestID, Help: []string{commandhint.Environment(input.Environment, "pulse", "metric", "inspect", "--id", input.MetricLUID)}}, nil
}

func readError(input Input, err error) error {
	var structured *errs.Error
	if input.Cache && errors.As(err, &structured) {
		return err
	}
	retryable, corrective := errs.CompleteRetryAdvice(err, "Review the exact metric LUID and selected site, then retry.")
	return &errs.Error{ID: "pulse.metric.followers.failed", Kind: errs.KindOperation, Operation: "pulse.metric.followers", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: "Pulse metric follower listing failed.", Cause: err, Retryable: retryable, CorrectiveAction: corrective, TableauRequestID: errs.TableauRequestID(err)}
}

func fail(id string, kind errs.Kind, input Input, summary string, cause error) error {
	return &errs.Error{ID: id, Kind: kind, Operation: "pulse.metric.followers", Resource: input.MetricLUID, Environment: input.Environment, Site: input.Site, Summary: summary, Cause: cause, Retryable: errs.Bool(false), CorrectiveAction: "Provide one exact Pulse metric LUID and review its subscriptions."}
}
