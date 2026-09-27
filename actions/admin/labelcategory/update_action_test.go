package labelcategory

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type updateDefinitionStub struct {
	item        value.LabelCategory
	writes      int
	readbackErr bool
}

func (s *updateDefinitionStub) ListLabelCategories(context.Context) ([]value.LabelCategory, error) {
	if s.item.Name == "" {
		return nil, nil
	}
	return []value.LabelCategory{s.item}, nil
}
func (s *updateDefinitionStub) GetLabelCategory(context.Context, string) (value.LabelCategory, error) {
	if s.writes > 0 && s.readbackErr {
		return value.LabelCategory{}, errors.New("readback unavailable")
	}
	return s.item, nil
}
func (s *updateDefinitionStub) UpdateLabelCategory(_ context.Context, _ string, v value.LabelCategory) (value.LabelCategory, error) {
	s.writes++
	s.item = v
	return v, nil
}
func TestUpdatePreviewAndConfirmedReceipt(t *testing.T) {
	for _, preview := range []bool{true, false} {
		s := &updateDefinitionStub{item: value.LabelCategory{Name: "Definition", Description: "old"}, readbackErr: true}
		description := "new"
		in := UpdateInput{Environment: "test", Name: "Definition", Description: &description}
		out, e := Update(t.Context(), s, s, in, preview)
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
func TestUpdateValidation(t *testing.T) {
	s := &updateDefinitionStub{}
	if e := ValidateUpdateInput(UpdateInput{}); e == nil || s.writes != 0 {
		t.Fatal("invalid input accepted")
	}
}
