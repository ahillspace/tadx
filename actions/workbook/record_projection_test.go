package workbook_test

import (
	"encoding/json"
	"strings"
	"testing"

	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/identity"
)

func TestSharedRecordDoesNotWidenMutationOrPullProjections(t *testing.T) {
	record := workbookops.Record{LUID: "wb-1", Name: "Finance", ProjectLUID: "p-1", ProjectPath: "Ops", OwnerLUID: "owner", Description: "description", ContentURL: "content", Tags: []string{"tag"}, RequestID: "request"}
	selector := identity.Selector{LUID: "wb-1"}
	deleted, err := workbookops.Delete(t.Context(), &deleteResolver{results: []workbookops.Record{record}}, &deleteDeleter{}, workbookops.DeleteInput{Environment: "test", Site: "site", Selector: selector}, true)
	if err != nil {
		t.Fatal(err)
	}
	moved, err := workbookops.Move(t.Context(), &moveResolver{workbook: record, project: workbookops.Project{LUID: "p-2", Path: "Destination"}}, &moveMover{}, workbookops.MoveInput{Environment: "test", Site: "site", WorkbookSelector: selector, ProjectSelector: identity.Selector{LUID: "p-2"}}, true)
	if err != nil {
		t.Fatal(err)
	}
	updated, err := workbookops.Update(t.Context(), updateResolver{item: record}, &updateUpdater{}, workbookops.UpdateInput{Environment: "test", Site: "site", Selector: selector, Name: new("Renamed")}, true)
	if err != nil {
		t.Fatal(err)
	}
	pulled := workbookops.PullOutput{Workbook: record}
	for name, projection := range map[string]any{"delete": deleted.FullOutput(), "move": moved.FullOutput(), "update": updated.FullOutput(), "pull": pulled.FullOutput()} {
		t.Run(name, func(t *testing.T) {
			encoded, err := json.Marshal(projection)
			if err != nil {
				t.Fatal(err)
			}
			for _, key := range []string{`"content_url"`, `"tags"`, `"RequestID"`} {
				if strings.Contains(string(encoded), key) {
					t.Fatalf("shared record widened %s projection: %s", name, encoded)
				}
			}
			if name != "update" && strings.Contains(string(encoded), `"description"`) {
				t.Fatalf("description leaked into %s projection: %s", name, encoded)
			}
			if (name == "delete" || name == "pull") && strings.Contains(string(encoded), `"owner_luid"`) {
				t.Fatalf("owner leaked into %s projection: %s", name, encoded)
			}
		})
	}
}
