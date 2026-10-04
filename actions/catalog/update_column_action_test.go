package catalog

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/value"
	"strings"
	"testing"
)

type columnFixture struct {
	reads, writes   int
	failTag         bool
	lastTagTarget   value.LabelTarget
	description     *string
	lastUpdate      value.MetadataUpdate
	omitDescription bool
}

func (f *columnFixture) GetColumn(context.Context, string, string) (value.MetadataColumn, error) {
	f.reads++
	return value.MetadataColumn{MetadataIdentity: value.MetadataIdentity{LUID: "item", Name: "Fixture", Type: "column"}, Table: value.MetadataIdentity{LUID: "table"}, Description: f.description, TagsObserved: true}, nil
}
func (f *columnFixture) UpdateColumn(_ context.Context, _, _ string, patch value.MetadataUpdate) (value.MetadataColumn, error) {
	f.writes++
	f.lastUpdate = patch
	description := patch.Description
	if f.omitDescription {
		description = nil
	}
	return value.MetadataColumn{MetadataIdentity: value.MetadataIdentity{LUID: "item"}, Table: value.MetadataIdentity{LUID: "table"}, Description: description, TagsObserved: true}, nil
}
func (f *columnFixture) AddTags(_ context.Context, target value.LabelTarget, _ []string) ([]string, error) {
	f.writes++
	f.lastTagTarget = target
	if f.failTag {
		return nil, errors.New("tag failed")
	}
	return []string{"test"}, nil
}
func (f *columnFixture) DeleteTag(_ context.Context, target value.LabelTarget, _ string) error {
	f.writes++
	f.lastTagTarget = target
	return nil
}
func columnValid() ColumnInput {
	v := "Description"
	return ColumnInput{Environment: "dev", Site: "site", TargetResolved: true, ID: "item", TableID: "table", Description: &v}
}
func TestColumnPreviewDoesNotWrite(t *testing.T) {
	f := &columnFixture{}
	out, err := newColumnRunner(f, f).Execute(context.Background(), columnValid(), true)
	if err != nil || f.writes != 0 || len(out.Plan.Changes) != 1 {
		t.Fatalf("%+v %v writes%d", out, err, f.writes)
	}
}
func TestColumnLocalValidationDoesNotRead(t *testing.T) {
	for _, invalid := range []string{" \t\n", strings.Repeat("x", 65537)} {
		f := &columnFixture{}
		in := columnValid()
		in.Description = &invalid
		_, err := newColumnRunner(f, f).Execute(context.Background(), in, false)
		if err == nil || f.reads != 0 {
			t.Fatal("invalid description reached read")
		}
	}
}

func TestColumnExplicitEmptyDescriptionClearsAndPreviews(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(map[bool]string{true: "preview", false: "execute"}[preview], func(t *testing.T) {
			original, empty := "Existing column meaning", ""
			f := &columnFixture{description: &original}
			in := columnValid()
			in.Description = &empty
			out, err := newColumnRunner(f, f).Execute(context.Background(), in, preview)
			if err != nil {
				t.Fatal(err)
			}
			if out.Plan.NoOp || len(out.Plan.Changes) != 1 || out.Plan.Changes[0].Property != "description" || out.Plan.Changes[0].Before == nil || *out.Plan.Changes[0].Before != original || out.Plan.Changes[0].After != "" {
				t.Fatalf("clear plan %+v", out.Plan)
			}
			if preview {
				if f.reads != 1 || f.writes != 0 || out.Result != nil {
					t.Fatalf("preview wrote or misreported: %+v reads=%d writes=%d", out, f.reads, f.writes)
				}
			} else if f.reads != 2 || f.writes != 1 || f.lastUpdate.Description == nil || *f.lastUpdate.Description != "" || f.lastUpdate.ContactLUID != nil || out.Result.Status != "updated" || out.Result.Description == nil || *out.Result.Description != "" || len(out.Result.Completed) != 1 || out.Result.Completed[0] != "description" {
				t.Fatalf("clear execution %+v patch=%+v reads=%d writes=%d", out, f.lastUpdate, f.reads, f.writes)
			}
		})
	}
}

func TestColumnEmptyDescriptionAlreadyObservedIsNoOp(t *testing.T) {
	empty := ""
	f := &columnFixture{description: &empty}
	in := columnValid()
	in.Description = &empty
	out, err := newColumnRunner(f, f).Execute(context.Background(), in, false)
	if err != nil || !out.Plan.NoOp || out.Result == nil || out.Result.Status != "unchanged" || f.writes != 0 {
		t.Fatalf("%+v %v writes=%d", out, err, f.writes)
	}
}

func TestColumnMissingReturnedDescriptionIsConfirmedVerificationFailure(t *testing.T) {
	f := &columnFixture{omitDescription: true}
	in := columnValid()
	out, err := newColumnRunner(f, f).Execute(context.Background(), in, false)
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Phase != errs.PhaseVerification || structured.Outcome != errs.OutcomeConfirmed || out.Result == nil || out.Result.Identity.LUID != "item" || out.Result.Description != nil {
		t.Fatalf("missing returned property was treated as success or plan echo: %+v %v", out, err)
	}
}

func TestColumnOmittedDescriptionIsNotAClear(t *testing.T) {
	original := "Preserve column meaning"
	f := &columnFixture{description: &original}
	in := columnValid()
	in.Description = nil
	if _, err := newColumnRunner(f, f).Execute(context.Background(), in, false); err == nil || f.reads != 0 {
		t.Fatal("omitting every requested change must fail without reading")
	}
	in.AddTags = []string{"test"}
	out, err := newColumnRunner(f, f).Execute(context.Background(), in, false)
	if err != nil || f.lastUpdate.Description != nil || f.writes != 1 || len(out.Plan.Changes) != 1 || out.Plan.Changes[0].Property != "add_tag" {
		t.Fatalf("tag-only update changed description: %+v %v patch=%+v", out, err, f.lastUpdate)
	}
}
func TestColumnPartialSuccessRetainsIdentity(t *testing.T) {
	f := &columnFixture{failTag: true}
	in := columnValid()
	in.AddTags = []string{"test"}
	out, err := newColumnRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || out.Result == nil || out.Result.Identity.LUID != "item" || len(out.Result.Completed) != 1 || out.Result.Status != "partial" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestColumnConflictingTagsRejected(t *testing.T) {
	f := &columnFixture{}
	in := columnValid()
	in.AddTags = []string{"x"}
	in.RemoveTags = []string{"x"}
	_, err := newColumnRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || f.reads != 0 {
		t.Fatal("conflicting tags accepted")
	}
}
