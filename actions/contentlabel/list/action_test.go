package list

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type readerStub struct {
	items []value.ContentLabel
	calls int
}

func (s *readerStub) GetLabels(context.Context, value.LabelTarget, []string) ([]value.ContentLabel, error) {
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
	v := value.ContentLabel{LUID: "label-1", Type: "table", TargetLUID: "table-1", Value: "Warning"}
	r := &readerStub{items: []value.ContentLabel{v}}
	out, e := New(r).Execute(context.Background(), Input{Type: "table", TargetID: "table-1"})
	if e != nil || out.Returned != 1 {
		t.Fatalf("list: %+v %v", out, e)
	}
	r.items = append(r.items, v)
	if _, e := New(r).Execute(context.Background(), Input{Type: "table", TargetID: "table-1"}); e == nil {
		t.Fatal("duplicate identity accepted")
	}
	r.items = r.items[:1]
	for i := 0; i < 21; i++ {
		copy := v
		copy.LUID = string(rune('a' + i))
		r.items = append(r.items, copy)
	}
	out, e = New(r).Execute(context.Background(), Input{Type: "table", TargetID: "table-1"})
	if e != nil || out.Returned != 20 || !out.MoreAvailable {
		t.Fatalf("limit: %+v %v", out, e)
	}
}
