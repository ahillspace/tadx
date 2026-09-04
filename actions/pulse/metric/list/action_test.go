package list_test

import (
	"context"
	"testing"

	metriclist "github.com/ahillspace/tadx/actions/pulse/metric/list"
)

type reader struct{ request metriclist.PageRequest }

func (r *reader) ListMetrics(_ context.Context, definition string, request metriclist.PageRequest) (metriclist.Page, error) {
	r.request = request
	return metriclist.Page{Metrics: []metriclist.Metric{{LUID: "metric-1", Name: "Revenue", DefinitionLUID: definition}}, NextPageToken: "next", RequestID: "request-1"}, nil
}

func TestListReturnsOneBoundedDefinitionPage(t *testing.T) {
	r := &reader{}
	output, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if output.Page.Returned != 1 || output.Metrics[0].DefinitionLUID != "definition-1" || output.Page.NextCursor == "" || r.request.PageSize != 10 {
		t.Fatalf("output=%#v request=%#v", output, r.request)
	}
}

func TestListRejectsMissingDefinition(t *testing.T) {
	if _, err := metriclist.New(&reader{}).Execute(context.Background(), metriclist.Input{}); err == nil {
		t.Fatal("missing definition accepted")
	}
}

func TestCursorCannotSwitchBetweenTableauAndCatalog(t *testing.T) {
	r := &reader{}
	output, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "dev", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10, Cursor: output.Page.NextCursor, Catalog: true}); err == nil {
		t.Fatal("cursor accepted after source switch")
	}
	if _, err := metriclist.New(r).Execute(context.Background(), metriclist.Input{Environment: "prod", Site: "sandbox", DefinitionLUID: "definition-1", Limit: 10, Cursor: output.Page.NextCursor}); err == nil {
		t.Fatal("cursor accepted after environment switch")
	}
}
