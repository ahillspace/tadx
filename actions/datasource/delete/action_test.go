package delete_test

import (
	"context"
	"errors"
	"testing"

	datasourcedelete "github.com/ahillspace/tadx/actions/datasource/delete"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

type deleteResolver struct {
	items []datasourcedelete.Datasource
	calls int
}

func (r *deleteResolver) ResolveDatasource(context.Context, identity.Selector) (datasourcedelete.Datasource, error) {
	item := r.items[min(r.calls, len(r.items)-1)]
	r.calls++
	return item, nil
}

type deleter struct{ calls int }

func (d *deleter) DeleteDatasource(context.Context, string) (datasourcedelete.Result, error) {
	d.calls++
	return datasourcedelete.Result{Status: "succeeded", DatasourceLUID: "ds-1", TableauRequestID: "request-1"}, nil
}

func TestDeletePreviewsWithoutMutationAndRevalidatesOnApply(t *testing.T) {
	item := datasourcedelete.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics"}
	resolver := &deleteResolver{items: []datasourcedelete.Datasource{item, item, item}}
	deleter := &deleter{}
	action := datasourcedelete.New(resolver, deleter)
	preview, err := action.Execute(context.Background(), datasourcedelete.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "ds-1"}}, true)
	if err != nil || preview.Result != nil || deleter.calls != 0 {
		t.Fatalf("preview = %#v, error = %v, delete calls = %d", preview, err, deleter.calls)
	}
	result, err := action.Execute(context.Background(), datasourcedelete.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "ds-1"}}, false)
	if err != nil || result.Result == nil || deleter.calls != 1 || resolver.calls != 3 {
		t.Fatalf("result = %#v, error = %v, resolver calls = %d", result, err, resolver.calls)
	}
}

func TestDeleteStopsWhenExactTargetChanges(t *testing.T) {
	first := datasourcedelete.Datasource{LUID: "ds-1", Name: "Sales", ProjectLUID: "project-1", ProjectPath: "Analytics"}
	second := first
	second.ProjectPath = "Moved"
	resolver := &deleteResolver{items: []datasourcedelete.Datasource{first, second}}
	deleter := &deleter{}
	_, err := datasourcedelete.New(resolver, deleter).Execute(context.Background(), datasourcedelete.Input{Environment: "dev", Site: "sandbox", Selector: identity.Selector{LUID: "ds-1"}}, false)
	var structured *errs.Error
	if err == nil || !errors.As(err, &structured) || structured.ID != "datasource.delete.target_changed" || deleter.calls != 0 {
		t.Fatalf("error = %#v, delete calls = %d", err, deleter.calls)
	}
}

func TestDeleteRequiresExplicitEnvironmentAndSite(t *testing.T) {
	_, err := datasourcedelete.New(&deleteResolver{items: []datasourcedelete.Datasource{{}}}, &deleter{}).Execute(context.Background(), datasourcedelete.Input{}, false)
	if err == nil {
		t.Fatal("delete without explicit target succeeded")
	}
}
