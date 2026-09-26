package datasource_test

import (
	"context"
	"errors"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"testing"
)

type deleteDeleteResolver struct {
	items []datasourceops.Record
	calls int
}

func (r *deleteDeleteResolver) ResolveDatasource(context.Context, identity.Selector) (datasourceops.Record, error) {
	item := r.items[min(r.calls, len(r.items)-1)]
	r.calls++
	return item, nil
}

type deleteDeleter struct{ calls int }

func (d *deleteDeleter) DeleteDatasource(context.Context, string) (datasourceops.DeleteResult, error) {
	d.calls++
	return datasourceops.DeleteResult{Status: "succeeded", DatasourceLUID: "ds-1", TableauRequestID: "request-1"}, nil
}

func TestDeleteDeletePreviewsWithoutMutationAndRevalidatesOnApply(t *testing.T) {
	item := datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics"}
	resolver := &deleteDeleteResolver{items: []datasourceops.Record{item, item, item}}
	deleter := &deleteDeleter{}
	actionResolver, actionDeleter := resolver, deleter
	preview, err := datasourceops.Delete(context.Background(), actionResolver, actionDeleter, datasourceops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "ds-1"}}, true)
	if err != nil || preview.Result != nil || deleter.calls != 0 {
		t.Fatalf("preview = %#v, error = %v, delete calls = %d", preview, err, deleter.calls)
	}
	result, err := datasourceops.Delete(context.Background(), actionResolver, actionDeleter, datasourceops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "ds-1"}}, false)
	if err != nil || result.Result == nil || deleter.calls != 1 || resolver.calls != 3 {
		t.Fatalf("result = %#v, error = %v, resolver calls = %d", result, err, resolver.calls)
	}
}

func TestDeleteDeleteStopsWhenExactTargetChanges(t *testing.T) {
	first := datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics"}
	second := first
	second.ProjectPath = "Moved"
	resolver := &deleteDeleteResolver{items: []datasourceops.Record{first, second}}
	deleter := &deleteDeleter{}
	_, err := datasourceops.Delete(context.Background(), resolver, deleter, datasourceops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "ds-1"}}, false)
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "datasource.delete.target_changed" || deleter.calls != 0 {
		t.Fatalf("error = %#v, delete calls = %d", err, deleter.calls)
	}
}

func TestDeleteDeleteRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := datasourceops.Delete(context.Background(), &deleteDeleteResolver{items: []datasourceops.Record{{}}}, &deleteDeleter{}, datasourceops.DeleteInput{}, false)
	if err == nil {
		t.Fatal("delete without explicit target succeeded")
	}
}

func TestDeleteIgnoresNonIdentityMetadataChanges(t *testing.T) {
	first := datasourceops.Record{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics", Tags: []string{"first"}, OwnerLUID: "first-owner"}
	second := first
	second.Tags, second.OwnerLUID, second.Description = []string{"second"}, "second-owner", "changed metadata"
	resolver := &deleteDeleteResolver{items: []datasourceops.Record{first, second}}
	deleter := &deleteDeleter{}
	_, err := datasourceops.Delete(t.Context(), resolver, deleter, datasourceops.DeleteInput{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "ds-1"}}, false)
	if err != nil || deleter.calls != 1 || resolver.calls != 2 {
		t.Fatalf("err=%v deletes=%d resolutions=%d", err, deleter.calls, resolver.calls)
	}
}
