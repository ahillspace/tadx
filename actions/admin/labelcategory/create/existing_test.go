package create

import (
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

func TestExistingCategoryNeverBecomesNoOp(t *testing.T) {
	for _, preview := range []bool{false, true} {
		stub := &definitionStub{item: value.LabelCategory{Name: "Definition", Description: "same"}}
		out, err := New(stub, stub).Execute(t.Context(), Input{Environment: "test", TargetResolved: true, Name: "Definition", Description: "same"}, preview)
		detail, ok := errors.AsType[*errs.Error](err)
		if !ok || detail.ID != "admin.label.category.create.exists" || out.Plan.NoOp || out.Result != nil || stub.writes != 0 {
			t.Fatalf("out=%+v err=%v writes=%d", out, err, stub.writes)
		}
	}
}
