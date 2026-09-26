package app

import (
	"encoding/json"
	datasourceops "github.com/ahillspace/tadx/actions/datasource"
	flowops "github.com/ahillspace/tadx/actions/flow"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"testing"
)

func TestCompactPullReceiptsRetainUsableKnownPaths(t *testing.T) {
	for _, result := range []interface{ CompactOutput() any }{
		workbookops.PullOutput{Artifact: workbookops.PullArtifactResult{Path: "artifacts/workbook/Book", CanonicalPath: "artifacts/workbook/Book/source.twbx"}},
		datasourceops.PullOutput{Artifact: datasourceops.PullArtifactResult{Path: "artifacts/datasource/Data", CanonicalPath: "artifacts/datasource/Data/source.tdsx"}},
		flowops.PullOutput{Artifact: flowops.PullArtifactResult{Path: "artifacts/flow/Flow", CanonicalPath: "artifacts/flow/Flow/source.tflx"}},
	} {
		data, err := json.Marshal(result.CompactOutput())
		if err != nil {
			t.Fatal(err)
		}
		var object map[string]json.RawMessage
		if err := json.Unmarshal(data, &object); err != nil {
			t.Fatal(err)
		}
		// Other artifact fields include booleans; extract only the two path values.
		var fields map[string]json.RawMessage
		if err := json.Unmarshal(object["artifact"], &fields); err != nil {
			t.Fatal(err)
		}
		for _, key := range []string{"path", "canonical_path"} {
			var decodedPath string
			if err := json.Unmarshal(fields[key], &decodedPath); err != nil || decodedPath == "" {
				t.Fatalf("%T omitted usable %s: %s", result, key, data)
			}
		}
	}
}
