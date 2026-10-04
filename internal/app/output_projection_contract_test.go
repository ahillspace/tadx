package app

import (
	"bytes"
	"encoding/json"
	"reflect"
	"testing"

	authcheck "github.com/ahillspace/tadx/actions/auth"
	catalogread "github.com/ahillspace/tadx/actions/catalog"
	envupdate "github.com/ahillspace/tadx/actions/env/profile"
	jobactions "github.com/ahillspace/tadx/actions/job"
	projectops "github.com/ahillspace/tadx/actions/project"

	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/toon"
	"github.com/ahillspace/tadx/internal/value"
)

func TestProjectMutationProjectionEncodingContracts(t *testing.T) {
	for _, test := range []struct {
		name          string
		value         any
		compact, full string
	}{
		{
			name: "create",
			value: projectops.CreateOutput{
				Plan:   projectops.CreatePlan{Mode: "execute", Operation: "project.create", Environment: "dev", Site: "site", Project: projectops.CreateProjectSpec{Name: "Created"}},
				Result: &projectops.CreateResult{Status: "succeeded", Project: projectops.CreateProject{LUID: "created", Name: "Created", Path: "Created", Description: "Full details"}, TableauRequestID: "request"},
				Help:   []string{"inspect"},
			},
			compact: `{"plan":{"mode":"execute","operation":"project.create","environment":"dev","site":"site","project":{"name":"Created"}},"result":{"status":"succeeded","project":{"luid":"created","name":"Created","path":"Created"}},"details":"--full","help":["inspect"]}`,
			full:    `{"plan":{"mode":"execute","operation":"project.create","environment":"dev","site":"site","project":{"name":"Created"}},"result":{"status":"succeeded","project":{"luid":"created","name":"Created","path":"Created","description":"Full details"},"tableau_request_id":"request"},"help":["inspect"]}`,
		},
		{
			name: "move",
			value: projectops.MoveOutput{
				Plan:   projectops.MovePlan{Mode: "execute", Operation: "project.move", Environment: "dev", Site: "site", Source: projectops.MoveProject{LUID: "moved", Name: "Moved", Path: "Parent/Moved", ParentLUID: "parent"}, TopLevel: true},
				Result: &projectops.MoveResult{Status: "succeeded", Project: projectops.MoveProject{LUID: "moved", Name: "Moved", Path: "Moved"}, TableauRequestID: "request"},
				Help:   []string{"inspect"},
			},
			compact: `{"plan":{"mode":"execute","operation":"project.move","environment":"dev","site":"site","source":{"luid":"moved","name":"Moved","path":"Parent/Moved","parent_luid":"parent"},"top_level":true,"no_op":false},"result":{"status":"succeeded","project":{"luid":"moved","name":"Moved","path":"Moved"}},"details":"--full","help":["inspect"]}`,
			full:    `{"plan":{"mode":"execute","operation":"project.move","environment":"dev","site":"site","source":{"luid":"moved","name":"Moved","path":"Parent/Moved","parent_luid":"parent"},"top_level":true,"no_op":false},"result":{"status":"succeeded","project":{"luid":"moved","name":"Moved","path":"Moved"},"tableau_request_id":"request"},"help":["inspect"]}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			for _, full := range []bool{false, true} {
				wantJSON := test.compact
				if full {
					wantJSON = test.full
				}
				var want any
				if err := json.Unmarshal([]byte(wantJSON), &want); err != nil {
					t.Fatal(err)
				}
				for _, asJSON := range []bool{false, true} {
					var rendered bytes.Buffer
					if err := output.RenderWithOptions(&rendered, test.value, output.Options{Full: full, JSON: asJSON}); err != nil {
						t.Fatal(err)
					}
					var decoded any
					var err error
					if asJSON {
						err = json.Unmarshal(rendered.Bytes(), &decoded)
					} else {
						decoded, err = toon.Decode(rendered.Bytes())
					}
					if err != nil {
						t.Fatal(err)
					}
					if !reflect.DeepEqual(want, decoded) {
						t.Errorf("full=%t json=%t projection keys/values differ: got=%#v want=%#v", full, asJSON, decoded, want)
					}
				}
			}
		})
	}
}

func TestCrossFamilyProjectionEncodingContracts(t *testing.T) {
	for name, result := range map[string]any{
		"auth":   authcheck.CheckOutput{Status: "authenticated", Environment: "dev", SiteContentURL: "site", UserLUID: "user"},
		"env":    envupdate.UpdateOutput{Status: "updated", Profile: envupdate.UpdateProfile{Alias: "dev", ServerURL: "https://example.invalid", SiteContentURL: "site"}, ChangedFields: []string{"site_content_url"}},
		"table":  catalogread.TableInspectOutput{Status: "inspected", Environment: "dev", Site: "site", Item: &value.MetadataTable{MetadataIdentity: value.MetadataIdentity{LUID: "table-1"}, Database: value.MetadataIdentity{LUID: "database-1"}}},
		"column": catalogread.ColumnInspectOutput{Status: "inspected", Environment: "dev", Site: "site", Item: &value.MetadataColumn{MetadataIdentity: value.MetadataIdentity{LUID: "column-1"}, Table: value.MetadataIdentity{LUID: "table-1"}}},
		"job":    jobactions.InspectOutput{Status: "running", Environment: "dev", Site: "site", Job: value.JobStatus{ID: "job-1", Status: "running"}},
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
