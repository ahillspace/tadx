package app

import (
	"encoding/json"
	"testing"

	datasourcepull "github.com/ahillspace/tadx/actions/datasource/pull"
	flowpull "github.com/ahillspace/tadx/actions/flow/pull"
	workbookpull "github.com/ahillspace/tadx/actions/workbook/pull"
)

func TestCompactPullReceiptsRetainUsableKnownPaths(t *testing.T) {
	for _, result := range []interface{ CompactOutput() any }{
		workbookpull.Output{Artifact: workbookpull.ArtifactResult{Path: "artifacts/workbook/Book", CanonicalPath: "artifacts/workbook/Book/source.twbx"}},
		datasourcepull.Output{Artifact: datasourcepull.ArtifactResult{Path: "artifacts/datasource/Data", CanonicalPath: "artifacts/datasource/Data/source.tdsx"}},
		flowpull.Output{Artifact: flowpull.ArtifactResult{Path: "artifacts/flow/Flow", CanonicalPath: "artifacts/flow/Flow/source.tflx"}},
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
