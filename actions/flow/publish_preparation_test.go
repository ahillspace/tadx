package flow_test

import (
	"errors"
	"testing"

	flowops "github.com/ahillspace/tadx/actions/flow"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
)

func TestPublishPreparationRetainsDestinationGeneration(t *testing.T) {
	for _, original := range []string{"", "original"} {
		t.Run(original, func(t *testing.T) {
			reader := publishArtifactReader{artifact: flowops.PublishArtifact{Path: "artifact", PayloadPath: "Daily.tfl", Filename: "Daily.tfl", Name: "Daily", Fingerprint: "sha256:source"}}
			resolver := &publishResolver{project: flowops.Project{LUID: "project-1", Path: "Analytics"}}
			if original != "" {
				resolver.collisions = []flowops.Record{{LUID: original, Name: "Daily", ProjectLUID: "project-1"}}
			}
			writer := &publishPublisher{}
			action := flowops.NewPublish(reader, resolver, writer)
			input := flowops.PublishInput{ArtifactPath: "artifact", Environment: "production", Site: "site", ProjectSelector: identity.Selector{LUID: "project-1"}, Overwrite: true}
			preview, err := action.Execute(t.Context(), input, true)
			if err != nil {
				t.Fatal(err)
			}
			prepared := preview.Plan.PrepareInput(input)
			resolver.collisions = []flowops.Record{{LUID: "replacement", Name: "Daily", ProjectLUID: "project-1"}}
			_, err = action.Execute(t.Context(), prepared, false)
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.ID != "flow.publish.destination_changed" || writer.calls != 0 {
				t.Fatalf("writes=%d error=%v", writer.calls, err)
			}
		})
	}
}
