package app_test

import (
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

func TestLineageInvalidInputPrecedesWorkspaceSetup(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing-config.yaml")
	for _, tc := range []struct {
		name string
		args []string
	}{
		{"kind", []string{"--kind", "sheet", "--id", "item-1"}},
		{"direction", []string{"--kind", "flow", "--id", "item-1", "--direction", "sideways"}},
		{"depth", []string{"--kind", "flow", "--id", "item-1", "--depth", "4"}},
		{"flow fields", []string{"--kind", "flow", "--id", "item-1", "--fields"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out strings.Builder
			args := append([]string{"catalog", "lineage", "pull"}, tc.args...)
			args = append(args, "--json")
			if code := app.Run(t.Context(), args, &out, app.Options{ConfigPath: missing}); code == 0 || strings.Contains(out.String(), "lineage.pull.workspace") {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
		})
	}
	var out strings.Builder
	if code := app.Run(t.Context(), []string{"catalog", "lineage", "pull", "--kind", "flow", "--id", "item-1", "--json"}, &out, app.Options{ConfigPath: missing}); code == 0 || !strings.Contains(out.String(), "lineage.pull.workspace") {
		t.Fatalf("valid input setup: code=%d output=%s", code, out.String())
	}
}

func TestDatasourceFieldLineageRoundTripsThroughCLIAndArtifact(t *testing.T) {
	base, mutations := newGroupOneTableauServer(t)
	defer base.Close()
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/datasources/ds-1" {
			w.Header().Set("Content-Type", "application/xml")
			_, _ = fmt.Fprint(w, `<tsResponse><datasource id="ds-1" name="Sales"><project id="project-ops" name="Operations"/></datasource></tsResponse>`)
			return
		}
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
		root := map[string]any{"id": "ds-meta", "luid": "ds-1", "name": "Sales"}
		for _, field := range []string{"upstreamDatasourcesConnection", "upstreamFlowsConnection", "upstreamDatabasesConnection", "upstreamTablesConnection"} {
			if strings.Contains(body.Query, field) {
				root[field] = map[string]any{"totalCount": 0, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}
				break
			}
		}
		if strings.Contains(body.Query, "fieldsConnection") {
			root["fieldsConnection"] = map[string]any{"totalCount": 2, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{
				map[string]any{"__typename": "CalculatedField", "id": "calc-meta", "name": "Margin", "datasource": map[string]any{"id": "ds-meta"}, "fieldsConnection": map[string]any{"totalCount": 1, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{map[string]any{"id": "amount-meta", "name": "Amount", "datasource": map[string]any{"id": "ds-meta"}}}}, "upstreamColumnsConnection": map[string]any{"totalCount": 0, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}},
				map[string]any{"__typename": "ColumnField", "id": "amount-meta", "name": "Amount", "datasource": map[string]any{"id": "ds-meta"}, "upstreamColumnsConnection": map[string]any{"totalCount": 0, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{}}},
			}}
		}
		w.Header().Set("Content-Type", "application/json")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"publishedDatasourcesConnection": map[string]any{"totalCount": 1, "pageInfo": map[string]any{"hasNextPage": false}, "nodes": []any{root}}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	config := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	workspace := createNamedWorkspace(t, config, "fields-lineage")
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	options := app.Options{ConfigPath: config, HTTPClient: server.Client(), JobDirectory: t.TempDir()}
	var out strings.Builder
	if code := app.Run(t.Context(), []string{"catalog", "lineage", "pull", "--workspace", "fields-lineage", "--kind", "datasource", "--id", "ds-1", "--direction", "upstream", "--fields", "--full", "--json"}, &out, options); code != 0 {
		t.Fatalf("field lineage pull: code=%d output=%s", code, out.String())
	}
	var receipt struct {
		Artifact struct {
			Fields      bool   `json:"fields"`
			Complete    bool   `json:"complete"`
			LineagePath string `json:"lineage_path"`
		} `json:"artifact"`
		Nodes []artifact.LineageNode `json:"nodes"`
		Edges []artifact.LineageEdge `json:"edges"`
	}
	if err := json.Unmarshal([]byte(out.String()), &receipt); err != nil {
		t.Fatal(err)
	}
	if !receipt.Artifact.Fields || !receipt.Artifact.Complete || len(receipt.Nodes) != 3 || len(receipt.Edges) != 3 {
		t.Fatalf("field receipt = %s", out.String())
	}
	data, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(receipt.Artifact.LineagePath)))
	if err != nil {
		t.Fatal(err)
	}
	var graph artifact.LineageDocument
	if err := json.Unmarshal(data, &graph); err != nil {
		t.Fatal(err)
	}
	if !graph.Fields || !graph.Complete || len(graph.Nodes) != 3 || len(graph.Edges) != 3 {
		t.Fatalf("persisted field lineage = %#v", graph)
	}
	var last strings.Builder
	if code := app.Run(t.Context(), []string{"last", "--full", "--json"}, &last, options); code != 0 {
		t.Fatalf("saved field result: code=%d output=%s", code, last.String())
	}
	var saved struct {
		Result struct {
			Artifact struct {
				Fields bool `json:"fields"`
			} `json:"artifact"`
		} `json:"result"`
	}
	if err := json.Unmarshal([]byte(last.String()), &saved); err != nil || !saved.Result.Artifact.Fields {
		t.Fatalf("saved field result = %s, error = %v", last.String(), err)
	}
	if mutations.Load() != 0 {
		t.Fatalf("lineage made %d remote mutations", mutations.Load())
	}
}

func TestMalformedLineageRootKeepsObservableEmptyGraphContracts(t *testing.T) {
	base, mutations := newGroupOneTableauServer(t)
	defer base.Close()
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/metadata/graphql" {
			base.Config.Handler.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"data":{"flowsConnection":{"totalCount":1,"nodes":[{"id":"flow-meta","luid":"wrong-flow","name":"Daily Prep"}],"pageInfo":{"hasNextPage":false,"endCursor":null}}}}`)
	}))
	defer server.Close()
	config := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	workspace := createNamedWorkspace(t, config, "malformed-lineage")
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	options := app.Options{ConfigPath: config, HTTPClient: server.Client(), JobDirectory: t.TempDir()}
	var out strings.Builder
	args := []string{"catalog", "lineage", "pull", "--workspace", "malformed-lineage", "--kind", "flow", "--id", "flow-1", "--full", "--json"}
	if code := app.Run(t.Context(), args, &out, options); code != 0 {
		t.Fatalf("partial pull: code=%d output=%s", code, out.String())
	}
	var receipt struct {
		Status   string                     `json:"status"`
		Artifact map[string]json.RawMessage `json:"artifact"`
		Nodes    json.RawMessage            `json:"nodes"`
		Edges    json.RawMessage            `json:"edges"`
	}
	if err := json.Unmarshal([]byte(out.String()), &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Status != "pulled" || string(receipt.Nodes) != "null" || string(receipt.Edges) != "null" || string(receipt.Artifact["complete"]) != "false" {
		t.Fatalf("partial receipt = %s", out.String())
	}
	if _, ok := receipt.Artifact["node_count"]; ok {
		t.Fatalf("unknown node count appeared: %s", out.String())
	}
	if _, ok := receipt.Artifact["edge_count"]; ok {
		t.Fatalf("unknown edge count appeared: %s", out.String())
	}
	var sidecarPath string
	if err := json.Unmarshal(receipt.Artifact["lineage_path"], &sidecarPath); err != nil {
		t.Fatal(err)
	}
	sidecar, err := os.ReadFile(filepath.Join(workspace, filepath.FromSlash(sidecarPath)))
	if err != nil {
		t.Fatal(err)
	}
	var graph map[string]json.RawMessage
	if err := json.Unmarshal(sidecar, &graph); err != nil {
		t.Fatal(err)
	}
	if string(graph["nodes"]) != "[]" || string(graph["edges"]) != "[]" || string(graph["complete"]) != "false" {
		t.Fatalf("partial sidecar = %s", sidecar)
	}
	var last strings.Builder
	if code := app.Run(t.Context(), []string{"last", "--full", "--json"}, &last, options); code != 0 {
		t.Fatalf("saved result: code=%d output=%s", code, last.String())
	}
	var saved struct {
		Operation string          `json:"operation"`
		Result    json.RawMessage `json:"result"`
	}
	if err := json.Unmarshal([]byte(last.String()), &saved); err != nil {
		t.Fatal(err)
	}
	var savedResult map[string]json.RawMessage
	if err := json.Unmarshal(saved.Result, &savedResult); err != nil {
		t.Fatal(err)
	}
	if saved.Operation != "lineage.pull" || string(savedResult["nodes"]) != "null" || string(savedResult["edges"]) != "null" {
		t.Fatalf("saved partial result = %s", last.String())
	}
	if mutations.Load() != 0 {
		t.Fatalf("lineage made %d remote mutations", mutations.Load())
	}
}

// Exercise the user-visible failure: Tableau has physical upstream assets but
// no published-content neighbors, so the former query returned zero edges.
func TestPhysicalFlowLineageThroughCLIAndAutomaticPull(t *testing.T) {
	base, mutations := newGroupOneTableauServer(t)
	defer base.Close()
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	args := []string{"catalog", "lineage", "pull", "--workspace", "physical-lineage", "--kind", "flow", "--id", "flow-1", "--direction", "upstream"}
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

// Exercise the user-visible partial result: a root and one validated relation
// are retained when a later relationship is denied by the Metadata API.
func TestPartialFlowLineageThroughCLIAndArtifact(t *testing.T) {
	base, mutations := newGroupOneTableauServer(t)
	defer base.Close()
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		field := ""
		for _, candidate := range []string{"upstreamDatasourcesConnection", "upstreamLinkedFlowsConnection", "upstreamDatabasesConnection"} {
			if strings.Contains(body.Query, candidate) {
				field = candidate
				break
			}
		}
		if field == "upstreamDatabasesConnection" {
			w.Header().Set("Content-Type", "application/json")
			w.Header().Set("X-Tableau-Request-Id", "partial-lineage-database-request")
			if err := json.NewEncoder(w).Encode(map[string]any{"errors": []any{map[string]any{
				"message": "database relationship denied", "extensions": map[string]any{"code": "ACCESS_DENIED"},
			}}}); err != nil {
				t.Error(err)
			}
			return
		}
		if field == "upstreamDatasourcesConnection" {
			root[field] = map[string]any{
				"totalCount": 1,
				"pageInfo":   map[string]any{"hasNextPage": false, "endCursor": nil},
				"nodes":      []any{map[string]any{"id": "datasource-meta", "luid": "datasource-rest", "name": "Sales"}},
			}
		} else if field != "" {
			root[field] = map[string]any{
				"totalCount": 0,
				"pageInfo":   map[string]any{"hasNextPage": false, "endCursor": nil},
				"nodes":      []any{},
			}
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Tableau-Request-Id", "partial-lineage-request")
		if err := json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"flowsConnection": map[string]any{
			"totalCount": 1,
			"nodes":      []any{root},
			"pageInfo":   map[string]any{"hasNextPage": false, "endCursor": nil},
		}}}); err != nil {
			t.Error(err)
		}
	}))
	defer server.Close()
	config := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	workspace := createNamedWorkspace(t, config, "partial-lineage")
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	options := app.Options{ConfigPath: config, HTTPClient: server.Client()}
	args := []string{"catalog", "lineage", "pull", "--workspace", "partial-lineage", "--kind", "flow", "--id", "flow-1", "--direction", "upstream"}
	compact := runGroupOneCLI(t, options, args...)
	if !strings.Contains(compact, "complete: false") || !strings.Contains(compact, "lineage_path:") || strings.Contains(compact, "node_count:") || strings.Contains(compact, "edge_count:") {
		t.Fatalf("partial compact lineage = %s", compact)
	}
	full := runGroupOneCLI(t, options, append(args, "--full")...)
	for _, want := range []string{"complete: false", "datasource-meta", "upstreamDatabasesConnection", "partial-lineage-database-request", "provider: tableau-metadata", "root_rest_luid: flow-1"} {
		if !strings.Contains(full, want) {
			t.Fatalf("partial full lineage missing %q:\n%s", want, full)
		}
	}
	if strings.Contains(full, "node_count:") || strings.Contains(full, "edge_count:") {
		t.Fatalf("partial full lineage claimed unavailable counts:\n%s", full)
	}
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
		if graph.Complete || len(graph.Nodes) != 2 || len(graph.Edges) != 1 || graph.Failure == nil || graph.Failure.Relation != "upstreamDatabasesConnection" || graph.Failure.RequestID != "partial-lineage-database-request" {
			t.Fatalf("persisted partial lineage = %#v", graph)
		}
		seen := map[string]bool{}
		for _, node := range graph.Nodes {
			seen[node.MetadataID] = true
		}
		if !seen["flow-meta"] || !seen["datasource-meta"] {
			t.Fatalf("persisted partial lineage lost validated identities = %#v", graph.Nodes)
		}
		graphs++
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if graphs != 1 {
		t.Fatalf("persisted %d lineage graphs, want one", graphs)
	}
	if mutations.Load() != 0 {
		t.Fatalf("lineage made %d remote mutations", mutations.Load())
	}
}
