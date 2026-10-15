package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/managedpolicy"
)

func TestWorkbookInspectCachesConfirmedRESTWhenMetadataFails(t *testing.T) {
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			fmt.Fprint(w, `{"credentials":{"token":"fixture-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/workbooks/wb-1":
			fmt.Fprint(w, `<tsResponse><workbook id="wb-1" name="Finance"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook></tsResponse>`)
		case "/api/3.29/sites/site-1/projects":
			fmt.Fprint(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
		case "/api/metadata/graphql":
			http.Error(w, "metadata unavailable", http.StatusServiceUnavailable)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
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
	options := Options{ConfigPath: configPath, HTTPClient: server.Client(), UserHomeDir: func() (string, error) { return userHome, nil }, managedPolicy: fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"workbook.inspect": true}}}
	run := func(args ...string) (int, map[string]any) {
		t.Helper()
		var output bytes.Buffer
		code := Run(t.Context(), append([]string{"content", "workbook", "inspect", "--id", "wb-1", "--env", "test", "--json"}, args...), &output, options)
		var payload map[string]any
		if err := json.Unmarshal(output.Bytes(), &payload); err != nil {
			t.Fatalf("invalid JSON (exit %d): %s", code, output.String())
		}
		return code, payload
	}
	code, payload := run()
	partial, _ := payload["output"].(map[string]any)
	confirmed, _ := partial["workbook"].(map[string]any)
	if code == 0 || partial["status"] != "partial" || confirmed["luid"] != "wb-1" {
		t.Fatalf("metadata failure erased confirmed REST output: exit %d: %+v", code, payload)
	}
	noNetwork := &failNetworkTransport{}
	options.HTTPClient = &http.Client{Transport: noNetwork}
	code, payload = run("--cache")
	confirmed, _ = payload["workbook"].(map[string]any)
	if code != 0 || confirmed["luid"] != "wb-1" || payload["datasource_status"] != "not_captured" || payload["datasources"] != nil {
		t.Fatalf("confirmed REST workbook missing from offline cache: exit %d: %+v", code, payload)
	}
	if noNetwork.calls != 0 {
		t.Fatalf("cached inspection made %d network requests", noNetwork.calls)
	}
}
