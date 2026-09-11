package list

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type readerStub struct {
	items []value.LabelCategory
	calls int
}

func (s *readerStub) ListLabelCategories(context.Context) ([]value.LabelCategory, error) {
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
	v := value.LabelCategory{Name: "Definition", Description: "Meaning"}
	r := &readerStub{items: []value.LabelCategory{v}}
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
