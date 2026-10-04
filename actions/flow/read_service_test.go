package flow_test

import (
	"context"
	"testing"
	"time"

	flow "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/readsource"
)

type readServicePages struct{}

type cachedReadServicePages struct{ readServicePages }

func (cachedReadServicePages) Source() *readsource.Metadata { return &readsource.Metadata{} }

func (readServicePages) ListFlows(_ context.Context, input flow.ListPageRequest) (flow.ListPage, error) {
	items := []flow.Record{{LUID: "one", Name: "One"}, {LUID: "two", Name: "Two"}}
	start := (input.PageNumber - 1) * input.PageSize
	return flow.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(items), Flows: items[start:min(start+input.PageSize, len(items))]}, nil
}

type readServiceProvider struct{ opens, cacheTargets int }

func (p *readServiceProvider) CacheTarget(string) (flow.ReadTarget, error) {
	p.cacheTargets++
	return flow.ReadTarget{Environment: "canonical", Site: "exact-site"}, nil
}
func (*readServiceProvider) CachedList(flow.ReadTarget) flow.CachedListReader {
	return cachedReadServicePages{}
}
func (*readServiceProvider) CachedInspect(flow.ReadTarget) flow.CachedInspectResolver {
	return nil
}
func (*readServiceProvider) LegacyInventoryCursor(string) bool         { return false }
func (*readServiceProvider) ListFilter(flow.ListInput) (string, error) { return "", nil }
func (p *readServiceProvider) OpenFlowRead(context.Context, string, string, string) (flow.ReadSession, error) {
	p.opens++
	return flow.ReadSession{ReadTarget: flow.ReadTarget{Environment: "canonical", Site: "exact-site"}, Reader: readServicePages{}}, nil
}
func (*readServiceProvider) ValidateComplete(bool, *readsource.Metadata) error { return nil }
func (*readServiceProvider) RefreshError(_, _, _ string, err error) error      { return err }
func (*readServiceProvider) Now() time.Time                                    { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }

func TestReadServiceCanonicalCursorAndPreOpenValidation(t *testing.T) {
	provider := &readServiceProvider{}
	service := flow.New(flow.Ports{Read: provider})
	if _, err := service.ListFlows(t.Context(), flow.ListInput{Limit: 10001}); err == nil || provider.opens != 0 {
		t.Fatalf("invalid limit reached provider: opens=%d err=%v", provider.opens, err)
	}
	if _, err := service.InspectFlow(t.Context(), flow.InspectInput{}); err == nil || provider.opens != 0 {
		t.Fatalf("invalid selector reached provider: opens=%d err=%v", provider.opens, err)
	}
	first, err := service.ListFlows(t.Context(), flow.ListInput{Limit: 1})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "exact-site" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := service.ListFlows(t.Context(), flow.ListInput{Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Flows) != 1 || second.Flows[0].LUID != "two" || provider.opens != 2 {
		t.Fatalf("second=%#v opens=%d err=%v", second, provider.opens, err)
	}
	if _, err := service.ListFlows(t.Context(), flow.ListInput{Limit: 1, Cursor: "invalid"}); err == nil || provider.opens != 2 || provider.cacheTargets == 0 {
		t.Fatalf("invalid cursor reached remote provider: opens=%d err=%v", provider.opens, err)
	}
}
func TestReadServiceCachedFirstPageBindsCanonicalTarget(t *testing.T) {
	provider := &readServiceProvider{}
	service := flow.New(flow.Ports{Read: provider})
	first, err := service.ListFlows(t.Context(), flow.ListInput{Cache: true, Limit: 1})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "exact-site" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := service.ListFlows(t.Context(), flow.ListInput{Cache: true, Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Flows) != 1 || second.Flows[0].LUID != "two" || provider.opens != 0 {
		t.Fatalf("second=%#v opens=%d err=%v", second, provider.opens, err)
	}
}
