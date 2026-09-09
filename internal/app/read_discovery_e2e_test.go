package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestReadDiscoveryWithoutPublicCursorsThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/datasources/ds-1":
			_, _ = io.WriteString(w, `<tsResponse><datasource id="ds-1" name="Orders"><project id="p1" name="Test"/></datasource></tsResponse>`)
		case "/api/v1/vizql-data-service/read-metadata":
			_, _ = io.WriteString(w, `{"data":[{"fieldName":"Alpha","dataType":"STRING","fieldRole":"DIMENSION"},{"fieldName":"Beta","dataType":"STRING","fieldRole":"DIMENSION"}]}`)
		case "/api/-/search":
			limit, _ := strconv.Atoi(r.URL.Query().Get("limit"))
			page, _ := strconv.Atoi(r.URL.Query().Get("page"))
			if limit > 100 {
				t.Errorf("unbounded provider page: %d", limit)
			}
			items := []any{}
			for index := page * limit; index < min((page+1)*limit, 205); index++ {
				items = append(items, map[string]any{"content": map[string]any{"luid": fmt.Sprintf("wb-%03d", index), "type": "workbook", "name": fmt.Sprintf("Report %03d", index)}})
			}
			payload := map[string]any{"items": items, "limit": limit, "pageIndex": page, "startIndex": page * limit, "total": 205}
			if (page+1)*limit < 205 {
				payload["next"] = fmt.Sprintf("/api/-/search?page=%d", page+1)
			}
			_ = json.NewEncoder(w).Encode(payload)
		case "/api/-/pulse/definitions":
			if r.URL.Query().Get("page_token") == "" {
				_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"definition-1","name":"Alpha"},"specification":{"datasource":{"id":"ds-1"}}}],"next_page_token":"private-token"}`)
			} else {
				_, _ = io.WriteString(w, `{"definitions":[{"metadata":{"id":"definition-2","name":"Beta"},"specification":{"datasource":{"id":"ds-1"}}}]}`)
			}
		case "/api/-/pulse/definitions/definition-1/metrics":
			if r.URL.Query().Get("page_token") == "" {
				_, _ = io.WriteString(w, `{"metrics":[{"metadata":{"id":"metric-1","name":"Alpha"},"definition_id":"definition-1","specification":{}}],"next_page_token":"private-token"}`)
			} else {
				_, _ = io.WriteString(w, `{"metrics":[{"metadata":{"id":"metric-2","name":"Beta"},"definition_id":"definition-1","specification":{}}]}`)
			}
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	configuration := fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: test\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: DISCOVERY_PAT_NAME\n      pat_secret_env: DISCOVERY_PAT_SECRET\n", server.URL)
	if err := os.WriteFile(configPath, []byte(configuration), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("DISCOVERY_PAT_NAME", "test-pat")
	t.Setenv("DISCOVERY_PAT_SECRET", "test-secret")
	for _, command := range [][]string{{"capability", "list"}, {"env", "list"}, {"workspace", "list"}} {
		var output bytes.Buffer
		args := append(command, "--limit", "10000")
		code := app.Run(context.Background(), args, &output, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
		if code != 0 || strings.Contains(output.String(), "next_cursor") || !strings.Contains(output.String(), "more_available: false") {
			t.Errorf("local args=%v code=%d output=%s", args, code, output.String())
		}
	}
	for _, limit := range []int{20, 150, 205, 2000} {
		for _, full := range []bool{false, true} {
			args := []string{"search", "report", "--type", "workbook", "--environment", "test", "--limit", strconv.Itoa(limit)}
			if full {
				args = append(args, "--full")
			}
			var output bytes.Buffer
			code := app.Run(context.Background(), args, &output, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
			if code != 0 || strings.Contains(output.String(), "next_cursor") || !strings.Contains(output.String(), fmt.Sprintf("returned: %d", min(limit, 205))) || !strings.Contains(output.String(), fmt.Sprintf("more_available: %t", limit < 205)) {
				t.Errorf("search args=%v code=%d output=%s", args, code, output.String())
			}
		}
	}
	for _, command := range [][]string{
		{"content", "datasource", "schema", "--id", "ds-1"},
		{"pulse", "definition", "list"},
		{"pulse", "metric", "list", "--definition-id", "definition-1"},
	} {
		for _, full := range []bool{false, true} {
			for _, all := range []bool{false, true} {
				args := append(append([]string(nil), command...), "--environment", "test")
				if all {
					args = append(args, "--all")
				} else {
					args = append(args, "--limit", "1")
				}
				if full {
					args = append(args, "--full")
				}
				var stdout bytes.Buffer
				code := app.Run(context.Background(), args, &stdout, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
				output := stdout.String()
				if code != 0 || strings.Contains(output, "next_cursor") || strings.Contains(output, "private-token") || !strings.Contains(output, fmt.Sprintf("more_available: %t", !all)) || strings.Contains(output, "Beta") != all {
					t.Errorf("args=%v code=%d output=%s", args, code, output)
				}
			}
		}
	}
	var stdout bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "definition", "list", "--environment", "test", "--name", "Beta", "--limit", "1"}, &stdout, app.Options{ConfigPath: configPath, HTTPClient: server.Client()})
	if code != 0 || !strings.Contains(stdout.String(), "definition-2") || !strings.Contains(stdout.String(), "more_available: false") {
		t.Fatalf("name lookup code=%d output=%s", code, stdout.String())
	}
}
