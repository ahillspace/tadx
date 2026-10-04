package catalog

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type databaseFixture struct {
	reads, writes int
	failTag       bool
	lastTagTarget value.LabelTarget
}

func (f *databaseFixture) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	f.reads++
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "item", Name: "Fixture", Type: "database"}, TagsObserved: true}, nil
}
func (f *databaseFixture) UpdateDatabase(_ context.Context, _ string, patch value.MetadataUpdate) (value.MetadataDatabase, error) {
	f.writes++
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "item"}, Description: patch.Description, ContactLUID: databaseDeref(patch.ContactLUID), TagsObserved: true}, nil
}
func databaseDeref(v *string) string {
	if v == nil {
		return ""
	}
	return *v
}
func (f *databaseFixture) AddTags(_ context.Context, target value.LabelTarget, _ []string) ([]string, error) {
	f.writes++
	f.lastTagTarget = target
	if f.failTag {
		return nil, errors.New("tag failed")
	}
	return []string{"test"}, nil
}
func (f *databaseFixture) DeleteTag(_ context.Context, target value.LabelTarget, _ string) error {
	f.writes++
	f.lastTagTarget = target
	return nil
}
func databaseValid() DatabaseInput {
	v := "Description"
	return DatabaseInput{Environment: "dev", Site: "site", TargetResolved: true, ID: "item", Description: &v}
}
func TestDatabasePreviewDoesNotWrite(t *testing.T) {
	f := &databaseFixture{}
	out, err := newDatabaseRunner(f, f).Execute(context.Background(), databaseValid(), true)
	if err != nil || f.writes != 0 || len(out.Plan.Changes) != 1 {
		t.Fatalf("%+v %v writes%d", out, err, f.writes)
	}
}
func TestDatabaseLocalValidationDoesNotRead(t *testing.T) {
	f := &databaseFixture{}
	in := databaseValid()
	v := ""
	in.Description = &v
	_, err := newDatabaseRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || f.reads != 0 {
		t.Fatal("unverified clear reached read")
	}
}
func TestDatabasePartialSuccessRetainsIdentity(t *testing.T) {
	f := &databaseFixture{failTag: true}
	in := databaseValid()
	in.AddTags = []string{"test"}
	out, err := newDatabaseRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || out.Result == nil || out.Result.Identity.LUID != "item" || len(out.Result.Completed) != 1 || out.Result.Status != "partial" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestDatabaseConflictingTagsRejected(t *testing.T) {
	f := &databaseFixture{}
	in := databaseValid()
	in.AddTags = []string{"x"}
	in.RemoveTags = []string{"x"}
	_, err := newDatabaseRunner(f, f).Execute(context.Background(), in, false)
	if err == nil || f.reads != 0 {
		t.Fatal("conflicting tags accepted")
	}
}
