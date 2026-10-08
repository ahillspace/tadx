package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/managedpolicy"
)

func TestWorkbookMetadataCommandsThroughHTTP(t *testing.T) {
	var networkBlocked atomic.Bool
	var graphCalls atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if networkBlocked.Load() {
			t.Errorf("cached workbook inspection contacted Tableau: %s %s", r.Method, r.URL)
			http.Error(w, "unexpected network", 500)
			return
		}
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
		case "/api/metadata/graphql":
			graphCalls.Add(1)
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
			w.Header().Set("Content-Type", "application/json")
			switch {
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
					items = `{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[{"__typename":"DatasourceField","id":"local-meta","name":"Local Revenue","fullyQualifiedName":"[Revenue]","description":"Local meaning","remoteField":{"__typename":"ColumnField","id":"remote-meta","name":"Revenue"},"descriptionInherited":[],"relatedSheets":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"sheet-meta","luid":"sheet-1","name":"Overview"}]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}},{"__typename":"CalculatedField","id":"hidden-meta","name":"Private Calc","fullyQualifiedName":"[Private Calc]","description":"","isHidden":true,"formula":"[Revenue] * 2","role":"MEASURE","dataType":"REAL","descriptionInherited":[],"referencedFields":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"local-meta","name":"Local Revenue"}]},"relatedSheets":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]},"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}`
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
		{"schema", []string{"content", "workbook", "schema", "--id", "wb-1", "--descriptions", "--full"}, []string{"Local Revenue", "Local meaning", "Published meaning", "Private Calc", "remote-meta", "sheet-meta", "referenced_fields", "measure", "REAL"}},
		{"audit", []string{"catalog", "audit", "--type", "workbook", "--id", "wb-1", "--full"}, []string{"local-meta", "published-meta", "missing", "present"}},
		{"inspect", []string{"content", "workbook", "inspect", "--id", "wb-1", "--full"}, []string{"embedded-meta", "published-meta", "field_count", "ds-1"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			code, payload := resultJSON(t, append(tc.args, "--env", "test"))
			if code != 0 {
				t.Fatalf("exit %d: %+v", code, payload)
			}
			encoded, _ := json.Marshal(payload)
			for _, want := range tc.required {
				if !strings.Contains(string(encoded), want) {
					t.Errorf("missing %q: %s", want, encoded)
				}
			}
		})
	}
	if graphCalls.Load() == 0 {
		t.Fatal("Metadata API was not queried")
	}
	networkBlocked.Store(true)
	code, payload := resultJSON(t, []string{"content", "workbook", "inspect", "--id", "wb-1", "--env", "test", "--cache"})
	workbook, ok := payload["workbook"].(map[string]any)
	if code != 0 || !ok || workbook["luid"] != "wb-1" {
		t.Fatalf("cached inspection exit %d: %+v", code, payload)
	}
}
