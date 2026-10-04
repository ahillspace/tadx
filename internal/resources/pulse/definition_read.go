package pulse

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strconv"
	"time"

	definition "github.com/ahillspace/tadx/actions/pulse/definition"
	"github.com/ahillspace/tadx/internal/cache"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/readsource"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

// CacheSupport binds shared cache error and coverage policy to typed Pulse ports.
type CacheSupport struct {
	ReadError    func(string, string, string, error) error
	ReadSource   func(cache.ResourceResult) *readsource.Metadata
	RecordSource func(cache.ResourceResult, cache.ResourceEntry) *readsource.Metadata
}

// DefinitionListPort adapts native Pulse pages and best-effort summary publication.
type DefinitionListPort struct {
	Client      *tableaupulse.Client
	Store       *cache.Store
	Environment string
	Site        string
	Now         func() time.Time
	items       []tableaupulse.Definition
}

func (p *DefinitionListPort) ListDefinitions(ctx context.Context, request definition.ListPageRequest) (definition.ListPage, error) {
	page, err := p.Client.ListDefinitions(ctx, tableaupulse.PageRequest{PageSize: request.PageSize, PageToken: request.PageToken})
	if err != nil {
		return definition.ListPage{}, err
	}
	p.items = append(p.items, page.Definitions...)
	items := make([]definition.ListDefinition, len(page.Definitions))
	for index, item := range page.Definitions {
		items[index] = definitionListItem(item)
	}
	return definition.ListPage{Definitions: items, NextPageToken: page.NextPageToken, RequestID: page.TableauRequestID}, nil
}

func (p *DefinitionListPort) Publish() {
	observedAt := p.Now().UTC()
	entries := make([]cache.ResourceEntry, 0, len(p.items))
	for _, item := range p.items {
		entry, err := definitionEntry(p.Environment, p.Site, "summary", observedAt, item)
		if err == nil {
			entries = append(entries, entry)
		}
	}
	inventory.PublishEntries(p.Store, entries)
}

// DefinitionInspectPort adapts one native Pulse definition and its detail publication.
type DefinitionInspectPort struct {
	Client      *tableaupulse.Client
	Store       *cache.Store
	Environment string
	Site        string
	Now         func() time.Time
	item        tableaupulse.Definition
}

func (p *DefinitionInspectPort) GetDefinition(ctx context.Context, luid string) (definition.Definition, error) {
	item, err := p.Client.GetDefinition(ctx, luid)
	if err != nil {
		return definition.Definition{}, err
	}
	p.item = item
	return definitionGetItem(item)
}

func (p *DefinitionInspectPort) Publish() {
	entry, err := definitionEntry(p.Environment, p.Site, "detail", p.Now().UTC(), p.item)
	if err == nil {
		inventory.PublishDetail(p.Store, entry)
	}
}

// CachedDefinitionListPort adapts one bounded cached definition page.
type CachedDefinitionListPort struct {
	Store       *cache.Store
	Environment string
	Site        string
	Support     CacheSupport
	source      *readsource.Metadata
}

func (p *CachedDefinitionListPort) Source() *readsource.Metadata { return p.source }

func (p *CachedDefinitionListPort) ListDefinitions(ctx context.Context, request definition.ListPageRequest) (definition.ListPage, error) {
	offset, err := pulseCacheOffset(request.PageToken)
	if err != nil {
		return definition.ListPage{}, err
	}
	result, err := p.Store.ReadResources(ctx, cache.ResourceQuery{Environment: p.Environment, Site: p.Site, Kind: "definition", Offset: offset, Limit: request.PageSize})
	if err != nil {
		return definition.ListPage{}, p.Support.ReadError("pulse.definition.list", p.Environment, p.Site, err)
	}
	p.source = p.Support.ReadSource(result)
	items := make([]definition.ListDefinition, len(result.Entries))
	for index, entry := range result.Entries {
		var item tableaupulse.Definition
		if err := json.Unmarshal(entry.Payload, &item); err != nil {
			return definition.ListPage{}, fmt.Errorf("decode cache Pulse definition %q: %w", entry.LUID, err)
		}
		items[index] = definitionListItem(item)
	}
	return definition.ListPage{Definitions: items, NextPageToken: nextPulseCacheOffset(offset, len(items), result.Total)}, nil
}

// CachedDefinitionInspectPort adapts one exact cached definition record.
type CachedDefinitionInspectPort struct {
	Store       *cache.Store
	Environment string
	Site        string
	Support     CacheSupport
	source      *readsource.Metadata
}

func (p *CachedDefinitionInspectPort) Source() *readsource.Metadata { return p.source }

func (p *CachedDefinitionInspectPort) GetDefinition(ctx context.Context, luid string) (definition.Definition, error) {
	result, err := p.Store.ReadResources(ctx, cache.ResourceQuery{Environment: p.Environment, Site: p.Site, Kind: "definition", LUID: luid, Limit: 1})
	if err != nil {
		return definition.Definition{}, p.Support.ReadError("pulse.definition.inspect", p.Environment, p.Site, err)
	}
	p.source = p.Support.RecordSource(result, result.Entries[0])
	var item tableaupulse.Definition
	if err := json.Unmarshal(result.Entries[0].Payload, &item); err != nil {
		return definition.Definition{}, fmt.Errorf("decode cache Pulse definition %q: %w", luid, err)
	}
	item.TableauRequestID = ""
	return definitionGetItem(item)
}

func definitionListItem(item tableaupulse.Definition) definition.ListDefinition {
	return definition.ListDefinition{LUID: item.LUID, Name: item.Name, Description: item.Description, DatasourceLUID: item.DatasourceLUID, MeasureField: item.MeasureField, Aggregation: item.Aggregation, TimeDimension: item.TimeDimension, AllowedDimensions: append([]string(nil), item.AllowedDimensions...)}
}

func definitionGetItem(item tableaupulse.Definition) (definition.Definition, error) {
	configuration := map[string]any{}
	if len(item.Configuration) != 0 {
		if err := json.Unmarshal(item.Configuration, &configuration); err != nil {
			return definition.Definition{}, fmt.Errorf("decode Pulse definition configuration: %w", err)
		}
	}
	result := DefinitionObservation(item)
	result.Configuration = configuration
	return result, nil
}

// DefinitionObservation preserves native facts without imposing inspect decoding.
func DefinitionObservation(item tableaupulse.Definition) definition.Definition {
	return definition.Definition{LUID: item.LUID, Name: item.Name, Description: item.Description, DatasourceLUID: item.DatasourceLUID, MeasureField: item.MeasureField, Aggregation: item.Aggregation, TimeDimension: item.TimeDimension, RunningTotal: item.RunningTotal, Temporality: item.Temporality, AllowedDimensions: append([]string(nil), item.AllowedDimensions...), AllowedGranularities: append([]string(nil), item.AllowedGranularities...), RequestID: item.TableauRequestID}
}

func definitionEntry(environment, site, coverage string, observedAt time.Time, item tableaupulse.Definition) (cache.ResourceEntry, error) {
	payload, err := json.Marshal(item)
	if err != nil {
		return cache.ResourceEntry{}, err
	}
	return cache.ResourceEntry{Environment: environment, Site: site, Kind: "definition", LUID: item.LUID, Name: item.Name, Coverage: coverage, ObservedAt: observedAt, Payload: payload}, nil
}

func pulseCacheOffset(token string) (int, error) {
	if token == "" {
		return 0, nil
	}
	offset, err := strconv.Atoi(token)
	if err != nil || offset < 0 {
		return 0, errors.New("invalid cache continuation token")
	}
	return offset, nil
}

func nextPulseCacheOffset(offset, returned, total int) string {
	if returned == 0 || offset+returned >= total {
		return ""
	}
	return strconv.Itoa(offset + returned)
}
