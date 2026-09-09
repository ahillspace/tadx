package flow_test

import (
	adapter "github.com/ahillspace/tadx/internal/tableau/flow"
	"testing"
)

func TestTypedListFilter(t *testing.T) {
	got, err := adapter.ListFilter(adapter.ListRequest{Name: "Sales", OwnerName: "Owner", ProjectLUID: "p1", ProjectName: "Project"})
	if err != nil || got != "name:eq:Sales,ownerName:eq:Owner,projectId:eq:p1,projectName:eq:Project" {
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
