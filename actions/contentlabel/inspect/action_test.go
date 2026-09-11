package inspect

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type readerStub struct {
	item  value.ContentLabel
	calls int
}

func (s *readerStub) GetLabel(context.Context, string) (value.ContentLabel, error) {
	s.calls++
	return s.item, nil
}
func TestExactIdentityAndValidation(t *testing.T) {
	r := &readerStub{item: value.ContentLabel{LUID: "label-1", Type: "table", TargetLUID: "table-1", Value: "Warning"}}
	a := New(r)
	if _, e := a.Execute(context.Background(), Input{}); e == nil || r.calls != 0 {
		t.Fatal("missing identity contacted reader")
	}
	out, e := a.Execute(context.Background(), Input{ID: "label-1"})
	if e != nil || out.Item.LUID != r.item.LUID {
		t.Fatalf("inspect: %+v %v", out, e)
	}
	r.item.LUID = "other"
	if _, e := a.Execute(context.Background(), Input{ID: "label-1"}); e == nil {
		t.Fatal("mismatched identity accepted")
	}
}
