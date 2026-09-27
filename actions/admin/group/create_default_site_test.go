package group

import (
	"context"
	"testing"
)

type defaultSiteBackend struct{ reads, writes int }

func (b *defaultSiteBackend) GroupExists(context.Context, string) (bool, error) {
	b.reads++
	return false, nil
}

func (b *defaultSiteBackend) CreateGroup(context.Context, CreateRequest) (Record, error) {
	b.writes++
	return Record{LUID: "group-1"}, nil
}

func TestDefaultSiteCreatePreviewKeepsEmptyContentURL(t *testing.T) {
	b := &defaultSiteBackend{}
	in := CreateInput{Environment: "dev", Name: "Group"}
	if err := ValidateCreateInput(in); err != nil {
		t.Fatal(err)
	}
	out, err := Create(t.Context(), b, b, in, true)
	if err != nil || out.Plan.Site != "" || b.reads != 1 || b.writes != 0 {
		t.Fatalf("output=%#v err=%v backend=%#v", out, err, b)
	}
}
