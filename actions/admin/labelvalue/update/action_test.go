package update

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type definitionStub struct {
	item        value.LabelValue
	writes      int
	readbackErr bool
}

func (s *definitionStub) ListLabelValues(context.Context) ([]value.LabelValue, error) {
	if s.item.Name == "" {
		return nil, nil
	}
	return []value.LabelValue{s.item}, nil
}
func (s *definitionStub) GetLabelValue(context.Context, string) (value.LabelValue, error) {
	if s.writes > 0 && s.readbackErr {
		return value.LabelValue{}, errors.New("readback unavailable")
	}
	return s.item, nil
}
func (s *definitionStub) SetLabelValue(_ context.Context, _ string, v value.LabelValue) (value.LabelValue, error) {
	s.writes++
	s.item = v
	return v, nil
}
func TestPreviewAndConfirmedReceipt(t *testing.T) {
	for _, preview := range []bool{true, false} {
		s := &definitionStub{item: value.LabelValue{Name: "Definition", Description: "old", Category: "Category"}, readbackErr: true}
		description := "new"
		in := Input{Environment: "test", TargetResolved: true, Name: "Definition", Description: &description}
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
	description := "new"
	if _, e := New(s, s).Execute(context.Background(), Input{Environment: "test", Name: "Definition", Description: &description}, false); e == nil || s.writes != 0 {
		t.Fatal("unresolved environment accepted")
	}
}
