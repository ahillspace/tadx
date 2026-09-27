package labelvalue

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type listReaderStub struct {
	items []value.LabelValue
	calls int
}

func (s *listReaderStub) ListLabelValues(context.Context) ([]value.LabelValue, error) {
	s.calls++
	return s.items, nil
}
func TestListValidationBeforeRead(t *testing.T) {
	r := &listReaderStub{}
	e := ValidateListInput(ListInput{Limit: -1})
	if e == nil || r.calls != 0 {
		t.Fatalf("invalid input contacted reader: %v", e)
	}
}
func TestListBoundedOrderedList(t *testing.T) {
	v := value.LabelValue{Name: "Definition", Description: "Meaning"}
	r := &listReaderStub{items: []value.LabelValue{v}}
	out, e := List(t.Context(), r, ListInput{})
	if e != nil || out.Returned != 1 {
		t.Fatalf("list: %+v %v", out, e)
	}
	for i := 0; i < 21; i++ {
		copy := v
		copy.Name = string(rune('a' + i))
		r.items = append(r.items, copy)
	}
	out, e = List(t.Context(), r, ListInput{})
	if e != nil || out.Returned != 20 || !out.MoreAvailable {
		t.Fatalf("limit: %+v %v", out, e)
	}
}

func TestListAllReturnsCompleteInventory(t *testing.T) {
	items := make([]value.LabelValue, 25)
	for i := range items {
		items[i].Name = string(rune('a' + i))
	}
	out, err := List(t.Context(), &listReaderStub{items: items}, ListInput{All: true})
	if err != nil || len(out.Items) != len(items) || out.Returned != len(items) || out.Total != len(items) || out.MoreAvailable || out.NextCommand != "" {
		t.Fatalf("all: %+v %v", out, err)
	}
}

func TestListAllRejectsLimit(t *testing.T) {
	if err := ValidateListInput(ListInput{All: true, Limit: 1}); err == nil {
		t.Fatal("--all accepted with --limit")
	}
}
