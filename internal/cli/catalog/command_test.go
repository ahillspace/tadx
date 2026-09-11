package catalog

import (
	"context"
	databaselist "github.com/ahillspace/tadx/actions/catalog/database/list"
	databaseupdate "github.com/ahillspace/tadx/actions/catalog/database/update"
	"testing"
)

type recorder struct {
	calls   int
	preview bool
	update  databaseupdate.Input
	list    databaselist.Input
}

func (r *recorder) Render(any) error { return nil }
func (r *recorder) UpdateCatalogDatabase(_ context.Context, in databaseupdate.Input, p bool) (databaseupdate.Output, error) {
	r.calls++
	r.update = in
	r.preview = p
	return databaseupdate.Output{}, nil
}
func (r *recorder) ListCatalogDatabases(_ context.Context, in databaselist.Input) (databaselist.Output, error) {
	r.calls++
	r.list = in
	return databaselist.Output{}, nil
}
func TestPreviewFalseAndRepeatedTags(t *testing.T) {
	r := &recorder{}
	c := New(Dependencies{DatabaseUpdater: r, Renderer: r})
	c.SetArgs([]string{"database", "update", "--id", "db", "--description", "Useful description", "--add-tag", "sales", "--add-tag", "retail", "--preview=false"})
	if e := c.Execute(); e != nil {
		t.Fatal(e)
	}
	if r.preview || r.calls != 1 || len(r.update.AddTags) != 2 || r.update.Description == nil {
		t.Fatalf("%+v", r)
	}
}
func TestInvalidLimitDoesNotReachService(t *testing.T) {
	r := &recorder{}
	c := New(Dependencies{DatabaseLister: r, Renderer: r})
	c.SetArgs([]string{"database", "list", "--limit", "-2"})
	if e := c.Execute(); e == nil || r.calls != 0 {
		t.Fatal("invalid bound reached service")
	}
}
func TestCapabilitiesPresent(t *testing.T) {
	c := New(Dependencies{})
	for _, kind := range []string{"database", "table", "column"} {
		for _, verb := range []string{"list", "inspect", "update"} {
			v, _, e := c.Find([]string{kind, verb})
			if e != nil || v.Annotations["tadx.capability"] != "catalog."+kind+"."+verb {
				t.Fatal(kind, verb, e)
			}
		}
	}
}
