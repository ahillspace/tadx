package project_test

import (
	adapter "github.com/ahillspace/tadx/internal/tableau/project"
	"testing"
)

func TestTypedListFilter(t *testing.T) {
	got, err := adapter.ListFilter(adapter.ListRequest{Name: "Sales", OwnerName: "Owner", ParentLUID: "p1"})
	if err != nil || got != "name:eq:Sales,parentProjectId:eq:p1,ownerName:eq:Owner" {
		t.Fatalf("filter=%q error=%v", got, err)
	}
	if got, err := adapter.ListFilter(adapter.ListRequest{}); err != nil || got != "" {
		t.Fatalf("empty filter=%q error=%v", got, err)
	}
	for _, value := range []string{"One,Two", "One&Two"} {
		if _, err := adapter.ListFilter(adapter.ListRequest{Name: value}); err == nil {
			t.Fatalf("accepted delimiter %q", value)
		}
	}
}

func TestTypedProjectFilterPreservesExplicitBoolean(t *testing.T) {
	for _, value := range []bool{false, true} {
		got, err := adapter.ListFilter(adapter.ListRequest{TopLevel: &value})
		want := "topLevelProject:eq:false"
		if value {
			want = "topLevelProject:eq:true"
		}
		if err != nil || got != want {
			t.Fatalf("filter=%q error=%v", got, err)
		}
	}
}
