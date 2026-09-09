package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/config"
)

func TestWriteTargetSelectionBeforeNetwork(t *testing.T) {
	for _, multiple := range []bool{false, true} {
		t.Run(map[bool]string{false: "sole", true: "multiple"}[multiple], func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			cfg := config.Config{Version: 1, Environments: map[string]config.Environment{"one": {URL: "https://example.invalid", Auth: config.Auth{Type: "pat"}}}}
			if multiple {
				cfg.Environments["two"] = config.Environment{URL: "https://another.invalid", Auth: config.Auth{Type: "pat"}}
			}
			if err := config.Save(path, cfg); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := app.Run(context.Background(), []string{"content", "project", "create", "--preview"}, &out, app.Options{ConfigPath: path})
			if code == 0 {
				t.Fatal("incomplete command succeeded")
			}
			if multiple {
				if !strings.Contains(out.String(), "Multiple environments") || !strings.Contains(out.String(), "one") || !strings.Contains(out.String(), "two") {
					t.Fatal(out.String())
				}
			} else if strings.Contains(out.String(), "environment is required") || strings.Contains(out.String(), "explicit environment") {
				t.Fatal(out.String())
			}
		})
	}
}

func TestSoleEnvironmentProjectCreateWithoutEnvironmentExecutesExactTarget(t *testing.T) {
	var signins, reads, creates atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin":
			signins.Add(1)
			var body struct {
				Credentials struct {
					Site struct {
						ContentURL string `json:"contentUrl"`
					} `json:"site"`
				} `json:"credentials"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Credentials.Site.ContentURL != "only-site" {
				t.Errorf("sign-in did not select the sole site: %#v %v", body, err)
			}
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"only-site-id"},"user":{"id":"fixture-user"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/only-site-id/projects":
			reads.Add(1)
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><projects/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/only-site-id/projects":
			creates.Add(1)
			body, err := io.ReadAll(r.Body)
			if err != nil || !strings.Contains(string(body), "Sole target project") {
				t.Errorf("unexpected project create body: %s %v", body, err)
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><project id="created-project" name="Sole target project"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected fixture request", 500)
		}
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	cfg := config.Config{Version: 1, Environments: map[string]config.Environment{"one": {URL: server.URL, SiteContentURL: "only-site", APIVersion: "3.29", Auth: config.Auth{Type: "pat", PATNameEnv: "SOLE_TARGET_PAT_NAME", PATSecretEnv: "SOLE_TARGET_PAT_SECRET"}}}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("SOLE_TARGET_PAT_NAME", "fixture-pat")
	t.Setenv("SOLE_TARGET_PAT_SECRET", "fixture-secret")
	var out bytes.Buffer
	exit := app.Run(context.Background(), []string{"content", "project", "create", "--name", "Sole target project"}, &out, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true})
	if exit != 0 || signins.Load() != 1 || reads.Load() != 2 || creates.Load() != 1 || !strings.Contains(out.String(), "created-project") || !strings.Contains(out.String(), "environment: one") || !strings.Contains(out.String(), "site: only-site") {
		t.Fatalf("exit=%d signins=%d reads=%d creates=%d output=%s", exit, signins.Load(), reads.Load(), creates.Load(), out.String())
	}
}

func TestMultipleEnvironmentsRejectOmittedTargetDespiteDefaultAndArtifactSource(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		http.Error(w, "must not authenticate or access a target", 500)
	}))
	defer server.Close()
	configPath := filepath.Join(t.TempDir(), "config.yaml")
	auth := config.Auth{Type: "pat", PATNameEnv: "MULTI_TARGET_PAT_NAME", PATSecretEnv: "MULTI_TARGET_PAT_SECRET"}
	cfg := config.Config{Version: 1, DefaultEnvironment: "one", Environments: map[string]config.Environment{"one": {URL: server.URL, SiteContentURL: "source-site", APIVersion: "3.29", Auth: auth}, "two": {URL: server.URL, SiteContentURL: "other-site", APIVersion: "3.29", Auth: auth}}}
	if err := config.Save(configPath, cfg); err != nil {
		t.Fatal(err)
	}
	t.Setenv("MULTI_TARGET_PAT_NAME", "fixture-pat")
	t.Setenv("MULTI_TARGET_PAT_SECRET", "fixture-secret")
	workspace := createNamedWorkspace(t, configPath, "source-artifacts")
	stored, err := artifact.NewWorkbookManager(nil).Pull(context.Background(), artifact.WorkbookPull{Workspace: workspace, Filename: "Source.twb", Content: []byte("<workbook/>"), Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: "Source", TableauID: "source-workbook", SourceServerOrigin: server.URL, SourceSiteLUID: "source-site-id", SourceEnvironment: "one", SourceSite: "source-site", SourceProjectID: "project-1", SourceProjectName: "Ops"}})
	if err != nil {
		t.Fatal(err)
	}
	selector, err := filepath.Rel(workspace, stored.ArtifactPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"content", "project", "create", "--name", "New project"}, {"content", "workbook", "publish", "--workspace", "source-artifacts", "--artifact", filepath.ToSlash(selector), "--overwrite"}} {
		var out bytes.Buffer
		exit := app.Run(context.Background(), args, &out, app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true})
		if exit != 2 || requests.Load() != 0 || !strings.Contains(out.String(), "Multiple environments") || !strings.Contains(out.String(), "one") || !strings.Contains(out.String(), "two") {
			t.Fatalf("args=%v exit=%d requests=%d output=%s", args, exit, requests.Load(), out.String())
		}
	}
}
