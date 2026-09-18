package list

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type readerStub struct {
	items []value.LabelValue
	calls int
}

func (s *readerStub) ListLabelValues(context.Context) ([]value.LabelValue, error) {
	s.calls++
	return s.items, nil
}
func TestValidationBeforeRead(t *testing.T) {
	r := &readerStub{}
	_, e := New(r).Execute(context.Background(), Input{Limit: -1})
	if e == nil || r.calls != 0 {
		t.Fatalf("invalid input contacted reader: %v", e)
	}
}
func TestBoundedOrderedListAndDuplicateRejection(t *testing.T) {
	v := value.LabelValue{Name: "Definition", Description: "Meaning"}
	r := &readerStub{items: []value.LabelValue{v}}
	out, e := New(r).Execute(context.Background(), Input{})
	if e != nil || out.Returned != 1 {
		t.Fatalf("list: %+v %v", out, e)
	}
	r.items = append(r.items, v)
	if _, e := New(r).Execute(context.Background(), Input{}); e == nil {
		t.Fatal("duplicate identity accepted")
	}
	r.items = r.items[:1]
	for i := 0; i < 21; i++ {
		copy := v
		copy.Name = string(rune('a' + i))
		r.items = append(r.items, copy)
	}
	out, e = New(r).Execute(context.Background(), Input{})
	if e != nil || out.Returned != 20 || !out.MoreAvailable {
		t.Fatalf("limit: %+v %v", out, e)
	}
}

func TestAllReturnsCompleteInventory(t *testing.T) {
	items := make([]value.LabelValue, 25)
	for i := range items {
		items[i].Name = string(rune('a' + i))
	}
	out, err := New(&readerStub{items: items}).Execute(t.Context(), Input{All: true})
	if err != nil || len(out.Items) != len(items) || out.Returned != len(items) || out.Total != len(items) || out.MoreAvailable || out.NextCommand != "" {
		t.Fatalf("all: %+v %v", out, err)
	}
}

func TestAllRejectsLimit(t *testing.T) {
	if _, err := New(&readerStub{}).Execute(t.Context(), Input{All: true, Limit: 1}); err == nil {
		t.Fatal("--all accepted with --limit")
	}
}
