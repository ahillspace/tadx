package list

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type reader struct {
	pages       []Page
	queries     []PageRequest
	metrics     []Metric
	definitions []Definition
	metricIDs   [][]string
	defIDs      [][]string
	pageErr     error
}

func (r *reader) ListUserSubscriptions(_ context.Context, _ string, request PageRequest) (Page, error) {
	r.queries = append(r.queries, request)
	if r.pageErr != nil {
		return Page{}, r.pageErr
	}
	page := r.pages[0]
	r.pages = r.pages[1:]
	return page, nil
}
func (r *reader) GetMetrics(_ context.Context, ids []string) ([]Metric, error) {
	r.metricIDs = append(r.metricIDs, append([]string(nil), ids...))
	return r.metrics, nil
}
func (r *reader) GetDefinitions(_ context.Context, ids []string) ([]Definition, error) {
	r.defIDs = append(r.defIDs, append([]string(nil), ids...))
	return r.definitions, nil
}

func TestListContinuesOnlyReturnedIDs(t *testing.T) {
	r := &reader{pages: []Page{
		{Subscriptions: []Subscription{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"}}, NextPageToken: "next"},
		{Subscriptions: []Subscription{{LUID: "sub-2", MetricLUID: "metric-1", FollowerType: "GROUP", FollowerLUID: "group-1"}}},
	}, metrics: []Metric{{LUID: "metric-1", DefinitionLUID: "definition-1", Specification: map[string]any{"measurement_period": "LAST_30_DAYS"}}}, definitions: []Definition{{LUID: "definition-1", Name: "Revenue"}}}
	input := Input{Environment: "dev", Site: "site", UserLUID: "user-1"}
	output, err := New(r).Execute(t.Context(), input)
	if err != nil || output.Count != 2 || !output.Coverage.Complete || len(r.metricIDs) != 1 || len(r.metricIDs[0]) != 1 || len(r.defIDs) != 1 || output.Subscriptions[1].DefinitionName != "Revenue" || output.Subscriptions[1].FollowerType != "GROUP" {
		t.Fatalf("output=%+v reader=%+v err=%v", output, r, err)
	}
	if r.queries[0].PageSize != 25 || r.queries[1].PageToken != "next" {
		t.Fatalf("queries=%+v", r.queries)
	}
}

func TestListCursorBindsAuthenticatedUser(t *testing.T) {
	r := &reader{pages: []Page{{Subscriptions: []Subscription{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"}}, NextPageToken: "next"}}, metrics: []Metric{{LUID: "metric-1", DefinitionLUID: "definition-1"}}, definitions: []Definition{{LUID: "definition-1", Name: "Revenue"}}}
	input := Input{Environment: "dev", Site: "site", UserLUID: "user-1", Limit: 1}
	output, err := New(r).Execute(t.Context(), input)
	if err != nil || output.Coverage.NextCursor == "" || !output.Coverage.MoreAvailable || output.Count != 1 {
		t.Fatalf("output=%+v err=%v", output, err)
	}
	input.Cursor = output.Coverage.NextCursor
	input.UserLUID = "user-2"
	if _, err := New(r).Execute(t.Context(), input); err == nil || len(r.queries) != 1 {
		t.Fatalf("foreign cursor used: err=%v queries=%+v", err, r.queries)
	}
}

func TestListRejectsMismatchedDirectFollowerAndDuplicate(t *testing.T) {
	for _, items := range [][]Subscription{
		{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "another-user"}},
		{{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"}, {LUID: "sub-1", MetricLUID: "metric-2", FollowerType: "USER", FollowerLUID: "user-1"}},
	} {
		r := &reader{pages: []Page{{Subscriptions: items}}}
		if _, err := New(r).Execute(t.Context(), Input{UserLUID: "user-1"}); err == nil || len(r.metricIDs) != 0 {
			t.Fatalf("invalid records accepted: %v", items)
		}
	}
}

func TestListMalformedLaterRowRetainsPartialCoverage(t *testing.T) {
	r := &reader{pages: []Page{{Subscriptions: []Subscription{
		{LUID: "sub-1", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "user-1"},
		{LUID: "sub-1", MetricLUID: "metric-2", FollowerType: "USER", FollowerLUID: "user-1"},
	}}}}
	output, err := New(r).Execute(t.Context(), Input{UserLUID: "user-1"})
	if err == nil || output.Status != "partial" || output.Count != 1 || len(output.Subscriptions) != 1 || output.Subscriptions[0].SubscriptionLUID != "sub-1" || output.Coverage.Complete || !output.Coverage.MoreAvailable {
		t.Fatalf("output=%+v err=%v", output, err)
	}
}

func TestListDistinguishesConfirmedEmptyFromUnreadable(t *testing.T) {
	input := Input{UserLUID: "user-1"}
	empty, err := New(&reader{pages: []Page{{Subscriptions: []Subscription{}}}}).Execute(t.Context(), input)
	if err != nil || empty.Count != 0 || !empty.Coverage.Complete || empty.Status != "listed" {
		t.Fatalf("empty=%+v err=%v", empty, err)
	}
	unreadable, err := New(&reader{pageErr: errors.New("unavailable")}).Execute(t.Context(), input)
	if err == nil || unreadable.Status != "partial" || unreadable.Coverage.Complete || !strings.Contains(err.Error(), "unavailable") {
		t.Fatalf("unreadable=%+v err=%v", unreadable, err)
	}
}

func TestListEmptyUserNeverRequestsUnfilteredSubscriptions(t *testing.T) {
	r := &reader{}
	if _, err := New(r).Execute(t.Context(), Input{}); err == nil || len(r.queries) != 0 {
		t.Fatalf("empty user sent a request: err=%v queries=%+v", err, r.queries)
	}
}
