package catalog

import (
	"context"
	"testing"

	databaselist "github.com/ahillspace/tadx/actions/catalog/database/list"
	catalogupdate "github.com/ahillspace/tadx/actions/catalog/update"
)

type recorder struct {
	calls   int
	preview bool
	update  catalogupdate.DatabaseInput
	list    databaselist.Input
}

func (r *recorder) Render(any) error { return nil }
func (r *recorder) UpdateCatalogDatabase(_ context.Context, in catalogupdate.DatabaseInput, p bool) (catalogupdate.DatabaseOutput, error) {
	r.calls++
	r.update = in
	r.preview = p
	return catalogupdate.DatabaseOutput{}, nil
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
