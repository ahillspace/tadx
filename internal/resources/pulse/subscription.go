package pulse

import (
	"context"

	subscriptionlist "github.com/ahillspace/tadx/actions/pulse/subscription"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

// SubscriptionClient is the native read contract used for subscription enrichment.
type SubscriptionClient interface {
	ListUserSubscriptions(context.Context, string, tableaupulse.PageRequest) (tableaupulse.SubscriptionPage, error)
	BatchGetMetrics(context.Context, []string) ([]tableaupulse.Metric, error)
	BatchGetDefinitions(context.Context, []string) ([]tableaupulse.Definition, error)
}

// SubscriptionReader translates native responses into the subscription action's observations.
type SubscriptionReader struct{ client SubscriptionClient }

func NewSubscriptionReader(client SubscriptionClient) *SubscriptionReader {
	return &SubscriptionReader{client: client}
}

var _ subscriptionlist.Reader = (*SubscriptionReader)(nil)

func (a SubscriptionReader) ListUserSubscriptions(ctx context.Context, userLUID string, request subscriptionlist.PageRequest) (subscriptionlist.Page, error) {
	page, err := a.client.ListUserSubscriptions(ctx, userLUID, tableaupulse.PageRequest{PageSize: request.PageSize, PageToken: request.PageToken})
	if err != nil {
		return subscriptionlist.Page{}, err
	}
	items := make([]subscriptionlist.Subscription, len(page.Subscriptions))
	for i, item := range page.Subscriptions {
		items[i] = subscriptionlist.Subscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID}
	}
	return subscriptionlist.Page{Subscriptions: items, NextPageToken: page.NextPageToken, RequestID: page.TableauRequestID}, nil
}

func (a SubscriptionReader) GetMetrics(ctx context.Context, ids []string) ([]subscriptionlist.Metric, error) {
	batch, err := a.client.BatchGetMetrics(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]subscriptionlist.Metric, len(batch))
	for i, item := range batch {
		items[i] = subscriptionlist.Metric{LUID: item.LUID, Name: item.Name, DefinitionLUID: item.DefinitionLUID, Specification: item.Specification}
	}
	return items, nil
}

func (a SubscriptionReader) GetDefinitions(ctx context.Context, ids []string) ([]subscriptionlist.Definition, error) {
	batch, err := a.client.BatchGetDefinitions(ctx, ids)
	if err != nil {
		return nil, err
	}
	items := make([]subscriptionlist.Definition, len(batch))
	for i, item := range batch {
		items[i] = subscriptionlist.Definition{LUID: item.LUID, Name: item.Name}
	}
	return items, nil
}
