package app_test

import (
	"context"
	"encoding/base64"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestSchemaInvalidLocalArgumentsFailBeforeAuthentication(t *testing.T) {
	for _, credentials := range []bool{false, true} {
		for _, flags := range [][]string{{"--role", "invalid"}, {"--limit", "-1"}, {"--all", "--limit", "1"}, {"--field-id", " "}, {"--cursor", "invalid"}, {"--cursor", base64.RawURLEncoding.EncodeToString([]byte(`{"offset":0,"fingerprint":"wrong-query"}`))}} {
			t.Run(fmt.Sprintf("credentials=%v/%v", credentials, flags), func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					http.Error(w, "must not authenticate", http.StatusUnauthorized)
				}))
				defer server.Close()
				options := cacheResilienceOptions(t, server)
				if !credentials {
					t.Setenv("PROD_PAT_NAME", "")
					t.Setenv("PROD_PAT_SECRET", "")
				}
				args := append([]string{"content", "datasource", "schema", "--environment", "production", "--id", "datasource"}, flags...)
				var output strings.Builder
				exit := app.Run(context.Background(), args, &output, options)
				if exit != 2 || !strings.Contains(output.String(), "kind: usage") || requests.Load() != 0 {
					t.Fatalf("exit=%d requests=%d output=%s", exit, requests.Load(), output.String())
				}
			})
		}
	}
}

func TestLocalReadAndMutationErrorsMakeNoAuthenticationRequest(t *testing.T) {
	cases := [][]string{
		{"content", "workbook", "list", "--limit", "-1"},
		{"content", "datasource", "list", "--limit", "10001"},
		{"content", "flow", "list", "--limit", "-1"},
		{"content", "flow", "list", "--all", "--limit", "1"},
		{"content", "project", "list", "--cursor", "invalid"},
		{"content", "project", "create", "--environment", "production", "--name", "bad/name", "--preview"},
		{"content", "datasource", "update", "--environment", "production", "--id", "datasource", "--name", "", "--preview"},
		{"catalog", "lineage", "pull", "--environment", "production", "--kind", "workbook", "--id", "book", "--depth", "4"},
		{"search", "--type", "workbook", "--limit", "2001"},
		{"admin", "user", "list", "--limit", "-1"},
		{"admin", "group", "list", "--limit", "10001"},
		{"admin", "user", "create", "--environment", "production", "--name", "person", "--site-role", "invalid", "--preview"},
		{"pulse", "metric", "fork", "--environment", "production", "--id", "metric", "--timeframe", "INVALID", "--preview"},
	}
	for _, args := range cases {
		t.Run(strings.Join(args, " "), func(t *testing.T) {
			var requests atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				http.Error(w, "must not authenticate", http.StatusUnauthorized)
			}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			options = withSiteMutationConsent(t, options, true)
			var output strings.Builder
			exit := app.Run(context.Background(), args, &output, options)
			if exit != 2 || requests.Load() != 0 {
				t.Fatalf("exit=%d requests=%d output=%s", exit, requests.Load(), output.String())
			}
		})
	}
}

func TestLocalPrerequisiteDoesNotMigrateLegacyWorkspaceManifest(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "must not authenticate", http.StatusUnauthorized)
	}))
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	root := createNamedWorkspace(t, options.ConfigPath, "legacy")
	manifest := filepath.Join(root, "tadx.yaml")
	legacy := []byte("version: 1\n")
	if err := os.WriteFile(manifest, legacy, 0600); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	exit := app.Run(context.Background(), []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "legacy", "--id", "workbook"}, &output, options)
	after, err := os.ReadFile(manifest)
	if err != nil || string(after) != string(legacy) || exit == 0 || requests.Load() != 0 {
		t.Fatalf("exit=%d requests=%d manifest=%q error=%v output=%s", exit, requests.Load(), after, err, output.String())
	}
}

func TestMissingLocalPrerequisitesFailBeforeAuthenticationWithoutWrites(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, operation := range []string{"pull", "publish"} {
			t.Run(kind+"/"+operation, func(t *testing.T) {
				var requests atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					requests.Add(1)
					http.Error(w, "must not authenticate", http.StatusUnauthorized)
				}))
				defer server.Close()
				options := cacheResilienceOptions(t, server)
				options = withSiteMutationConsent(t, options, true)
				root := createNamedWorkspace(t, options.ConfigPath, "local")
				before, err := os.ReadFile(filepath.Join(root, "tadx.yaml"))
				if err != nil {
					t.Fatal(err)
				}
				args := []string{"content", kind, operation, "--environment", "production"}
				if operation == "pull" {
					args = append(args, "--workspace", "missing", "--id", "item")
				} else {
					args = append(args, "--workspace", "local", "--artifact", "artifacts/"+kind+"/missing", "--project-id", "project", "--preview")
					if kind == "datasource" {
						args = append(args, "--create")
					}
				}
				var output strings.Builder
				exit := app.Run(context.Background(), args, &output, options)
				if exit == 0 || requests.Load() != 0 || strings.Contains(output.String(), "PAT") {
					t.Fatalf("exit=%d requests=%d output=%s", exit, requests.Load(), output.String())
				}
				after, err := os.ReadFile(filepath.Join(root, "tadx.yaml"))
				if err != nil || string(after) != string(before) {
					t.Fatalf("prerequisite check changed workspace manifest: %v", err)
				}
				entries, err := os.ReadDir(filepath.Join(root, "artifacts"))
				if err != nil || len(entries) != 0 {
					t.Fatalf("prerequisite check wrote artifacts: %v error=%v", entries, err)
				}
			})
		}
	}
}
