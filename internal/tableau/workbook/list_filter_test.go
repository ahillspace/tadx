package workbook_test

import (
	adapter "github.com/ahillspace/tadx/internal/tableau/workbook"
	"testing"
)

func TestTypedListFilter(t *testing.T) {
	got, err := adapter.ListFilter(adapter.ListRequest{Name: "Sales", OwnerName: "Owner", ProjectLUID: "project-1", ProjectName: "Project", Tag: "Tag"})
	if err != nil || got != "name:eq:Sales,ownerName:eq:Owner,projectName:eq:Project,tags:eq:Tag" {
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
	for _, value := range []string{"One,Two", "One&Two"} {
		if _, err := adapter.ListFilter(adapter.ListRequest{ProjectLUID: value}); err == nil {
			t.Fatalf("accepted project ID delimiter %q", value)
		}
	}
}
