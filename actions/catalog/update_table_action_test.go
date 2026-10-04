package catalog

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type tableFixture struct {
	reads, writes int
	failTag       bool
	lastTagTarget value.LabelTarget
}

func (f *tableFixture) GetTable(context.Context, string) (value.MetadataTable, error) {
	f.reads++
	return value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "item", Name: "Fixture", Type: "table"}, TagsObserved: true}, nil
}
func (f *tableFixture) UpdateTable(_ context.Context, _ string, patch value.MetadataUpdate) (value.MetadataTable, error) {
	f.writes++
	return value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "item"}, Description: patch.Description, ContactLUID: tableDeref(patch.ContactLUID), TagsObserved: true}, nil
}
func tableDeref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (f *tableFixture) AddTags(_ context.Context, target value.LabelTarget, _ []string) ([]string, error) {
	f.writes++
	f.lastTagTarget = target
	if f.failTag {
		return nil, errors.New("tag failed")
	}
	return []string{"test"}, nil
}
func (f *tableFixture) DeleteTag(_ context.Context, target value.LabelTarget, _ string) error {
	f.writes++
	f.lastTagTarget = target
	return nil
}
func tableValid() TableInput {
	v := "Description"
	return TableInput{Environment: "dev", Site: "site", TargetResolved: true, ID: "item", Description: &v}
}
func TestTablePreviewDoesNotWrite(t *testing.T) {
	f := &tableFixture{}
	out, err := newTableRunner(f, f).Execute(context.Background(), tableValid(), true)
	if err != nil || f.writes != 0 || len(out.Plan.Changes) != 1 {
		t.Fatalf("%+v %v writes%d", out, err, f.writes)
	}
}
func TestTableLocalValidationDoesNotRead(t *testing.T) {
	f := &tableFixture{}
	in := tableValid()
	v := ""
	in.Description = &v
	_, err := newTableRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || f.reads != 0 {
		t.Fatal("unverified clear reached read")
	}
}
func TestTablePartialSuccessRetainsIdentity(t *testing.T) {
	f := &tableFixture{failTag: true}
	in := tableValid()
	in.AddTags = []string{"test"}
	out, err := newTableRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || out.Result == nil || out.Result.Identity.LUID != "item" || len(out.Result.Completed) != 1 || out.Result.Status != "partial" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestTableConflictingTagsRejected(t *testing.T) {
	f := &tableFixture{}
	in := tableValid()
	in.AddTags = []string{"x"}
	in.RemoveTags = []string{"x"}
	_, err := newTableRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || f.reads != 0 {
		t.Fatal("conflicting tags accepted")
	}
}
