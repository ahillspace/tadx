package pulse

import (
	"context"
	"errors"
	"reflect"
	"testing"

	subscription "github.com/ahillspace/tadx/actions/pulse/subscription"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type subscriptionClient struct {
	page                     tableaupulse.SubscriptionPage
	metrics                  []tableaupulse.Metric
	definitions              []tableaupulse.Definition
	err                      error
	user                     string
	request                  tableaupulse.PageRequest
	metricIDs, definitionIDs []string
}

func (c *subscriptionClient) ListUserSubscriptions(_ context.Context, user string, request tableaupulse.PageRequest) (tableaupulse.SubscriptionPage, error) {
	c.user, c.request = user, request
	return c.page, c.err
}

func (c *subscriptionClient) BatchGetMetrics(_ context.Context, ids []string) ([]tableaupulse.Metric, error) {
	c.metricIDs = ids
	return c.metrics, c.err
}

func (c *subscriptionClient) BatchGetDefinitions(_ context.Context, ids []string) ([]tableaupulse.Definition, error) {
	c.definitionIDs = ids
	return c.definitions, c.err
}

func TestSubscriptionReaderPreservesNativeObservations(t *testing.T) {
	spec := map[string]any{"filters": []any{"filter"}}
	client := &subscriptionClient{
		page:        tableaupulse.SubscriptionPage{Subscriptions: []tableaupulse.Subscription{{LUID: "sub", MetricLUID: "metric", FollowerType: "GROUP", FollowerLUID: "group"}}, NextPageToken: "next", TableauRequestID: "request"},
		metrics:     []tableaupulse.Metric{{LUID: "metric", Name: "Metric", DefinitionLUID: "definition", Specification: spec}},
		definitions: []tableaupulse.Definition{{LUID: "definition", Name: "Definition"}},
	}
	reader := NewSubscriptionReader(client)
	page, err := reader.ListUserSubscriptions(t.Context(), "user", subscription.PageRequest{PageSize: 7, PageToken: "current"})
	want := subscription.Page{Subscriptions: []subscription.Subscription{{LUID: "sub", MetricLUID: "metric", FollowerType: "GROUP", FollowerLUID: "group"}}, NextPageToken: "next", RequestID: "request"}
	if err != nil || !reflect.DeepEqual(page, want) || client.user != "user" || client.request.PageSize != 7 || client.request.PageToken != "current" {
		t.Fatalf("page=%+v client=%+v err=%v", page, client, err)
	}
	metrics, err := reader.GetMetrics(t.Context(), []string{"metric"})
	if err != nil || !reflect.DeepEqual(metrics, []subscription.Metric{{LUID: "metric", Name: "Metric", DefinitionLUID: "definition", Specification: spec}}) || !reflect.DeepEqual(client.metricIDs, []string{"metric"}) {
		t.Fatalf("metrics=%+v IDs=%v err=%v", metrics, client.metricIDs, err)
	}
	definitions, err := reader.GetDefinitions(t.Context(), []string{"definition"})
	if err != nil || !reflect.DeepEqual(definitions, []subscription.Definition{{LUID: "definition", Name: "Definition"}}) || !reflect.DeepEqual(client.definitionIDs, []string{"definition"}) {
		t.Fatalf("definitions=%+v IDs=%v err=%v", definitions, client.definitionIDs, err)
	}
}

func TestSubscriptionReaderRetainsEmptySuccessAndNativeErrors(t *testing.T) {
	client := &subscriptionClient{}
	reader := NewSubscriptionReader(client)
	page, err := reader.ListUserSubscriptions(t.Context(), "user", subscription.PageRequest{})
	if err != nil || page.Subscriptions == nil || len(page.Subscriptions) != 0 {
		t.Fatalf("empty page=%+v err=%v", page, err)
	}
	metrics, err := reader.GetMetrics(t.Context(), nil)
	if err != nil || metrics == nil || len(metrics) != 0 {
		t.Fatalf("empty metrics=%+v err=%v", metrics, err)
	}
	definitions, err := reader.GetDefinitions(t.Context(), nil)
	if err != nil || definitions == nil || len(definitions) != 0 {
		t.Fatalf("empty definitions=%+v err=%v", definitions, err)
	}
	client.err = errors.New("native failure")
	page, err = reader.ListUserSubscriptions(t.Context(), "user", subscription.PageRequest{})
	if !errors.Is(err, client.err) || page.Subscriptions != nil {
		t.Fatalf("failed page=%+v err=%v", page, err)
	}
	metrics, err = reader.GetMetrics(t.Context(), nil)
	if !errors.Is(err, client.err) || metrics != nil {
		t.Fatalf("failed metrics=%+v err=%v", metrics, err)
	}
	definitions, err = reader.GetDefinitions(t.Context(), nil)
	if !errors.Is(err, client.err) || definitions != nil {
		t.Fatalf("failed definitions=%+v err=%v", definitions, err)
	}
}
