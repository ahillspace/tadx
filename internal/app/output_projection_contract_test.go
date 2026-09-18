package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	authcheck "github.com/ahillspace/tadx/actions/auth/check"
	columninspect "github.com/ahillspace/tadx/actions/catalog/column/inspect"
	tableinspect "github.com/ahillspace/tadx/actions/catalog/table/inspect"
	envupdate "github.com/ahillspace/tadx/actions/env/profile/update"
	jobinspect "github.com/ahillspace/tadx/actions/job/inspect"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/toon"
	"github.com/ahillspace/tadx/internal/value"
)

func TestCrossFamilyProjectionEncodingContracts(t *testing.T) {
	for name, result := range map[string]any{
		"auth":   authcheck.Output{Status: "authenticated", Environment: "dev", SiteContentURL: "site", UserLUID: "user"},
		"env":    envupdate.Output{Status: "updated", Profile: envupdate.Profile{Alias: "dev", ServerURL: "https://example.invalid", SiteContentURL: "site"}, ChangedFields: []string{"site_content_url"}},
		"table":  tableinspect.Output{Status: "inspected", Environment: "dev", Site: "site", Item: &value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "table-1"}, Database: value.MetadataIdentity{LUID: "database-1"}}},
		"column": columninspect.Output{Status: "inspected", Environment: "dev", Site: "site", Item: &value.MetadataColumn{MetadataIdentity: value.MetadataIdentity{LUID: "column-1"}, Table: value.MetadataIdentity{LUID: "table-1"}}},
		"job":    jobinspect.Output{Status: "running", Environment: "dev", Site: "site", Job: value.JobStatus{ID: "job-1", Status: "running"}},
	} {
		t.Run(name, func(t *testing.T) {
			var compact map[string]any
			for _, full := range []bool{false, true} {
				var expected any
				for _, asJSON := range []bool{false, true} {
					var out bytes.Buffer
					if err := output.RenderWithOptions(&out, result, output.Options{Full: full, JSON: asJSON}); err != nil {
						t.Fatal(err)
					}
					var decoded any
					var err error
					if asJSON {
						err = json.Unmarshal(out.Bytes(), &decoded)
					} else {
						decoded, err = toon.Decode(out.Bytes())
					}
					if err != nil {
						t.Fatal(err)
					}
					encoded, err := json.Marshal(decoded)
					if err != nil {
						t.Fatal(err)
					}
					var normalized map[string]any
					if err := json.Unmarshal(encoded, &normalized); err != nil {
						t.Fatal(err)
					}
					if asJSON && !reflect.DeepEqual(expected, normalized) {
						t.Fatalf("JSON changes semantics: %v != %v", expected, normalized)
					}
					expected = normalized
					if _, duplicate := normalized["parent"]; duplicate {
						t.Errorf("redundant catalog parent: %s", out.String())
					}
					if name == "table" || name == "column" {
						parent := "database"
						if name == "column" {
							parent = "table"
						}
						if _, ok := normalized["item"].(map[string]any)[parent].(map[string]any); !ok {
							t.Errorf("missing nested parent: %s", out.String())
						}
					}
					if !full {
						compact = normalized
					} else {
						for _, key := range []string{"status", "environment", "site", "item", "job"} {
							if before, ok := compact[key]; ok {
								if reflect.TypeOf(before) != reflect.TypeOf(normalized[key]) {
									t.Errorf("%s changes type between compact and full", key)
								}
							}
						}
					}
				}
			}
		})
	}
}
