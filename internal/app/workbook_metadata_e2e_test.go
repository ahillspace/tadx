package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/actions/catalog/audit"
	workbookops "github.com/ahillspace/tadx/actions/workbook"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/vektah/gqlparser/v2/ast"
	"github.com/vektah/gqlparser/v2/parser"
)

func TestWorkbookMetadataCommandsThroughHTTP(t *testing.T) {
	var fieldPageFailure atomic.Bool
	var graphCalls atomic.Int32
	var vdsCalls atomic.Int32
	var rejectSheets atomic.Bool
	var metadataHTTPFailure atomic.Int32
	var hiddenSourceMode atomic.Int32
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/3.29/auth/signin" {
			fmt.Fprint(w, `{"credentials":{"token":"fixture-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/workbooks/wb-1":
			if r.Method != http.MethodGet {
				t.Errorf("unexpected workbook method %s", r.Method)
			}
			fmt.Fprint(w, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook></tsResponse>`)
		case "/api/3.29/sites/site-1/projects":
			fmt.Fprint(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
		case "/api/3.29/sites/site-1/datasources/ds-1":
			fmt.Fprint(w, `<tsResponse><datasource id="ds-1" name="Published"><project id="project-1" name="Operations"/></datasource></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			vdsCalls.Add(1)
			if hiddenSourceMode.Load() == 1 || hiddenSourceMode.Load() == 2 {
				http.Error(w, "VDS unavailable", http.StatusNotFound)
				return
			}
			var body struct {
				Datasource struct {
					LUID string `json:"datasourceLuid"`
				} `json:"datasource"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Datasource.LUID != "ds-1" {
				t.Errorf("VDS datasource selector: %+v, %v", body, err)
			}
			if hiddenSourceMode.Load() == 3 {
				fmt.Fprint(w, `{"data":[{"fieldName":"Revenue","fieldCaption":"Revenue","dataType":"REAL","fieldRole":"MEASURE","defaultAggregation":"SUM","columnClass":"COLUMN","isHidden":true,"isInternal":false,"logicalTableId":"Orders_A1B2C3D4"}]}`)
			} else {
				fmt.Fprint(w, `{"data":[{"fieldName":"Revenue","fieldCaption":"Revenue","dataType":"REAL","fieldRole":"MEASURE","defaultAggregation":"AGG","columnClass":"CALCULATION","logicalTableId":"Orders_A1B2C3D4"}]}`)
			}
		case "/api/v1/vizql-data-service/describe-datasource":
			http.Error(w, "VDS unavailable", http.StatusNotFound)
		case "/api/metadata/graphql":
			graphCalls.Add(1)
			if status := metadataHTTPFailure.Load(); status != 0 {
				w.Header().Set("X-Tableau-Request-Id", "metadata-retry-request")
				http.Error(w, "metadata temporarily unavailable", int(status))
				return
			}
			var request struct {
				Query     string         `json:"query"`
				Variables map[string]any `json:"variables"`
			}
			if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
				t.Error(err)
				http.Error(w, "invalid query", 500)
				return
			}
			q := request.Query
			if rejectSheets.Load() && strings.Contains(q, "downstreamSheetsConnection") {
				http.Error(w, "unrelated sheet pagination unavailable", http.StatusServiceUnavailable)
				return
			}
			if _, err := parser.ParseQuery(&ast.Source{Name: "generated.graphql", Input: q}); err != nil {
				t.Errorf("invalid generated GraphQL: %v", err)
				http.Error(w, "invalid GraphQL", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			switch {
			case strings.Contains(q, "query GetDatasourceFields"):
				formula := "SUM([Sales])"
				if hiddenSourceMode.Load() == 1 {
					formula = "WINDOW_SUM(SUM([Sales]))"
				}
				fmt.Fprintf(w, `{"data":{"publishedDatasources":[{"luid":"ds-1","name":"Published","fieldsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"CalculatedField","id":"remote-meta","name":"Revenue","fullyQualifiedName":"[Revenue]","role":"MEASURE","dataType":"REAL","aggregation":"AGG","formula":%q,"isHidden":true}]}}]}}`, formula)
			case strings.Contains(q, "workbooksConnection"):
				if request.Variables["luid"] != "wb-1" {
					t.Errorf("workbook selector: %+v", request.Variables)
				}
				fmt.Fprint(w, `{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"wb-meta","luid":"wb-1","name":"Finance","description":"Workbook meaning","tagsConnection":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"name":"reviewed"}]},"items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","hasExtracts":false}]}}]}}}`)
			case strings.Contains(q, "embeddedDatasourcesConnection"):
				if request.Variables["id"] != "embedded-meta" {
					t.Errorf("embedded selector: %+v", request.Variables)
				}
				items := `{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}`
				switch {
				case strings.Contains(q, "items:fieldsConnection"):
					if fieldPageFailure.Load() && request.Variables["after"] != nil {
						fmt.Fprint(w, `{"errors":[{"message":"fixture later field page unavailable"}]}`)
						return
					}
					items = `{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"DatasourceField","id":"local-meta","name":"Local Revenue","fullyQualifiedName":"[Revenue]","description":"Local meaning","remoteField":{"__typename":"ColumnField","id":"remote-meta","name":"Revenue"},"descriptionInherited":[],"relatedSheets":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"sheet-meta","luid":"sheet-1","name":"Overview"}]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}},{"__typename":"CalculatedField","id":"hidden-meta","name":"Private Calc","fullyQualifiedName":"[Private Calc]","description":"","isHidden":true,"formula":"[Revenue] * 2","role":"MEASURE","dataType":"REAL","descriptionInherited":[],"referencedFields":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"local-meta","name":"Local Revenue"}]},"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}`
					if fieldPageFailure.Load() {
						var page map[string]any
						if err := json.Unmarshal([]byte(items), &page); err != nil {
							t.Fatal(err)
						}
						page["nodes"] = page["nodes"].([]any)[:1]
						page["pageInfo"] = map[string]any{"hasNextPage": true, "endCursor": "fields-next"}
						encoded, _ := json.Marshal(page)
						items = string(encoded)
					}
					if hiddenSourceMode.Load() != 0 {
						items = strings.Replace(items, `"name":"Local Revenue",`, `"name":"Local Revenue","role":"MEASURE","dataType":"REAL",`, 1)
					}
					if hiddenSourceMode.Load() == 3 {
						items = strings.Replace(items, `"isHidden":true`, `"isHidden":false`, 1)
					}
				case strings.Contains(q, "items:parentPublishedDatasourcesConnection"):
					items = `{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"published-meta","luid":"ds-1","name":"Published"}]}`
				}
				fmt.Fprintf(w, `{"data":{"embedded":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"embedded-meta","name":"Local","items":%s}]}}}`, items)
			case strings.Contains(q, "publishedDatasourcesConnection"):
				if request.Variables["parent"] != "ds-1" {
					t.Errorf("published selector: %+v", request.Variables)
				}
				items := `{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}`
				if strings.Contains(q, "items:fieldsConnection") {
					items = `{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"ColumnField","id":"remote-meta","name":"Revenue","fullyQualifiedName":"[Revenue]","description":"Published meaning","role":"MEASURE","dataType":"REAL","aggregation":"SUM","descriptionInherited":[],"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}`
					if hiddenSourceMode.Load() != 0 {
						formula := "SUM([Sales])"
						if hiddenSourceMode.Load() == 1 {
							formula = "WINDOW_SUM(SUM([Sales]))"
						}
						items = fmt.Sprintf(`{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"CalculatedField","id":"remote-meta","name":"Revenue","fullyQualifiedName":"[Revenue]","description":"Published meaning","role":"MEASURE","dataType":"REAL","aggregation":"AGG","formula":%q,"isHidden":true,"descriptionInherited":[],"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}`, formula)
					}
				}
				fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"id":"published-meta","luid":"ds-1","name":"Published","description":"Published source","tagsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"items":%s}]}}}`, items)
			default:
				t.Errorf("unexpected GraphQL query: %s", q)
				http.Error(w, "unexpected query", 500)
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
			http.Error(w, "unexpected request", 500)
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	config := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: ''\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: WORKBOOK_PAT_NAME\n      pat_secret_env: WORKBOOK_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(config), 0600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("WORKBOOK_PAT_NAME", "fixture-name")
	t.Setenv("WORKBOOK_PAT_SECRET", "fixture-secret")
	userHome := t.TempDir()
	options := Options{ConfigPath: configPath, HTTPClient: server.Client(), UserHomeDir: func() (string, error) { return userHome, nil }, managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"workbook.schema": true, "workbook.inspect": true, "catalog.audit": true}}}
	resultJSON := func(t *testing.T, args []string) (int, map[string]any) {
		t.Helper()
		var output bytes.Buffer
		code := Run(t.Context(), append(args, "--json"), &output, options)
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatalf("invalid JSON (exit %d): %s: %v", code, output.String(), err)
		}
		return code, payload
	}
	for _, tc := range []struct {
		name     string
		args     []string
		required []string
	}{
		{"schema", []string{"content", "workbook", "schema", "--id", "wb-1", "--descriptions", "--full"}, []string{"Local Revenue", "Local meaning", "Published meaning", "Private Calc", "remote-meta", "vds_read_metadata", "Orders_A1B2C3D4", "measure", "REAL"}},
		{"audit", []string{"catalog", "audit", "--type", "workbook", "--id", "wb-1", "--full"}, []string{"local-meta", "published-meta", "missing", "present"}},
		{"inspect", []string{"content", "workbook", "inspect", "--id", "wb-1", "--full"}, []string{"embedded-meta", "published-meta", "field_count", "ds-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, payload := resultJSON(t, append(tc.args, "--env", "test"))
			if code != 0 {
				t.Fatalf("exit %d: %+v", code, payload)
			}
			encoded, _ := json.Marshal(payload)
			switch tc.name {
			case "schema":
				var result workbookops.SchemaOutput
				if err := json.Unmarshal(encoded, &result); err != nil {
					t.Fatal(err)
				}
				if result.Status != "listed" || !result.Complete || !result.Page.TotalComplete || result.Page.Total != 3 || result.Page.Returned != 3 || len(result.Datasources) != 2 {
					t.Fatalf("schema coverage: %+v", result)
				}
				local, source := result.Datasources[0], result.Datasources[1]
				if local.Kind != "embedded" || len(local.Fields) != 2 || source.Kind != "published" || len(source.Fields) != 1 || len(local.Parents) != 1 || local.Parents[0].MetadataID != source.Identity.MetadataID {
					t.Fatalf("schema provenance: %+v", result.Datasources)
				}
				field := source.Fields[0]
				if vdsCalls.Load() != 1 || field.DefaultAggregation != "AGG" || !field.RequiresUserAggregation || field.Provenance != "vds_read_metadata" || field.LogicalTableID != "Orders_A1B2C3D4" || field.Metadata == nil || field.Metadata.MetadataID != "remote-meta" {
					t.Fatalf("published schema bypassed canonical VDS semantics: %+v, calls %d", field, vdsCalls.Load())
				}
			case "audit":
				var result audit.Output
				if err := json.Unmarshal(encoded, &result); err != nil {
					t.Fatal(err)
				}
				if !result.Complete || result.Scanned != 5 || len(result.Groups) != 3 || result.Summary.Present != 5 || result.Summary.Missing != 2 || result.Summary.Unknown != 0 {
					t.Fatalf("audit coverage: %+v", result)
				}
			case "inspect":
				var result workbookops.InspectFullResult
				if err := json.Unmarshal(encoded, &result); err != nil {
					t.Fatal(err)
				}
				if result.Status != "found" || result.DatasourceStatus != "complete" || len(result.Datasources) != 2 || result.Datasources[0].FieldCount != 2 || result.Datasources[1].FieldCount != 1 {
					t.Fatalf("inspection coverage: %+v", result)
				}
			}
			for _, want := range tc.required {
				if !strings.Contains(string(encoded), want) {
					t.Errorf("missing %q: %s", want, encoded)
				}
			}
		})
	}
	t.Run("VDS source display hiding does not exclude local fields", func(t *testing.T) {
		hiddenSourceMode.Store(3)
		defer hiddenSourceMode.Store(0)
		code, payload := resultJSON(t, []string{"content", "workbook", "schema", "--id", "wb-1", "--env", "test", "--full"})
		encoded, _ := json.Marshal(payload)
		var result workbookops.SchemaOutput
		if err := json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
		if code != 0 || !result.Complete || len(result.Datasources) != 2 {
			t.Fatalf("VDS schema coverage changed: exit %d: %+v", code, payload)
		}
		for _, group := range result.Datasources {
			for _, field := range group.Fields {
				if group.Kind == "embedded" && (field.Excluded || field.Role != "measure") {
					t.Fatalf("source display hiding became a local semantic exclusion: %+v", field)
				}
				if group.Kind == "published" && (!field.Excluded || field.ExclusionReason != "internal") {
					t.Fatalf("canonical datasource display contract changed: %+v", field)
				}
			}
		}
	})
	t.Run("audit ignores unassessed sheet relationships", func(t *testing.T) {
		rejectSheets.Store(true)
		defer rejectSheets.Store(false)
		code, payload := resultJSON(t, []string{"catalog", "audit", "--type", "workbook", "--id", "wb-1", "--env", "test", "--full"})
		encoded, _ := json.Marshal(payload)
		var result audit.Output
		if err := json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
		if code != 0 || !result.Complete || result.Scanned != 5 {
			t.Fatalf("audit depended on unassessed sheets: exit %d: %+v", code, payload)
		}
		for _, finding := range result.Findings {
			if finding.Type != "workbook" && finding.Type != "datasource" && finding.Type != "field" {
				t.Errorf("noncanonical finding type: %+v", finding)
			}
		}
	})
	t.Run("filtered partial schema preserves known fields", func(t *testing.T) {
		fieldPageFailure.Store(true)
		defer fieldPageFailure.Store(false)
		code, payload := resultJSON(t, []string{"content", "workbook", "schema", "--id", "wb-1", "--env", "test", "--datasource", "Local", "--field-id", "Revenue", "--field-id", "Absent", "--full"})
		failure, _ := payload["error"].(map[string]any)
		encoded, _ := json.Marshal(payload["output"])
		var result workbookops.SchemaOutput
		if err := json.Unmarshal(encoded, &result); err != nil {
			t.Fatal(err)
		}
		if code == 0 || failure["kind"] != "operation" || result.Status != "partial" || result.Complete || result.Page.TotalComplete || result.Page.Returned != 1 || len(result.Datasources) != 1 {
			t.Fatalf("filtered provider failure lost or mislabeled: exit %d, %+v", code, payload)
		}
		if result.Datasources[0].Fields[0].Metadata.MetadataID != "local-meta" {
			t.Fatalf("confirmed local field lost: %+v", result)
		}
	})
	for _, status := range []int{http.StatusTooManyRequests, http.StatusServiceUnavailable} {
		t.Run(fmt.Sprintf("schema preserves HTTP %d retry advice", status), func(t *testing.T) {
			metadataHTTPFailure.Store(int32(status))
			defer metadataHTTPFailure.Store(0)
			code, payload := resultJSON(t, []string{"content", "workbook", "schema", "--id", "wb-1", "--env", "test"})
			failure, _ := payload["error"].(map[string]any)
			if code == 0 || failure["kind"] != "operation" || failure["retryable"] != true || failure["tableau_request_id"] != "metadata-retry-request" || strings.Contains(fmt.Sprint(failure["corrective_action"]), "correct the exact selectors") {
				t.Fatalf("provider retry advice lost: exit %d: %+v", code, payload)
			}
		})
	}
	for _, tc := range []struct {
		name        string
		mode        int32
		exclusion   string
		aggregation bool
	}{
		{"table calculation", 1, "table_calc", false},
		{"aggregate calculation", 2, "", true},
	} {
		t.Run("hidden published "+tc.name+" survives GraphQL fallback", func(t *testing.T) {
			hiddenSourceMode.Store(tc.mode)
			defer hiddenSourceMode.Store(0)
			code, payload := resultJSON(t, []string{"content", "workbook", "schema", "--id", "wb-1", "--env", "test", "--full"})
			encoded, _ := json.Marshal(payload)
			var result workbookops.SchemaOutput
			if err := json.Unmarshal(encoded, &result); err != nil {
				t.Fatal(err)
			}
			if code != 0 || !result.Complete || len(result.Datasources) != 1 || result.Datasources[0].Kind != "embedded" {
				t.Fatalf("hidden source selection changed: exit %d: %+v", code, payload)
			}
			var aliasFound bool
			for _, field := range result.Datasources[0].Fields {
				if field.Caption == "Local Revenue" {
					aliasFound = true
					if field.ExclusionReason != tc.exclusion || field.RequiresUserAggregation != tc.aggregation {
						t.Fatalf("remote alias lost hidden published semantics: %+v", field)
					}
				}
			}
			if !aliasFound {
				t.Fatal("visible remote alias missing")
			}
		})
	}
	if graphCalls.Load() == 0 {
		t.Fatal("Metadata API was not queried")
	}
	noNetwork := &failNetworkTransport{}
	options.HTTPClient = &http.Client{Transport: noNetwork}
	code, payload := resultJSON(t, []string{"content", "workbook", "inspect", "--id", "wb-1", "--env", "test", "--cache"})
	if noNetwork.calls != 0 {
		t.Fatalf("cached inspection made %d network requests", noNetwork.calls)
	}
	workbook, ok := payload["workbook"].(map[string]any)
	if code != 0 || !ok || workbook["luid"] != "wb-1" {
		t.Fatalf("cached inspection exit %d: %+v", code, payload)
	}
	if payload["datasource_status"] != "not_captured" || payload["datasources"] != nil {
		t.Fatalf("cached inspection overstated datasource coverage: %+v", payload)
	}
}
