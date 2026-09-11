package delete

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

func (s *deleteStub) GetLabelCategory(context.Context, string) (value.LabelCategory, error) {
	return value.LabelCategory{Name: "Definition"}, nil
}

func (s *deleteStub) ListLabelValues(context.Context) ([]value.LabelValue, error) {
	if s.members {
		return []value.LabelValue{{Name: "Member", Category: "Definition"}}, nil
	}
	return nil, nil
}
func (s *deleteStub) DeleteLabelCategory(context.Context, string) error {
	s.writes++
	if s.writeErr {
		return errors.New("connection lost")
	}
	return nil
}
func TestPreviewPerformAndUnknownSubmission(t *testing.T) {
	in := Input{Environment: "test", TargetResolved: true, Name: "Definition"}
	s := &deleteStub{}
	out, e := New(s, s).Execute(context.Background(), in, true)
	if e != nil || s.writes != 0 || out.Mode != "preview" {
		t.Fatalf("preview: %+v %v", out, e)
	}
	out, e = New(s, s).Execute(context.Background(), in, false)
	if e != nil || s.writes != 1 || out.Status != "deleted" {
		t.Fatalf("delete: %+v %v", out, e)
	}
	s.writeErr = true
	out, e = New(s, s).Execute(context.Background(), in, false)
	var classified *errs.Error
	if !errors.As(e, &classified) || classified.Outcome != errs.OutcomeUnknown || out.Status != "unknown" {
		t.Fatalf("unknown: %+v %v", out, e)
	}
}
func TestInvalidInputCannotWrite(t *testing.T) {
	s := &deleteStub{}
	if _, e := New(s, s).Execute(context.Background(), Input{}, false); e == nil || s.writes != 0 {
		t.Fatal("invalid write")
	}
}
func TestNonemptyCategoryCannotBeDeleted(t *testing.T) {
	s := &deleteStub{members: true}
	if _, e := New(s, s).Execute(context.Background(), Input{Environment: "test", TargetResolved: true, Name: "Definition"}, false); e == nil || s.writes != 0 {
		t.Fatal("nonempty category deleted")
	}
}
