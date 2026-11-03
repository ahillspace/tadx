package content

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/batchinput"
)

func TestNativePublicationPreparationUsesResourceAndSlotConflicts(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, names := range [][2]string{{"Destination", "destination"}, {"\u03a3", "\u03c2"}, {"Before", "After"}} {
			t.Run(kind+"/"+names[0], func(t *testing.T) {
				ctx := batchinput.EnsurePublicationPreparation(t.Context())
				ctx, _ = batchinput.Collect(ctx, kind+".publish")
				seen := map[batchinput.Target]bool{}
				first := batchinput.PublishDestination{Environment: "test", Site: "site", ProjectLUID: "project", Name: names[0], ExistingLUID: "existing"}
				if err := recordPublication(ctx, kind, first, seen); err != nil {
					t.Fatal(err)
				}
				second := first
				second.Name = names[1]
				if err := recordPublication(ctx, kind, second, seen); err == nil || !strings.Contains(err.Error(), "same publication destination") {
					t.Fatalf("conflicting native selection admitted: %v", err)
				}
				if got := batchinput.PreparedPublications(ctx); len(got) != 1 || got[0] != first {
					t.Fatalf("exact prepared destination changed: %+v", got)
				}
			})
		}
	}
}
