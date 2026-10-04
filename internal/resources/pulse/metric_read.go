package pulse

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	metric "github.com/ahillspace/tadx/actions/pulse/metric"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

// MetricListPort adapts native metric pages and best-effort summary publication.
type MetricListPort struct {
	Client      *tableaupulse.Client
	Store       *cache.Store
	Environment string
	Site        string
	Now         func() time.Time
	items       []tableaupulse.Metric
}

func (p *MetricListPort) ListMetrics(ctx context.Context, definitionLUID string, request metric.ListPageRequest) (metric.ListPage, error) {
	page, err := p.Client.ListMetrics(ctx, definitionLUID, tableaupulse.PageRequest{PageSize: request.PageSize, PageToken: request.PageToken})
	if err != nil {
		return metric.ListPage{}, err
	}
	p.items = append(p.items, page.Metrics...)
	items := make([]metric.ListMetric, len(page.Metrics))
	for index, item := range page.Metrics {
		items[index] = metricListItem(item)
	}
	return metric.ListPage{Metrics: items, NextPageToken: page.NextPageToken, RequestID: page.TableauRequestID}, nil
}

func (p *MetricListPort) Publish() {
	observedAt := p.Now().UTC()
	entries := make([]cache.ResourceEntry, 0, len(p.items))
	for _, item := range p.items {
		entry, err := metricEntry(p.Environment, p.Site, "summary", observedAt, item)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	inventory.PublishEntries(p.Store, entries)
}

// MetricInspectPort adapts exact native metric reads and deletion receipts.
type MetricInspectPort struct {
	Client      *tableaupulse.Client
	Store       *cache.Store
	Environment string
	Site        string
	Now         func() time.Time
	item        tableaupulse.Metric
}

func (p *MetricInspectPort) GetMetric(ctx context.Context, luid string) (metric.Metric, error) {
	item, err := p.Client.GetMetric(ctx, luid)
	if err != nil {
		return metric.Metric{}, err
	}
	p.item = item
	return MetricObservation(item), nil
}

func (p *MetricInspectPort) DeleteMetric(ctx context.Context, luid string) (metric.DeleteResult, error) {
	item, err := p.Client.DeleteMetric(ctx, luid)
	return metric.DeleteResult{Status: item.Status, MetricLUID: item.LUID, HTTPStatus: item.HTTPStatus, TableauRequestID: item.TableauRequestID}, err
}

func (p *MetricInspectPort) Publish() {
	entry, err := metricEntry(p.Environment, p.Site, "detail", p.Now().UTC(), p.item)
	if err == nil {
		inventory.PublishDetail(p.Store, entry)
	}
}

// CachedMetricListPort adapts one bounded cached metric page.
type CachedMetricListPort struct {
	Store       *cache.Store
	Environment string
	Site        string
	Support     CacheSupport
	source      *readsource.Metadata
}

func (p *CachedMetricListPort) Source() *readsource.Metadata { return p.source }

func (p *CachedMetricListPort) ListMetrics(ctx context.Context, definitionLUID string, request metric.ListPageRequest) (metric.ListPage, error) {
	offset, err := pulseCacheOffset(request.PageToken)
	if err != nil {
		return metric.ListPage{}, err
	}
	result, err := p.Store.ReadResources(ctx, cache.ResourceQuery{Environment: p.Environment, Site: p.Site, Kind: "metric", ProjectPath: definitionLUID, Offset: offset, Limit: request.PageSize})
	if err != nil {
		return metric.ListPage{}, p.Support.ReadError("pulse.metric.list", p.Environment, p.Site, err)
	}
	p.source = p.Support.ReadSource(result)
	items := make([]metric.ListMetric, len(result.Entries))
	for index, entry := range result.Entries {
		var item tableaupulse.Metric
		if err := json.Unmarshal(entry.Payload, &item); err != nil {
			return metric.ListPage{}, fmt.Errorf("decode cache Pulse metric %q: %w", entry.LUID, err)
		}
		items[index] = metricListItem(item)
	}
	return metric.ListPage{Metrics: items, NextPageToken: nextPulseCacheOffset(offset, len(items), result.Total)}, nil
}

// CachedMetricInspectPort adapts one exact cached metric record.
type CachedMetricInspectPort struct {
	Store       *cache.Store
	Environment string
	Site        string
	Support     CacheSupport
	source      *readsource.Metadata
}

func (p *CachedMetricInspectPort) Source() *readsource.Metadata { return p.source }

func (p *CachedMetricInspectPort) GetMetric(ctx context.Context, luid string) (metric.Metric, error) {
	result, err := p.Store.ReadResources(ctx, cache.ResourceQuery{Environment: p.Environment, Site: p.Site, Kind: "metric", LUID: luid, Limit: 1})
	if err != nil {
		return metric.Metric{}, p.Support.ReadError("pulse.metric.inspect", p.Environment, p.Site, err)
	}
	p.source = p.Support.RecordSource(result, result.Entries[0])
	var item tableaupulse.Metric
	if err := json.Unmarshal(result.Entries[0].Payload, &item); err != nil {
		return metric.Metric{}, fmt.Errorf("decode cache Pulse metric %q: %w", luid, err)
	}
	item.TableauRequestID = ""
	return MetricObservation(item), nil
}

func metricListItem(item tableaupulse.Metric) metric.ListMetric {
	return metric.ListMetric{LUID: item.LUID, Name: item.Name, DefinitionLUID: item.DefinitionLUID, IsDefault: item.IsDefault, Specification: cloneJSONMap(item.Specification)}
}

// MetricObservation preserves an independent copy of nested native values.
func MetricObservation(item tableaupulse.Metric) metric.Metric {
	return metric.Metric{LUID: item.LUID, Name: item.Name, DefinitionLUID: item.DefinitionLUID, SiteLUID: item.SiteLUID, IsDefault: item.IsDefault, DefaultKnown: item.DefaultKnown, Specification: cloneJSONMap(item.Specification), Configuration: append([]byte(nil), item.Configuration...), RequestID: item.TableauRequestID}
}

func metricEntry(environment, site, coverage string, observedAt time.Time, item tableaupulse.Metric) (cache.ResourceEntry, error) {
	payload, err := json.Marshal(item)
	if err != nil {
		return cache.ResourceEntry{}, err
	}
	name := strings.TrimSpace(item.Name)
	if name == "" {
		name = item.LUID
	}
	return cache.ResourceEntry{Environment: environment, Site: site, Kind: "metric", LUID: item.LUID, Name: name, ProjectPath: item.DefinitionLUID, Coverage: coverage, ObservedAt: observedAt, Payload: payload}, nil
}

func cloneJSONMap(input map[string]any) map[string]any {
	if input == nil {
		return nil
	}
	encoded, err := json.Marshal(input)
	if err != nil {
		return nil
	}
	var output map[string]any
	decoder := json.NewDecoder(bytes.NewReader(encoded))
	decoder.UseNumber()
	if decoder.Decode(&output) != nil {
		return nil
	}
	return output
}
