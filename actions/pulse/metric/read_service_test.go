package metric_test

import (
	"context"
	"errors"
	"testing"
	"time"

	metric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/readsource"
)

type metricReadPort struct {
	page      metric.ListPage
	item      metric.Metric
	listError error
	published int
}

func (p *metricReadPort) ListMetrics(context.Context, string, metric.ListPageRequest) (metric.ListPage, error) {
	return p.page, p.listError
}

func (p *metricReadPort) GetMetric(context.Context, string) (metric.Metric, error) {
	return p.item, nil
}

func (p *metricReadPort) Source() *readsource.Metadata { return nil }
func (p *metricReadPort) Publish()                     { p.published++ }

type metricReadProvider struct {
	port       *metricReadPort
	cacheCalls int
	openCalls  int
}

func (p *metricReadProvider) CacheTarget(string) (metric.ReadTarget, error) {
	p.cacheCalls++
	return metric.ReadTarget{Environment: "canonical", Site: "sales"}, nil
}

func (p *metricReadProvider) CachedList(metric.ReadTarget) metric.CachedListPort {
	return p.port
}

func (p *metricReadProvider) CachedInspect(metric.ReadTarget) metric.CachedInspectPort {
	return p.port
}

func (p *metricReadProvider) Open(context.Context, string, string, string) (metric.ReadSession, error) {
	p.openCalls++
	return metric.ReadSession{ReadTarget: metric.ReadTarget{Environment: "canonical", Site: "sales"}, List: p.port, Inspect: p.port}, nil
}

func (p *metricReadProvider) CacheSetupError(_, _ string, err error) error { return err }
func (p *metricReadProvider) Now() time.Time {
	return time.Date(2026, time.September, 4, 12, 0, 0, 0, time.UTC)
}

func TestMetricServiceRejectsLocalInputsBeforePortAcquisition(t *testing.T) {
	provider := &metricReadProvider{port: &metricReadPort{}}
	service := metric.New(provider)
	if _, err := service.ListPulseMetrics(t.Context(), metric.ListInput{}); err == nil {
		t.Fatal("missing exact definition LUID was accepted")
	}
	if _, err := service.InspectPulseMetric(t.Context(), metric.InspectInput{}); err == nil {
		t.Fatal("missing exact metric LUID was accepted")
	}
	if provider.cacheCalls != 0 || provider.openCalls != 0 {
		t.Fatalf("invalid input acquired ports: cache=%d open=%d", provider.cacheCalls, provider.openCalls)
	}
}

func TestMetricServiceBindsContinuationBeforeNativeOpen(t *testing.T) {
	provider := &metricReadProvider{port: &metricReadPort{page: metric.ListPage{NextPageToken: "opaque"}}}
	service := metric.New(provider)
	first, err := service.ListPulseMetrics(t.Context(), metric.ListInput{Environment: "alias", DefinitionLUID: "definition-1", Limit: 7})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "sales" {
		t.Fatalf("first page = %#v, %v", first, err)
	}
	_, err = service.ListPulseMetrics(t.Context(), metric.ListInput{Environment: "alias", DefinitionLUID: "definition-1", Limit: 8, Cursor: first.Page.NextCursor})
	if err == nil {
		t.Fatal("cursor changed limit was accepted")
	}
	if provider.openCalls != 1 || provider.port.published != 1 {
		t.Fatalf("native open count = %d; successful publication count = %d", provider.openCalls, provider.port.published)
	}
}

func TestMetricServiceDoesNotPublishFailedNativeRead(t *testing.T) {
	provider := &metricReadProvider{port: &metricReadPort{listError: errors.New("upstream failed")}}
	_, err := metric.New(provider).ListPulseMetrics(t.Context(), metric.ListInput{Environment: "alias", DefinitionLUID: "definition-1"})
	if err == nil || provider.port.published != 0 {
		t.Fatalf("failed read = %v; publication count = %d", err, provider.port.published)
	}
}
