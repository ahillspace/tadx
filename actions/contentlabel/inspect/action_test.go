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

func TestNativeDatasourceTypeIsCanonicalizedButMismatchesStillFail(t *testing.T) {
	r := &readerStub{item: value.ContentLabel{LUID: "label-1", Type: "DATASOURCE", TargetLUID: "source-1", Value: "Warning"}}
	out, err := New(r).Execute(t.Context(), Input{ID: "label-1", Type: "datasource", TargetID: "source-1"})
	if err != nil || out.Item == nil || out.Item.Type != "datasource" {
		t.Fatalf("native datasource type rejected: %+v %v", out, err)
	}
	r.item.Type = "workbook"
	if _, err := New(r).Execute(t.Context(), Input{ID: "label-1", Type: "datasource", TargetID: "source-1"}); err == nil {
		t.Fatal("true content type mismatch accepted")
	}
}
