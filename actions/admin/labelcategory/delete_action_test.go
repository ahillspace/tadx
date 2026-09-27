package labelcategory

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

func (s *deleteDeleteStub) GetLabelCategory(context.Context, string) (value.LabelCategory, error) {
	return value.LabelCategory{Name: "Definition"}, nil
}

func (s *deleteDeleteStub) ListLabelValues(context.Context) ([]value.LabelValue, error) {
	if s.members {
		return []value.LabelValue{{Name: "Member", Category: "Definition"}}, nil
	}
	return nil, nil
}
func (s *deleteDeleteStub) DeleteLabelCategory(context.Context, string) error {
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
	if e != nil || s.writes != 1 || out.Status != "deleted" {
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
func TestDeleteNonemptyCategoryCannotBeDeleted(t *testing.T) {
	s := &deleteDeleteStub{members: true}
	if _, e := Delete(t.Context(), s, s, DeleteInput{Environment: "test", Name: "Definition"}, false); e == nil || s.writes != 0 {
		t.Fatal("nonempty category deleted")
	}
}
