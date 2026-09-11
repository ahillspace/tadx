package list

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type reader struct {
	calls int
	pages []value.MetadataPage[value.MetadataDatabase]
}

func (r *reader) DiscoverDatabases(context.Context, value.MetadataQuery) (value.MetadataPage[value.MetadataDatabase], error) {
	r.calls++
	return r.pages[r.calls-1], nil
}
func TestValidationBeforeRead(t *testing.T) {
	r := &reader{}
	_, err := New(r).Execute(context.Background(), Input{Limit: -1})
	if err == nil || r.calls != 0 {
		t.Fatal("invalid bound reached reader")
	}
}
func TestBoundedPagesAndProjection(t *testing.T) {
	r := &reader{pages: []value.MetadataPage[value.MetadataDatabase]{{Items: []value.MetadataDatabase{{MetadataIdentity: value.MetadataIdentity{LUID: "one", Name: "One", Type: "database"}}}, NextCursor: "opaque", Total: 2}, {Items: []value.MetadataDatabase{{MetadataIdentity: value.MetadataIdentity{LUID: "two", Name: "Two", Type: "database"}}}, Complete: true, Total: 2}}}
	out, err := New(r).Execute(context.Background(), Input{All: true})
	if err != nil || len(out.Items) != 2 || !out.Complete {
		t.Fatalf("%+v %v", out, err)
	}
	out.CompactOutput()
	out.FullOutput()
	if r.calls != 2 {
		t.Fatal("projection fetched remotely")
	}
}
func TestRepeatedCursorFails(t *testing.T) {
	r := &reader{pages: []value.MetadataPage[value.MetadataDatabase]{{NextCursor: "x"}, {NextCursor: "x"}}}
	_, err := New(r).Execute(context.Background(), Input{All: true})
	if err == nil {
		t.Fatal("repeated cursor accepted")
	}
}
