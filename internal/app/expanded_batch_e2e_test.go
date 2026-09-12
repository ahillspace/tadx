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
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestWorkspaceBatchFileCreatesDistinctRootsAndReportsOrderedStatus(t *testing.T) {
	directory := t.TempDir()
	configuration := filepath.Join(directory, "config.yaml")
	if err := config.Save(configuration, config.Config{Version: config.CurrentVersion}); err != nil {
		t.Fatal(err)
	}
	items := map[string]any{"items": []map[string]any{
		{"args": []string{"alpha"}, "path": filepath.Join(directory, "alpha")},
		{"args": []string{"beta"}, "path": filepath.Join(directory, "beta")},
	}}
	data, err := json.Marshal(items)
	if err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(directory, "workspaces.json")
	if err := os.WriteFile(file, data, 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	options := app.Options{ConfigPath: configuration}
	code := app.Run(context.Background(), []string{"workspace", "create", "--batch-file", file, "--json"}, &output, options)
	if code != 0 {
		t.Fatalf("create code=%d output=%s", code, &output)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(directory, name, "tadx.yaml")); err != nil {
			t.Fatalf("workspace %s missing: %v", name, err)
		}
	}
	output.Reset()
	code = app.Run(context.Background(), []string{"workspace", "status", "--workspace", "alpha", "--workspace", "missing", "--workspace", "beta", "--json"}, &output, options)
	var batch struct {
		Status    string `json:"status"`
		Succeeded int    `json:"succeeded"`
		Failed    int    `json:"failed"`
		Items     []struct {
			Status string          `json:"status"`
			Result json.RawMessage `json:"result"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output.Bytes(), &batch); err != nil {
		t.Fatalf("decode: %v output=%s", err, &output)
	}
	if code == 0 || batch.Status != "partial_failure" || batch.Succeeded != 2 || batch.Failed != 1 || len(batch.Items) != 3 || batch.Items[0].Status != "succeeded" || batch.Items[1].Status != "failed" || batch.Items[2].Status != "succeeded" {
		t.Fatalf("status code=%d output=%s", code, &output)
	}
}

func TestPulseMetricListBatchSeparatesDefinitionScopesAndRetainsPartialResults(t *testing.T) {
	var signins int
	var definitions []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/api/3.29/auth/signin" {
			signins++
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		if r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/-/pulse/definitions/") && strings.HasSuffix(r.URL.Path, "/metrics") {
			definition := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/-/pulse/definitions/"), "/metrics")
			definitions = append(definitions, definition)
			if definition == "denied" {
				w.WriteHeader(http.StatusForbidden)
				return
			}
			_, _ = fmt.Fprintf(w, `{"metrics":[{"id":"metric-%s","definition_id":"%s"}]}`, definition, definition)
			return
		}
		t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	options.MutationsEnabled = false
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "metric", "list", "--definition-id", "alpha", "--definition-id", "denied", "--definition-id", "beta", "--json"}, &output, options)
	var batch struct {
		Succeeded int               `json:"succeeded"`
		Failed    int               `json:"failed"`
		Items     []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(output.Bytes(), &batch); err != nil {
		t.Fatalf("decode=%v output=%s", err, &output)
	}
	if code == 0 || signins != 1 || !reflect.DeepEqual(definitions, []string{"alpha", "denied", "beta"}) || batch.Succeeded != 2 || batch.Failed != 1 || len(batch.Items) != 3 || !bytes.Contains(batch.Items[0], []byte("metric-alpha")) || !bytes.Contains(batch.Items[2], []byte("metric-beta")) {
		t.Fatalf("code=%d signins=%d scopes=%v output=%s", code, signins, definitions, &output)
	}
}

func TestAuthStatusBatchFileUsesEachEnvironmentWithoutRemoteRequests(t *testing.T) {
	configuration := filepath.Join(t.TempDir(), "config.yaml")
	if err := config.Save(configuration, config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"alpha": {URL: "https://alpha.example.test", SiteContentURL: "alpha-site", Auth: config.Auth{Type: config.AuthTypePAT}},
		"beta":  {URL: "https://beta.example.test", SiteContentURL: "beta-site", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}); err != nil {
		t.Fatal(err)
	}
	file := filepath.Join(t.TempDir(), "environments.json")
	if err := os.WriteFile(file, []byte(`{"items":[{"environment":"alpha"},{"environment":"beta"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"auth", "status", "--batch-file", file, "--json"}, &output, app.Options{ConfigPath: configuration})
	var batch struct {
		Succeeded int `json:"succeeded"`
		Items     []struct {
			Result struct {
				Environment string `json:"environment"`
				Site        string `json:"site_content_url"`
			} `json:"result"`
		} `json:"items"`
	}
	if err := json.Unmarshal(output.Bytes(), &batch); err != nil {
		t.Fatalf("decode=%v output=%s", err, &output)
	}
	if code != 0 || batch.Succeeded != 2 || len(batch.Items) != 2 || batch.Items[0].Result.Environment != "alpha" || batch.Items[1].Result.Environment != "beta" || batch.Items[0].Result.Site != "alpha-site" || batch.Items[1].Result.Site != "beta-site" {
		t.Fatalf("code=%d output=%s", code, &output)
	}
}

func TestEnvironmentGetBatchPositionalQueriesRetainMissingItem(t *testing.T) {
	configuration := filepath.Join(t.TempDir(), "config.yaml")
	environments := map[string]config.Environment{
		"alpha": {URL: "https://alpha.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
		"beta":  {URL: "https://beta.example.test", Auth: config.Auth{Type: config.AuthTypePAT}},
	}
	if err := config.Save(configuration, config.Config{Version: config.CurrentVersion, Environments: environments}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"env", "get", "alpha", "missing", "beta", "--json"}, &output, app.Options{ConfigPath: configuration})
	var batch struct {
		Succeeded int               `json:"succeeded"`
		Failed    int               `json:"failed"`
		Items     []json.RawMessage `json:"items"`
	}
	if err := json.Unmarshal(output.Bytes(), &batch); err != nil {
		t.Fatalf("decode: %v output=%s", err, &output)
	}
	if code == 0 || batch.Succeeded != 2 || batch.Failed != 1 || len(batch.Items) != 3 || !bytes.Contains(batch.Items[0], []byte("alpha.example.test")) || !bytes.Contains(batch.Items[2], []byte("beta.example.test")) {
		t.Fatalf("code=%d output=%s", code, &output)
	}
}
