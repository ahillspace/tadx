package labelvalue

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type deleteDeleteStub struct {
	writes   int
	writeErr bool
	members  bool
}

func (s *deleteDeleteStub) GetLabelValue(context.Context, string) (value.LabelValue, error) {
	return value.LabelValue{Name: "Definition", BuiltIn: true}, nil
}

func (s *deleteDeleteStub) DeleteLabelValue(context.Context, string) error {
	s.writes++
	if s.writeErr {
		return errors.New("connection lost")
	}
	return nil
}
func TestDeletePreviewPerformAndUnknownSubmission(t *testing.T) {
	in := DeleteInput{Environment: "test", Name: "Definition"}
	s := &deleteDeleteStub{}
	out, e := Delete(t.Context(), s, s, in, true)
	if e != nil || s.writes != 0 || out.Mode != "preview" {
		t.Fatalf("preview: %+v %v", out, e)
	}
	out, e = Delete(t.Context(), s, s, in, false)
	if e != nil || s.writes != 1 || out.Status != "reset" {
		t.Fatalf("delete: %+v %v", out, e)
	}
	s.writeErr = true
	out, e = Delete(t.Context(), s, s, in, false)
	var classified *errs.Error
	if !errors.As(e, &classified) || classified.Outcome != errs.OutcomeUnknown || out.Status != "unknown" {
		t.Fatalf("unknown: %+v %v", out, e)
	}
}
func TestDeleteInvalidInputCannotWrite(t *testing.T) {
	s := &deleteDeleteStub{}
	if e := ValidateDeleteInput(DeleteInput{}); e == nil || s.writes != 0 {
		t.Fatal("invalid write")
	}
}
