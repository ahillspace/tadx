package contentlabel

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type deleteStub struct {
	writes   int
	writeErr bool
	members  bool
}

func (s *deleteStub) GetLabel(context.Context, string) (value.ContentLabel, error) {
	return value.ContentLabel{LUID: "label-1", Type: "table", TargetLUID: "table-1", Value: "Warning"}, nil
}
func (s *deleteStub) GetLabelValue(_ context.Context, n string) (value.LabelValue, error) {
	return value.LabelValue{Name: n}, nil
}

func (s *deleteStub) DeleteLabel(context.Context, string) error {
	s.writes++
	if s.writeErr {
		return errors.New("connection lost")
	}
	return nil
}
func TestPreviewPerformAndUnknownSubmission(t *testing.T) {
	in := DeleteInput{Environment: "test", ID: "label-1"}
	s := &deleteStub{}
	out, e := deleteLabel(t.Context(), s, s, in, true)
	if e != nil || s.writes != 0 || out.Mode != "preview" {
		t.Fatalf("preview: %+v %v", out, e)
	}
	out, e = deleteLabel(t.Context(), s, s, in, false)
	if e != nil || s.writes != 1 || out.Status != "deleted" {
		t.Fatalf("delete: %+v %v", out, e)
	}
	s.writeErr = true
	out, e = deleteLabel(t.Context(), s, s, in, false)
	var classified *errs.Error
	if !errors.As(e, &classified) || classified.Outcome != errs.OutcomeUnknown || out.Status != "unknown" {
		t.Fatalf("unknown: %+v %v", out, e)
	}
}
func TestInvalidInputCannotWrite(t *testing.T) {
	s := &deleteStub{}
	if e := ValidateDeleteInput(DeleteInput{}); e == nil || s.writes != 0 {
		t.Fatal("invalid write")
	}
}
