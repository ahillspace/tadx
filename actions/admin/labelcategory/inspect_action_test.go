package labelcategory

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type inspectReaderStub struct {
	item  value.LabelCategory
	calls int
}

func (s *inspectReaderStub) GetLabelCategory(context.Context, string) (value.LabelCategory, error) {
	s.calls++
	return s.item, nil
}
func TestInspectExactIdentityAndValidation(t *testing.T) {
	r := &inspectReaderStub{item: value.LabelCategory{Name: "Definition", Description: "Meaning"}}
	if e := ValidateInspectInput(InspectInput{}); e == nil || r.calls != 0 {
		t.Fatal("missing identity contacted reader")
	}
	out, e := Inspect(t.Context(), r, InspectInput{Name: "Definition"})
	if e != nil || out.Item.Name != r.item.Name {
		t.Fatalf("inspect: %+v %v", out, e)
	}
}
