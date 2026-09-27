package labelvalue

import (
	"context"
	"errors"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
)

type updateDefinitionStub struct {
	item        value.LabelValue
	writes      int
	readbackErr bool
	oldName     string
}

func (s *updateDefinitionStub) ListLabelValues(context.Context) ([]value.LabelValue, error) {
	if s.item.Name == "" {
		return nil, nil
	}
	return []value.LabelValue{s.item}, nil
}
func (s *updateDefinitionStub) GetLabelValue(context.Context, string) (value.LabelValue, error) {
	if s.writes > 0 && s.readbackErr {
		return value.LabelValue{}, errors.New("readback unavailable")
	}
	return s.item, nil
}
func (s *updateDefinitionStub) SetLabelValue(_ context.Context, oldName string, v value.LabelValue) (value.LabelValue, error) {
	s.writes++
	s.oldName = oldName
	s.item = v
	return v, nil
}

func TestUpdatePreservesExistenceSensitiveVocabularyRules(t *testing.T) {
	for _, tc := range []struct {
		name   string
		before value.LabelValue
		input  UpdateInput
	}{
		{"rename absent", value.LabelValue{}, UpdateInput{Name: "Definition", NewName: new("Renamed")}},
		{"creation incomplete", value.LabelValue{}, UpdateInput{Name: "Definition", Description: new("New")}},
		{"category immutable", value.LabelValue{Name: "Definition", Category: "Original", Description: "Old"}, UpdateInput{Name: "Definition", Category: new("Other")}},
		{"internal immutable", value.LabelValue{Name: "Definition", Category: "Original", Description: "Old", Internal: true}, UpdateInput{Name: "Definition", Description: new("New")}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			s := &updateDefinitionStub{item: tc.before}
			if err := ValidateUpdateInput(tc.input); err != nil {
				t.Fatalf("invalid test input: %v", err)
			}
			if _, err := Update(t.Context(), s, s, tc.input, false); err == nil || s.writes != 0 {
				t.Fatalf("error=%v writes=%d", err, s.writes)
			}
		})
	}
}

func TestUpdateRenameKeepsOldIdentityForSubmission(t *testing.T) {
	s := &updateDefinitionStub{item: value.LabelValue{Name: "Definition", Category: "Category", Description: "Old", BuiltIn: true, ElevatedDefault: true}}
	in := UpdateInput{Name: "Definition", NewName: new("Renamed")}
	out, err := Update(t.Context(), s, s, in, false)
	if err != nil || s.oldName != "Definition" || s.writes != 1 || out.Result == nil || out.Result.Item.Name != "Renamed" || !out.Result.Item.BuiltIn || !out.Result.Item.ElevatedDefault {
		t.Fatalf("output=%+v error=%v writer=%+v", out, err, s)
	}
}
func TestUpdatePreviewAndConfirmedReceipt(t *testing.T) {
	for _, preview := range []bool{true, false} {
		s := &updateDefinitionStub{item: value.LabelValue{Name: "Definition", Description: "old", Category: "Category"}, readbackErr: true}
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
