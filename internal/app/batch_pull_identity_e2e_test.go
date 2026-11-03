package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

func TestLineageBatchResolvesNamedSourcesAndKindAliases(t *testing.T) {
	for _, tc := range []struct {
		name, items string
		count       int
	}{
		{"different names in one project", `[{"kind":"flow","name":"Daily Prep","project":"Department/Operations"},{"kind":"flow","name":"Daily Copy","project":"Department/Operations"}]`, 2},
		{"same name in different projects", `[{"kind":"flow","name":"Shared","project":"Department/Operations"},{"kind":"flow","name":"Shared","project":"Department/Destination"}]`, 2},
		{"distinct kinds sharing a textual ID", `[{"kind":"flow","id":"shared-1"},{"kind":"datasource","id":"shared-1"}]`, 2},
		{"normalized datasource kinds", `[{"kind":"datasource","id":"shared-1"},{"kind":"published_datasource","id":"shared-1"}]`, 0},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, mutations := newGroupOneTableauServer(t)
			defer base.Close()
			type source struct{ id, name, project string }
			sources := []source{{"flow-1", "Daily Prep", "project-ops"}, {"flow-2", "Daily Copy", "project-ops"}, {"flow-3", "Shared", "project-ops"}, {"flow-4", "Shared", "project-destination"}, {"shared-1", "Shared Identity", "project-ops"}}
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				prefix := "/api/3.29/sites/site-1/"
				if r.Method == http.MethodGet && (strings.HasPrefix(r.URL.Path, prefix+"flows") || strings.HasPrefix(r.URL.Path, prefix+"datasources")) {
					parts := strings.Split(strings.TrimPrefix(r.URL.Path, prefix), "/")
					kind := strings.TrimSuffix(parts[0], "s")
					w.Header().Set("Content-Type", "application/xml")
					if len(parts) == 1 {
						fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="%d"/><%s>`, r.URL.Query().Get("pageSize"), len(sources), parts[0])
					} else {
						fmt.Fprint(w, `<tsResponse>`)
					}
					for _, item := range sources {
						if len(parts) > 1 && parts[1] != item.id {
							continue
						}
						fmt.Fprintf(w, `<%s id="%s" name="%s" fileType="tfl"><project id="%s" name="Project"/></%s>`, kind, item.id, item.name, item.project, kind)
					}
					if len(parts) == 1 {
						fmt.Fprintf(w, `</%s>`, parts[0])
					}
					fmt.Fprint(w, `</tsResponse>`)
					return
				}
				if r.URL.Path == "/api/metadata/graphql" {
					var request struct {
						Query     string
						Variables map[string]any
					}
					if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
						t.Error(err)
						w.WriteHeader(400)
						return
					}
					id, _ := request.Variables["rootLuid"].(string)
					connection := "flowsConnection"
					if strings.Contains(request.Query, "publishedDatasourcesConnection(") {
						connection = "publishedDatasourcesConnection"
					}
					node := map[string]any{"id": connection + "-" + id, "luid": id, "name": "Root"}
					for _, relation := range []string{"upstreamDatabasesConnection", "upstreamTablesConnection", "downstreamDatabasesConnection", "downstreamTablesConnection", "upstreamDatasourcesConnection", "upstreamFlowsConnection", "upstreamLinkedFlowsConnection", "downstreamDatasourcesConnection", "downstreamLinkedFlowsConnection", "downstreamWorkbooksConnection"} {
						node[relation] = map[string]any{"totalCount": 0, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}
					}
					if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{connection: map[string]any{"totalCount": 1, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{node}}}}); err != nil {
						t.Error(err)
					}
					return
				}
				base.Config.Handler.ServeHTTP(w, r)
			}))
			defer server.Close()
			options := diagnosticOptions(t, server)
			root := createNamedWorkspace(t, options.ConfigPath, "work")
			var out bytes.Buffer
			code := app.Run(t.Context(), []string{"catalog", "lineage", "pull", "--environment", "test", "--workspace", "work", "--batch-file", writePullIdentityBatch(t, tc.items), "--json"}, &out, options)
			if tc.count == 0 {
				if code == 0 || !strings.Contains(out.String(), "duplicates") {
					t.Fatalf("code=%d output=%s", code, out.String())
				}
			} else if code != 0 {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
			paths := requirePullIdentityArtifacts(t, root, "lineage/*", tc.count)
			for _, path := range paths {
				data, err := os.ReadFile(filepath.Join(path, "metadata.json"))
				var metadata artifact.LineageMetadata
				if err != nil || json.Unmarshal(data, &metadata) != nil || metadata.TableauID == "" || metadata.ResourceKind == "" {
					t.Fatalf("metadata=%s err=%v", data, err)
				}
				graph, err := os.ReadFile(filepath.Join(path, "lineage.json"))
				if err != nil || !bytes.Contains(graph, []byte(`"rest_luid": "`+metadata.TableauID+`"`)) {
					t.Fatalf("graph=%s err=%v", graph, err)
				}
			}
			if mutations.Load() != 0 {
				t.Fatalf("remote mutations=%d", mutations.Load())
			}
		})
	}
}

func TestLineageBatchUsesResolvedLocalDestinations(t *testing.T) {
	for _, tc := range []struct {
		name      string
		items     string
		duplicate bool
	}{
		{"independent workspaces", `[{"id":"flow-1","workspace":"first"},{"id":"flow-1","workspace":"second"}]`, false},
		{"id and name aliases", `[{"id":"flow-1","workspace":"first"},{"name":"Daily Prep","project":"Department/Operations","workspace":"first"}]`, true},
		{"default workspace alias", `[{"id":"flow-1","workspace":"first"},{"id":"flow-1"}]`, true},
		{"different projections still own one output", `[{"id":"flow-1","workspace":"first","direction":"upstream"},{"id":"flow-1","workspace":"first","direction":"downstream"}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server, mutations := newGroupOneTableauServer(t)
			defer server.Close()
			options := diagnosticOptions(t, server)
			first := createNamedWorkspace(t, options.ConfigPath, "first")
			second := createNamedWorkspace(t, options.ConfigPath, "second")
			runGroupOneCLI(t, options, "workspace", "set-default", "first")
			batch := writePullIdentityBatch(t, tc.items)
			var out bytes.Buffer
			code := app.Run(t.Context(), []string{"catalog", "lineage", "pull", "--kind", "flow", "--environment", "test", "--batch-file", batch, "--json"}, &out, options)
			if tc.duplicate {
				if code == 0 || !strings.Contains(out.String(), "duplicates") {
					t.Fatalf("code=%d output=%s", code, out.String())
				}
				requirePullIdentityArtifacts(t, first, "lineage/flow", 0)
				requirePullIdentityArtifacts(t, second, "lineage/flow", 0)
			} else {
				if code != 0 {
					t.Fatalf("code=%d output=%s", code, out.String())
				}
				for _, root := range []string{first, second} {
					paths := requirePullIdentityArtifacts(t, root, "lineage/flow", 1)
					data, err := os.ReadFile(filepath.Join(paths[0], "lineage.json"))
					if err != nil || !bytes.Contains(data, []byte(`"rest_luid": "flow-1"`)) {
						t.Fatalf("lineage=%s err=%v", data, err)
					}
				}
			}
			if mutations.Load() != 0 {
				t.Fatalf("remote mutations=%d", mutations.Load())
			}
		})
	}
}

func TestPulsePullBatchUsesResolvedLocalDestinations(t *testing.T) {
	for _, tc := range []struct {
		name, items string
		duplicate   bool
	}{
		{"independent workspaces", `[{"workspace":"portable"},{"workspace":"second"}]`, false},
		{"default workspace alias", `[{"workspace":"portable"},{}]`, true},
		{"different overwrite settings still own one output", `[{"workspace":"portable","overwrite":false},{"workspace":"portable","overwrite":true}]`, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			fixture := newPulsePreflightFixture(t)
			second := createNamedWorkspace(t, fixture.options.ConfigPath, "second")
			runGroupOneCLI(t, fixture.options, "workspace", "set-default", "portable")
			before, err := os.ReadFile(filepath.Join(fixture.directory, "bundle.json"))
			if err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := app.Run(t.Context(), []string{"pulse", "definition", "pull", "--id", "source-definition", "--batch-file", writePullIdentityBatch(t, tc.items), "--json"}, &out, fixture.options)
			if tc.duplicate {
				if code == 0 || !strings.Contains(out.String(), "duplicates") {
					t.Fatalf("code=%d output=%s", code, out.String())
				}
				requirePullIdentityArtifacts(t, second, "pulse-definition", 0)
				after, err := os.ReadFile(filepath.Join(fixture.directory, "bundle.json"))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("existing bundle changed: err=%v", err)
				}
			} else {
				if code != 0 {
					t.Fatalf("code=%d output=%s", code, out.String())
				}
				paths := requirePullIdentityArtifacts(t, second, "pulse-definition", 1)
				after, err := os.ReadFile(filepath.Join(paths[0], "bundle.json"))
				if err != nil || !bytes.Equal(before, after) {
					t.Fatalf("independent bundle differs: err=%v", err)
				}
			}
		})
	}
}

func writePullIdentityBatch(t *testing.T, items string) string {
	t.Helper()
	if !json.Valid([]byte(items)) {
		t.Fatal("invalid batch fixture")
	}
	path := filepath.Join(t.TempDir(), "batch.json")
	if err := os.WriteFile(path, []byte(`{"items":`+items+`}`), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func requirePullIdentityArtifacts(t *testing.T, root, kind string, count int) []string {
	t.Helper()
	paths, err := filepath.Glob(filepath.Join(root, "artifacts", filepath.FromSlash(kind), "*"))
	if err != nil || len(paths) != count {
		t.Fatalf("artifacts=%v want=%d err=%v", paths, count, err)
	}
	return paths
}
