package inspect

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type readerStub struct {
	item  value.LabelValue
	calls int
}

func (s *readerStub) GetLabelValue(context.Context, string) (value.LabelValue, error) {
	s.calls++
	return s.item, nil
}
func TestExactIdentityAndValidation(t *testing.T) {
	r := &readerStub{item: value.LabelValue{Name: "Definition", Description: "Meaning"}}
	a := New(r)
	if _, e := a.Execute(context.Background(), Input{}); e == nil || r.calls != 0 {
		t.Fatal("missing identity contacted reader")
	}
	out, e := a.Execute(context.Background(), Input{Name: "Definition"})
	if e != nil || out.Item.Name != r.item.Name {
		t.Fatalf("inspect: %+v %v", out, e)
	}
	r.item.Name = "other"
	if _, e := a.Execute(context.Background(), Input{Name: "Definition"}); e == nil {
		t.Fatal("mismatched identity accepted")
	}
}
