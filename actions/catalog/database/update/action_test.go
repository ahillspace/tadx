package update

import (
	"context"
	"errors"
	"github.com/ahillspace/tadx/internal/value"
	"testing"
)

type fixture struct {
	reads, writes int
	failTag       bool
}

func (f *fixture) GetDatabase(context.Context, string) (value.MetadataDatabase, error) {
	f.reads++
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "item", Name: "Fixture", Type: "database"}, TagsObserved: true}, nil
}
func (f *fixture) UpdateDatabase(context.Context, string, value.MetadataUpdate) (value.MetadataDatabase, error) {
	f.writes++
	return value.MetadataDatabase{MetadataIdentity: value.MetadataIdentity{LUID: "item"}}, nil
}
func (f *fixture) AddDatabaseTags(context.Context, string, []string) ([]string, error) {
	f.writes++
	if f.failTag {
		return nil, errors.New("tag failed")
	}
	return []string{"test"}, nil
}
func (f *fixture) DeleteDatabaseTag(context.Context, string, string) error { f.writes++; return nil }
func valid() Input {
	v := "Description"
	return Input{Environment: "dev", Site: "site", TargetResolved: true, ID: "item", Description: &v}
}
func TestPreviewDoesNotWrite(t *testing.T) {
	f := &fixture{}
	out, err := New(f, f).Execute(context.Background(), valid(), true)
	if err != nil || f.writes != 0 || len(out.Plan.Changes) != 1 {
		t.Fatalf("%+v %v writes%d", out, err, f.writes)
	}
}
func TestLocalValidationDoesNotRead(t *testing.T) {
	f := &fixture{}
	in := valid()
	v := ""
	in.Description = &v
	_, err := New(f, f).Execute(context.Background(), in, false)
	if err == nil || f.reads != 0 {
		t.Fatal("unverified clear reached read")
	}
}
func TestPartialSuccessRetainsIdentity(t *testing.T) {
	f := &fixture{failTag: true}
	in := valid()
	in.AddTags = []string{"test"}
	out, err := New(f, f).Execute(context.Background(), in, false)
	if err == nil || out.Result == nil || out.Result.Identity.LUID != "item" || len(out.Result.Completed) != 1 || out.Result.Status != "partial" {
		t.Fatalf("%+v %v", out, err)
	}
}
func TestConflictingTagsRejected(t *testing.T) {
	f := &fixture{}
	in := valid()
	in.AddTags = []string{"x"}
	in.RemoveTags = []string{"x"}
	_, err := New(f, f).Execute(context.Background(), in, false)
	if err == nil || f.reads != 0 {
		t.Fatal("conflicting tags accepted")
	}
}
