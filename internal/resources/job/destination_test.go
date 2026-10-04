package job

import (
	"context"
	"testing"

	"github.com/ahillspace/tadx/internal/value"
)

type destinationFake struct {
	calls int
	input value.PublicationDestination
}

func (f *destinationFake) ResolvePublicationDestination(_ context.Context, input value.PublicationDestination) (value.ResourceDestination, error) {
	f.calls++
	f.input = input
	return value.ResourceDestination{ResourceID: input.ResourceID, Name: input.Name, ProjectID: input.ProjectID}, nil
}

func TestDestinationsUseOnlyTheSelectedResourceReader(t *testing.T) {
	for _, operation := range []string{"workbook.publish", "datasource.publish"} {
		workbook, datasource := &destinationFake{}, &destinationFake{}
		input := value.PublicationDestination{Operation: operation, ResourceID: "resource-1", Name: "Sales", ProjectID: "project-1"}
		result, err := (Destinations{Workbook: workbook, Datasource: datasource}).Resolve(t.Context(), input)
		selected, other := workbook, datasource
		if operation == "datasource.publish" {
			selected, other = datasource, workbook
		}
		if err != nil || selected.calls != 1 || other.calls != 0 || selected.input != input || result.ResourceID != "resource-1" {
			t.Fatalf("result=%+v err=%v selected=%+v other=%+v", result, err, selected, other)
		}
	}
}

func TestDestinationsRetainConfirmedIdentityWithoutInventingUnsupportedResolution(t *testing.T) {
	for _, id := range []string{"", "flow-1"} {
		result, err := (Destinations{}).Resolve(t.Context(), value.PublicationDestination{Operation: "flow.publish", ResourceID: id, Name: "Flow", ProjectID: "project-1"})
		if (err != nil) != (id == "") || result.ResourceID != id || result.Name != "Flow" || result.ProjectID != "project-1" {
			t.Fatalf("result=%+v err=%v", result, err)
		}
	}
}
