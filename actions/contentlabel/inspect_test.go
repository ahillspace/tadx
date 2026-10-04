package contentlabel

import (
	"context"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type inspectReaderStub struct {
	item  value.ContentLabel
	calls int
}

func (s *inspectReaderStub) GetLabel(context.Context, string) (value.ContentLabel, error) {
	s.calls++
	return s.item, nil
}
func TestExactIdentityAndValidation(t *testing.T) {
	r := &inspectReaderStub{item: value.ContentLabel{LUID: "label-1", Type: "table", TargetLUID: "table-1", Value: "Warning"}}
	if e := ValidateInspectInput(InspectInput{}); e == nil || r.calls != 0 {
		t.Fatal("missing identity contacted reader")
	}
	out, e := inspectLabel(t.Context(), r, InspectInput{ID: "label-1"})
	if e != nil || out.Item.LUID != r.item.LUID {
		t.Fatalf("inspect: %+v %v", out, e)
	}
	r.item.LUID = "other"
	if _, e := inspectLabel(t.Context(), r, InspectInput{ID: "label-1"}); e == nil {
		t.Fatal("mismatched identity accepted")
	}
}

func TestNativeDatasourceTypeIsCanonicalizedButMismatchesStillFail(t *testing.T) {
	r := &inspectReaderStub{item: value.ContentLabel{LUID: "label-1", Type: "DATASOURCE", TargetLUID: "source-1", Value: "Warning"}}
	out, err := inspectLabel(t.Context(), r, InspectInput{ID: "label-1", Type: "datasource", TargetID: "source-1"})
	if err != nil || out.Item == nil || out.Item.Type != "datasource" {
		t.Fatalf("native datasource type rejected: %+v %v", out, err)
	}
	r.item.Type = "workbook"
	if _, err := inspectLabel(t.Context(), r, InspectInput{ID: "label-1", Type: "datasource", TargetID: "source-1"}); err == nil {
		t.Fatal("true content type mismatch accepted")
	}
}
