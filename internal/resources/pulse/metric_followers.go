package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	metric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/readsource"
	tableauadmin "github.com/ahillspace/tadx/internal/tableau/admin"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type MetricFollowerPort struct {
	*MetricInspectPort
	Client          *tableaupulse.Client
	CheckCapability func(string) error
	AdminClient     *tableauadmin.Client
	Store           *cache.Store
	Environment     string
	Site            string
	Now             func() time.Time
	items           []tableaupulse.Subscription
}

const followerSnapshotKind = "pulse_follower_snapshot"

type followerSnapshot struct {
	Version       int                   `json:"version"`
	MetricLUID    string                `json:"metric_luid"`
	Subscriptions []metric.Subscription `json:"subscriptions"`
}

func (a *MetricFollowerPort) Publish(ctx context.Context, metricLUID string) error {
	snapshot := followerSnapshot{Version: 1, MetricLUID: metricLUID, Subscriptions: make([]metric.Subscription, 0, len(a.items))}
	for _, item := range a.items {
		snapshot.Subscriptions = append(snapshot.Subscriptions, metric.Subscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID, FollowerName: item.FollowerName})
	}
	encoded, err := json.Marshal(snapshot)
	if err != nil {
		return err
	}
	entry := cache.ResourceEntry{Environment: a.Environment, Site: a.Site, Kind: followerSnapshotKind, LUID: metricLUID, Name: metricLUID, Coverage: "detail", ObservedAt: a.Now().UTC(), Payload: encoded}
	return a.Store.UpsertResources(ctx, []cache.ResourceEntry{entry})
}

func (a *MetricFollowerPort) DeleteSubscription(ctx context.Context, luid string) error {
	return a.Client.DeleteSubscription(ctx, luid)
}

type CachedFollowerPort struct {
	Store             *cache.Store
	Environment, Site string
	Support           CacheSupport
	source            *readsource.Metadata
	snapshot          *followerSnapshot
}

func (r *CachedFollowerPort) GetMetric(ctx context.Context, luid string) (metric.Metric, error) {
	result, err := r.Store.ReadResources(ctx, cache.ResourceQuery{Environment: r.Environment, Site: r.Site, Kind: followerSnapshotKind, LUID: luid, Limit: 1})
	if err != nil {
		return metric.Metric{}, r.Support.ReadError("pulse.metric.followers", r.Environment, r.Site, err)
	}
	r.source = r.Support.RecordSource(result, result.Entries[0])
	var snapshot followerSnapshot
	if err := json.Unmarshal(result.Entries[0].Payload, &snapshot); err != nil {
		return metric.Metric{}, fmt.Errorf("decode cache Pulse follower snapshot: %w", err)
	}
	if snapshot.Version != 1 || snapshot.MetricLUID != luid || snapshot.Subscriptions == nil {
		return metric.Metric{}, errors.New("cache Pulse follower snapshot is incomplete or has an unsupported version; run the exact follower command without --cache to replace it")
	}
	r.snapshot = &snapshot
	return metric.Metric{LUID: snapshot.MetricLUID}, nil
}

func (r *CachedFollowerPort) ListSubscriptions(context.Context, string) ([]metric.Subscription, error) {
	items := make([]metric.Subscription, len(r.snapshot.Subscriptions))
	copy(items, r.snapshot.Subscriptions)
	return items, nil
}

func (r *CachedFollowerPort) Source() *readsource.Metadata { return r.source }

func (a *MetricFollowerPort) ResolveMetric(ctx context.Context, luid string) (metric.FollowMetric, error) {
	item, err := a.Client.GetMetric(ctx, luid)
	if err != nil {
		return metric.FollowMetric{}, err
	}
	return metric.FollowMetric{LUID: item.LUID}, nil
}

func (a *MetricFollowerPort) ResolveUser(ctx context.Context, luid string) (metric.FollowUser, error) {
	if a.CheckCapability != nil {
		if err := a.CheckCapability("admin.user.inspect"); err != nil {
			return metric.FollowUser{}, err
		}
	}
	item, err := a.AdminClient.GetUser(ctx, luid)
	if err != nil {
		return metric.FollowUser{}, err
	}
	return metric.FollowUser{LUID: item.LUID}, nil
}

func (a *MetricFollowerPort) ResolveGroup(ctx context.Context, luid string) (metric.FollowGroup, error) {
	if a.CheckCapability != nil {
		if err := a.CheckCapability("admin.group.inspect"); err != nil {
			return metric.FollowGroup{}, err
		}
	}
	if _, err := a.AdminClient.ListGroupUsers(ctx, luid, tableauadmin.PageRequest{PageNumber: 1, PageSize: 1}); err != nil {
		return metric.FollowGroup{}, err
	}
	return metric.FollowGroup{LUID: luid}, nil
}

func (a *MetricFollowerPort) ListSubscriptions(ctx context.Context, metricLUID string) ([]metric.Subscription, error) {
	items, err := a.Client.ListSubscriptions(ctx, metricLUID)
	if err != nil {
		return nil, err
	}
	a.items = append([]tableaupulse.Subscription(nil), items...)
	result := make([]metric.Subscription, len(items))
	for index, item := range items {
		result[index] = metric.Subscription{LUID: item.LUID, MetricLUID: item.MetricLUID, FollowerType: item.FollowerType, FollowerLUID: item.FollowerLUID, FollowerName: item.FollowerName, RequestID: item.TableauRequestID}
	}
	return result, nil
}

func (a *MetricFollowerPort) CreateSubscription(ctx context.Context, request metric.FollowCreateRequest) (metric.FollowCreateResult, error) {
	result, err := a.Client.CreateSubscription(ctx, tableaupulse.CreateSubscriptionRequest{MetricLUID: request.MetricLUID, FollowerType: request.FollowerType, FollowerLUID: request.FollowerLUID})
	return metric.FollowCreateResult{Status: result.Status, SubscriptionLUID: result.SubscriptionLUID, RequestID: result.TableauRequestID}, err
}
