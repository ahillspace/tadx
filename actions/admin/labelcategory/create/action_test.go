package create

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type definitionStub struct {
	item        value.LabelCategory
	writes      int
	readbackErr bool
}

func (s *definitionStub) ListLabelCategories(context.Context) ([]value.LabelCategory, error) {
	if s.item.Name == "" {
		return nil, nil
	}
	return []value.LabelCategory{s.item}, nil
}
func (s *definitionStub) GetLabelCategory(context.Context, string) (value.LabelCategory, error) {
	if s.writes > 0 && s.readbackErr {
		return value.LabelCategory{}, errors.New("readback unavailable")
	}
	return s.item, nil
}
func (s *definitionStub) CreateLabelCategory(_ context.Context, v value.LabelCategory) (value.LabelCategory, error) {
	s.writes++
	s.item = v
	return v, nil
}
func TestPreviewAndConfirmedReceipt(t *testing.T) {
	for _, preview := range []bool{true, false} {
		s := &definitionStub{readbackErr: true}
		in := Input{Environment: "test", TargetResolved: true, Name: "Definition", Description: "new"}
		out, e := New(s, s).Execute(context.Background(), in, preview)
		if preview {
			if e != nil || s.writes != 0 || out.Result != nil {
				t.Fatalf("preview: %+v %v", out, e)
			}
		} else {
			var classified *errs.Error
			if !errors.As(e, &classified) || classified.Outcome != errs.OutcomeConfirmed || out.Result == nil || out.Result.Item.Name != "Definition" || s.writes != 1 {
				t.Fatalf("receipt: %+v %v", out, e)
			}
		}
	}
}
func TestValidationAndResolvedEnvironment(t *testing.T) {
	s := &definitionStub{}
	if _, e := New(s, s).Execute(context.Background(), Input{}, false); e == nil || s.writes != 0 {
		t.Fatal("invalid input accepted")
	}
	if _, e := New(s, s).Execute(context.Background(), Input{Environment: "test", Name: "Definition", Description: "new"}, false); e == nil || s.writes != 0 {
		t.Fatal("unresolved environment accepted")
	}
}
