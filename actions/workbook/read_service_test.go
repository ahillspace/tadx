package workbook_test

import (
	"context"
	"testing"
	"time"

	workbook "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/readsource"
)

type readServicePages struct{}

type cachedReadServicePages struct{ readServicePages }

func (cachedReadServicePages) Source() *readsource.Metadata { return &readsource.Metadata{} }

func (readServicePages) ListWorkbooks(_ context.Context, input workbook.ListPageRequest) (workbook.ListPage, error) {
	items := []workbook.Record{{LUID: "one", Name: "One"}, {LUID: "two", Name: "Two"}}
	start := (input.PageNumber - 1) * input.PageSize
	return workbook.ListPage{Number: input.PageNumber, Size: input.PageSize, Total: len(items), Workbooks: items[start:min(start+input.PageSize, len(items))]}, nil
}

type readServiceProvider struct{ opens, cacheTargets int }

func (p *readServiceProvider) CacheTarget(string) (workbook.ReadTarget, error) {
	p.cacheTargets++
	return workbook.ReadTarget{Environment: "canonical", Site: "exact-site"}, nil
}
func (*readServiceProvider) CachedList(workbook.ReadTarget) workbook.CachedListReader {
	return cachedReadServicePages{}
}
func (*readServiceProvider) CachedInspect(workbook.ReadTarget) workbook.CachedInspectResolver {
	return nil
}
func (*readServiceProvider) ListFilter(workbook.ListInput) (string, error) { return "", nil }
func (p *readServiceProvider) OpenWorkbookRead(context.Context, string, string, string) (workbook.ReadSession, error) {
	p.opens++
	return workbook.ReadSession{ReadTarget: workbook.ReadTarget{Environment: "canonical", Site: "exact-site"}, Reader: readServicePages{}}, nil
}
func (*readServiceProvider) Now() time.Time { return time.Date(2026, 9, 1, 12, 0, 0, 0, time.UTC) }

func TestReadServiceCanonicalCursorAndPreOpenValidation(t *testing.T) {
	provider := &readServiceProvider{}
	service := workbook.New(workbook.Ports{Read: provider})
	if _, err := service.ListWorkbooks(t.Context(), workbook.ListInput{Limit: 10001}); err == nil || provider.opens != 0 {
		t.Fatalf("invalid limit reached provider: opens=%d err=%v", provider.opens, err)
	}
	if _, err := service.InspectWorkbook(t.Context(), workbook.InspectInput{}); err == nil || provider.opens != 0 {
		t.Fatalf("invalid selector reached provider: opens=%d err=%v", provider.opens, err)
	}
	first, err := service.ListWorkbooks(t.Context(), workbook.ListInput{Limit: 1})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "exact-site" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := service.ListWorkbooks(t.Context(), workbook.ListInput{Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Workbooks) != 1 || second.Workbooks[0].LUID != "two" || provider.opens != 2 {
		t.Fatalf("second=%#v opens=%d err=%v", second, provider.opens, err)
	}
	if _, err := service.ListWorkbooks(t.Context(), workbook.ListInput{Limit: 1, Cursor: "invalid"}); err == nil || provider.opens != 2 || provider.cacheTargets == 0 {
		t.Fatalf("invalid cursor reached remote provider: opens=%d err=%v", provider.opens, err)
	}
}
func TestReadServiceCachedFirstPageBindsCanonicalTarget(t *testing.T) {
	provider := &readServiceProvider{}
	service := workbook.New(workbook.Ports{Read: provider})
	first, err := service.ListWorkbooks(t.Context(), workbook.ListInput{Cache: true, Limit: 1})
	if err != nil || first.Page.NextCursor == "" || first.Environment != "canonical" || first.Site != "exact-site" {
		t.Fatalf("first=%#v err=%v", first, err)
	}
	second, err := service.ListWorkbooks(t.Context(), workbook.ListInput{Cache: true, Limit: 1, Cursor: first.Page.NextCursor})
	if err != nil || len(second.Workbooks) != 1 || second.Workbooks[0].LUID != "two" || provider.opens != 0 {
		t.Fatalf("second=%#v opens=%d err=%v", second, provider.opens, err)
	}
}
