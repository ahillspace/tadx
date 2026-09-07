package app_test

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestDefaultSiteLifecyclePreviewThroughCLI(t *testing.T) {
	server, _ := newGroupOneTableauServer(t)
	defer server.Close()
	configPath := writePhaseOneConfigWithSite(t, server.URL, "")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true}
	for _, args := range [][]string{
		{"content", "project", "create", "--name", "New"},
		{"content", "project", "update", "--project-id", "project-ops", "--name", "Renamed"},
		{"content", "project", "delete", "--project-id", "project-ops"},
		{"content", "project", "move", "--project-id", "project-ops", "--parent-id", "project-destination"},
		{"content", "flow", "move", "--id", "flow-1", "--destination-project-id", "project-destination"},
		{"content", "flow", "update", "--id", "flow-1", "--owner-id", "owner-2"},
		{"content", "flow", "delete", "--id", "flow-1"},
	} {
		t.Run(args[1]+"/"+args[2], func(t *testing.T) {
			runGroupOneCLI(t, options, append(args, "--environment", "production", "--preview")...)
		})
	}
}

func TestDefaultSiteAdminCreateThroughCLI(t *testing.T) {
	var creates atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			var body struct {
				Credentials struct {
					Site struct {
						ContentURL string `json:"contentUrl"`
					} `json:"site"`
				} `json:"credentials"`
			}
			if err := json.NewDecoder(r.Body).Decode(&body); err != nil || body.Credentials.Site.ContentURL != "" {
				t.Errorf("Default-site sign-in: %#v, %v", body, err)
			}
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/groups":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><groups/></tsResponse>`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/groups":
			creates.Add(1)
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><group id="group-new" name="New Group"/></tsResponse>`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	configPath := writePhaseOneConfigWithSite(t, server.URL, "")
	t.Setenv("PROD_PAT_NAME", "pat-name")
	t.Setenv("PROD_PAT_SECRET", "pat-secret")
	options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true}
	args := []string{"admin", "group", "create", "--environment", "production", "--name", "New Group"}
	runGroupOneCLI(t, options, append(args, "--preview")...)
	if creates.Load() != 0 {
		t.Fatal("preview created a group")
	}
	output := runGroupOneCLI(t, options, args...)
	if creates.Load() != 1 || !strings.Contains(output, "group-new") {
		t.Fatalf("creates=%d output=%s", creates.Load(), output)
	}
}
