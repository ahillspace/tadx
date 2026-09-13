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

func TestWorkspaceCreateRejectsBatchFileBeforeCreatingRoots(t *testing.T) {
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
	if code == 0 || !strings.Contains(output.String(), "unknown flag: --batch-file") {
		t.Fatalf("expected batch flag rejection: code=%d output=%s", code, &output)
	}
	for _, name := range []string{"alpha", "beta"} {
		if _, err := os.Stat(filepath.Join(directory, name)); !os.IsNotExist(err) {
			t.Fatalf("rejected batch created workspace %s: %v", name, err)
		}
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

func TestSingleOperationCommandsRejectBatchFileBeforeAccessingInputs(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	for _, command := range []string{
		"agent install", "agent uninstall", "auth check", "auth status", "auth logout",
		"env add", "env get", "env update", "env remove", "cache refresh", "cache status",
		"capability get", "doctor", "search", "catalog search",
		"workspace create", "workspace register", "workspace clone", "workspace status",
	} {
		t.Run(command, func(t *testing.T) {
			directory := t.TempDir()
			configuration := filepath.Join(directory, "config.yaml")
			if err := config.Save(configuration, config.Config{Version: config.CurrentVersion, DefaultEnvironment: "alpha", Environments: map[string]config.Environment{
				"alpha": {URL: server.URL, Auth: config.Auth{Type: config.AuthTypePAT}},
			}}); err != nil {
				t.Fatal(err)
			}
			before, err := os.ReadFile(configuration)
			if err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			args := append(strings.Fields(command), "--batch-file", filepath.Join(directory, "must-not-read.json"), "--json")
			code := app.Run(context.Background(), args, &output, app.Options{ConfigPath: configuration, HTTPClient: server.Client()})
			if code == 0 || !strings.Contains(output.String(), "unknown flag: --batch-file") || requests != 0 {
				t.Fatalf("code=%d requests=%d output=%s", code, requests, &output)
			}
			after, err := os.ReadFile(configuration)
			if err != nil || !bytes.Equal(before, after) {
				t.Fatalf("rejected batch changed configuration: %v", err)
			}
			entries, err := os.ReadDir(directory)
			if err != nil {
				t.Fatal(err)
			}
			for _, entry := range entries {
				// Usage failures still persist the normal last-result record.
				if entry.IsDir() || (entry.Name() != "config.yaml" && entry.Name() != "last-result.json" && entry.Name() != "last-result.json.lock") {
					t.Errorf("rejected batch wrote unexpected entry %s", entry.Name())
				}
			}
		})
	}
}

func TestEnvironmentGetRejectsMultiplePositionalTargets(t *testing.T) {
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
	if code == 0 || !strings.Contains(output.String(), "accepts 1 arg(s)") || bytes.Contains(output.Bytes(), []byte(`"items"`)) {
		t.Fatalf("code=%d output=%s", code, &output)
	}
}
