package pulse

import (
	"context"
	"encoding/base64"
	"strings"
	"testing"

	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

type metricSearchFake struct {
	definitions     tableaupulse.DefinitionPage
	metrics         []tableaupulse.MetricPage
	definitionCalls int
	requests        []tableaupulse.PageRequest
	definitionIDs   []string
}

func (f *metricSearchFake) ListDefinitions(_ context.Context, input tableaupulse.PageRequest) (tableaupulse.DefinitionPage, error) {
	f.definitionCalls++
	return f.definitions, nil
}

func (f *metricSearchFake) ListMetrics(_ context.Context, id string, input tableaupulse.PageRequest) (tableaupulse.MetricPage, error) {
	f.requests = append(f.requests, input)
	f.definitionIDs = append(f.definitionIDs, id)
	page := f.metrics[0]
	f.metrics = f.metrics[1:]
	return page, nil
}

func TestMetricSearchPreservesNativeContinuationAndCachesDefinitions(t *testing.T) {
	client := &metricSearchFake{
		definitions: tableaupulse.DefinitionPage{Definitions: []tableaupulse.Definition{{LUID: "d-1", Name: "Revenue"}, {LUID: "d-2", Name: "Cost"}}},
		metrics: []tableaupulse.MetricPage{
			{Metrics: []tableaupulse.Metric{{LUID: "m-1", Name: " "}}, NextPageToken: "metric-next"},
			{Metrics: []tableaupulse.Metric{{LUID: "m-2", Name: "Regional revenue"}}},
			{Metrics: []tableaupulse.Metric{{LUID: "m-3", Name: "Cost"}}},
		},
	}
	source := NewMetricSearch(client)
	first, err := source.List(t.Context(), "", 1)
	if err != nil || len(first.Items) != 1 || first.Items[0].Name != "Revenue" || first.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	second, err := source.List(t.Context(), first.NextCursor, 1)
	if err != nil || len(second.Items) != 1 || second.Items[0].Name != "Regional revenue" || second.NextCursor == "" {
		t.Fatalf("second=%+v err=%v", second, err)
	}
	third, err := source.List(t.Context(), second.NextCursor, 1)
	if err != nil || len(third.Items) != 1 || third.Items[0].LUID != "m-3" || third.NextCursor != "" {
		t.Fatalf("third=%+v err=%v", third, err)
	}
	if client.definitionCalls != 1 || client.requests[1].PageToken != "metric-next" || client.requests[1].PageSize != 1 || client.definitionIDs[2] != "d-2" {
		t.Fatalf("client=%+v", client)
	}
}

func TestMetricSearchRejectsChangedDefinitionInventory(t *testing.T) {
	original := &metricSearchFake{
		definitions: tableaupulse.DefinitionPage{Definitions: []tableaupulse.Definition{{LUID: "d-1"}, {LUID: "d-2"}}},
		metrics:     []tableaupulse.MetricPage{{Metrics: []tableaupulse.Metric{{LUID: "m-1"}}}},
	}
	first, err := NewMetricSearch(original).List(t.Context(), "", 1)
	if err != nil || first.NextCursor == "" {
		t.Fatalf("first=%+v err=%v", first, err)
	}
	changed := &metricSearchFake{definitions: tableaupulse.DefinitionPage{Definitions: []tableaupulse.Definition{{LUID: "d-other"}, {LUID: "d-2"}}}}
	_, err = NewMetricSearch(changed).List(t.Context(), first.NextCursor, 1)
	if err == nil || !strings.Contains(err.Error(), "inventory changed") || len(changed.requests) != 0 {
		t.Fatalf("err=%v client=%+v", err, changed)
	}
}

func TestMetricSearchRejectsMalformedCursorBeforeNativeReads(t *testing.T) {
	for _, encoded := range []string{"!", base64.RawURLEncoding.EncodeToString([]byte(`{"v":2}`)), base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"i":-1}`)), base64.RawURLEncoding.EncodeToString([]byte(`{"v":1,"i":1}`))} {
		client := &metricSearchFake{}
		if _, err := NewMetricSearch(client).List(t.Context(), encoded, 1); err == nil || client.definitionCalls != 0 || len(client.requests) != 0 {
			t.Fatalf("cursor=%q err=%v client=%+v", encoded, err, client)
		}
	}
}
