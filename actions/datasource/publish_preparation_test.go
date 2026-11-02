package datasource_test

import (
	"errors"
	"testing"

	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

func TestPublishPreparationRetainsDestinationGeneration(t *testing.T) {
	for _, original := range []string{"", "original"} {
		t.Run(original, func(t *testing.T) {
			reader := &publishPublishArtifactReader{artifact: datasourceops.PublishArtifact{Path: "artifact", PayloadPath: "Sales.tds", Filename: "Sales.tds", Name: "Sales", Fingerprint: "sha256:source", CompositionStatus: "ordinary"}}
			resolver := &publishPublishResolver{project: datasourceops.Project{LUID: "project-1", Path: "Analytics"}}
			if original != "" {
				resolver.collisions = []datasourceops.Record{{LUID: original, Name: "Sales", ProjectLUID: "project-1"}}
			}
			writer := &publishPublisher{}
			action := datasourceops.NewPublish(reader, resolver, writer)
			input := datasourceops.PublishInput{ArtifactPath: "artifact", Environment: "production", Site: "site", ProjectSelector: identity.Selector{LUID: "project-1"}, Mode: datasourceops.ModeOverwrite}
			preview, err := action.Execute(t.Context(), input, true)
			if err != nil {
				t.Fatal(err)
			}
			prepared := preview.Plan.PrepareInput(input)
			resolver.collisions = []datasourceops.Record{{LUID: "replacement", Name: "Sales", ProjectLUID: "project-1"}}
			_, err = action.Execute(t.Context(), prepared, false)
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.ID != "datasource.publish.destination_changed" || writer.calls != 0 {
				t.Fatalf("writes=%d error=%v", writer.calls, err)
			}
		})
	}
}
