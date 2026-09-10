package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
)

// Exercise the user-visible failure: Tableau has physical upstream assets but
// no published-content neighbors, so the former query returned zero edges.
func TestPhysicalFlowLineageThroughCLIAndAutomaticPull(t *testing.T) {
	base, mutations := newGroupOneTableauServer(t)
	defer base.Close()
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/graphql" {
			base.Config.Handler.ServeHTTP(w, r)
			return
		}
		var body struct {
			Query string `json:"query"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Errorf("decode lineage query: %v", err)
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		root := map[string]any{"id": "flow-meta", "luid": "flow-1", "name": "Daily Prep"}
		for _, field := range []string{"upstreamDatabasesConnection", "upstreamTablesConnection", "downstreamDatabasesConnection", "downstreamTablesConnection", "upstreamDatasourcesConnection", "upstreamLinkedFlowsConnection", "downstreamDatasourcesConnection", "downstreamLinkedFlowsConnection", "downstreamWorkbooksConnection"} {
			if !strings.Contains(body.Query, field) {
				continue
			}
			nodes := []any{}
			if field == "upstreamDatabasesConnection" || field == "upstreamTablesConnection" {
				kind := "database"
				if field == "upstreamTablesConnection" {
					kind = "table"
				}
				for i := 1; i <= 6; i++ {
					nodes = append(nodes, map[string]any{"id": fmt.Sprintf("%s-meta-%d", kind, i), "name": fmt.Sprintf("Source %s %d", kind, i)})
				}
			}
			root[field] = map[string]any{"totalCount": len(nodes), "nodes": nodes, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil}}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"flowsConnection": map[string]any{"totalCount": 1, "nodes": []any{root}, "pageInfo": map[string]any{"hasNextPage": false, "endCursor": nil}}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	config := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	workspace := createNamedWorkspace(t, config, "physical-lineage")
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	options := app.Options{ConfigPath: config, HTTPClient: server.Client()}
	args := []string{"content", "lineage", "pull", "--workspace", "physical-lineage", "--kind", "flow", "--id", "flow-1", "--direction", "upstream"}
	compact := runGroupOneCLI(t, options, args...)
	for _, want := range []string{"complete: true", "node_count: 13", "edge_count: 12"} {
		if !strings.Contains(compact, want) {
			t.Fatalf("lineage missing %q:\n%s", want, compact)
		}
	}
	full := runGroupOneCLI(t, options, append(args, "--full")...)
	for _, want := range []string{"database-meta-6", "table-meta-6", "edge_count: 12"} {
		if !strings.Contains(full, want) {
			t.Fatalf("full lineage missing %q:\n%s", want, full)
		}
	}
	runGroupOneCLI(t, options, "content", "flow", "pull", "--workspace", "physical-lineage", "--id", "flow-1")
	graphs := 0
	err := filepath.WalkDir(filepath.Join(workspace, "artifacts"), func(path string, entry os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if entry.IsDir() || entry.Name() != "lineage.json" {
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		var graph artifact.LineageDocument
		if err := json.Unmarshal(data, &graph); err != nil {
			return err
		}
		if !graph.Complete || len(graph.Nodes) != 13 || len(graph.Edges) != 12 {
			t.Fatalf("persisted lineage = %#v", graph)
		}
		counts := map[string]int{}
		for _, node := range graph.Nodes {
			counts[node.Kind]++
			if node.Kind != "flow" && node.RESTLUID != "" {
				t.Fatalf("physical node has invented REST identity: %#v", node)
			}
		}
		if counts["database"] != 6 || counts["table"] != 6 {
			t.Fatalf("physical counts = %v", counts)
		}
		for _, edge := range graph.Edges {
			if edge.ToMetadataID != "flow-meta" || edge.FromMetadataID == "flow-meta" {
				t.Fatalf("wrong upstream direction: %#v", edge)
			}
		}
		graphs++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if graphs != 2 {
		t.Fatalf("persisted %d lineage graphs, want standalone and flow sidecar", graphs)
	}
	if mutations.Load() != 0 {
		t.Fatalf("lineage made %d remote mutations", mutations.Load())
	}
}
