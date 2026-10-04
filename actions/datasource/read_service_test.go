package datasource_test

import (
	"context"
	"testing"
	"time"

	datasource "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/readsource"
)

type readServicePages struct{}

type cachedReadServicePages struct{ readServicePages }

func (cachedReadServicePages) Source() *readsource.Metadata { return &readsource.Metadata{} }

func (readServicePages) ListDatasources(_ context.Context, input datasource.ListPageRequest) (datasource.ListPage, error) {
	items := []datasource.Record{{LUID: "one", Name: "One"}, {LUID: "two", Name: "Two"}}
	start := (input.PageNumber - 1) * input.PageSize
	return datasource.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(items), Datasources: items[start:min(start+input.PageSize, len(items))]}, nil
}

type readServiceProvider struct{ opens, cacheTargets int }

func (p *readServiceProvider) CacheTarget(string) (datasource.ReadTarget, error) {
	p.cacheTargets++
	return datasource.ReadTarget{Environment: "canonical", Site: "exact-site"}, nil
}
func (*readServiceProvider) CachedList(datasource.ReadTarget) datasource.CachedListReader {
	return cachedReadServicePages{}
}
func (*readServiceProvider) CachedInspect(datasource.ReadTarget) datasource.CachedInspectResolver {
	return nil
}
func (*readServiceProvider) LegacyInventoryCursor(string) bool               { return false }
func (*readServiceProvider) ListFilter(datasource.ListInput) (string, error) { return "", nil }
func (p *readServiceProvider) OpenDatasourceRead(context.Context, string, string, string) (datasource.ReadSession, error) {
	p.opens++
	return datasource.ReadSession{ReadTarget: datasource.ReadTarget{Environment: "canonical", Site: "exact-site"}, Reader: readServicePages{}}, nil
}
func (*readServiceProvider) ValidateComplete(bool, *readsource.Metadata) error { return nil }
func (*readServiceProvider) RefreshError(_, _, _ string, err error) error      { return err }
func (*readServiceProvider) Now() time.Time                                    { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }

func TestReadServiceCanonicalCursorAndPreOpenValidation(t *testing.T) {
	provider := &readServiceProvider{}
	service := datasource.New(datasource.Ports{Read: provider})
	if _, err := service.ListDatasources(t.Context(), datasource.ListInput{Limit: 10001}); err == nil || provider.opens != 0 {
		t.Fatalf("invalid limit reached provider: opens=%d err=%v", provider.opens, err)
	}
	if _, err := service.InspectDatasource(t.Context(), datasource.InspectInput{}); err == nil || provider.opens != 0 {
		t.Fatalf("invalid selector reached provider: opens=%d err=%v", provider.opens, err)
	}
	first, err := service.ListDatasources(t.Context(), datasource.ListInput{Limit: 1})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "exact-site" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := service.ListDatasources(t.Context(), datasource.ListInput{Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Datasources) != 1 || second.Datasources[0].LUID != "two" || provider.opens != 2 {
		t.Fatalf("second=%#v opens=%d err=%v", second, provider.opens, err)
	}
	if _, err := service.ListDatasources(t.Context(), datasource.ListInput{Limit: 1, Cursor: "invalid"}); err == nil || provider.opens != 2 || provider.cacheTargets == 0 {
		t.Fatalf("invalid cursor reached remote provider: opens=%d err=%v", provider.opens, err)
	}
}
func TestReadServiceCachedFirstPageBindsCanonicalTarget(t *testing.T) {
	provider := &readServiceProvider{}
	service := datasource.New(datasource.Ports{Read: provider})
	first, err := service.ListDatasources(t.Context(), datasource.ListInput{Cache: true, Limit: 1})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "exact-site" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := service.ListDatasources(t.Context(), datasource.ListInput{Cache: true, Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Datasources) != 1 || second.Datasources[0].LUID != "two" || provider.opens != 0 {
		t.Fatalf("second=%#v opens=%d err=%v", second, provider.opens, err)
	}
}
