package app_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestRESTVersionNegotiationThroughCLI(t *testing.T) {
	for _, scenario := range []struct {
		name, reported, selected string
		unsupported              bool
	}{
		{name: "older server", reported: "3.9", selected: "3.9"},
		{name: "newer server", reported: "4.0", selected: "3.29"},
		{name: "below PAT minimum", reported: "3.5", unsupported: true},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			var discoveries, signins atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case r.URL.Path == "/api/2.4/serverinfo":
					discoveries.Add(1)
					if r.Method != http.MethodGet || r.Header.Get("X-Tableau-Auth") != "" || r.Header.Get("Authorization") != "" {
						t.Error("server discovery must be an unauthenticated GET")
					}
					_, _ = fmt.Fprintf(w, `<tsResponse><serverInfo><productVersion build="fixture">fixture</productVersion><restApiVersion>%s</restApiVersion></serverInfo></tsResponse>`, scenario.reported)
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					signins.Add(1)
					if scenario.unsupported || r.URL.Path != "/api/"+scenario.selected+"/auth/signin" || discoveries.Load() != 1 {
						t.Error("sign-in did not use successful server version discovery")
						w.WriteHeader(http.StatusBadRequest)
						return
					}
					_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-id"},"user":{"id":"user-id"}}}`)
				case strings.HasSuffix(r.URL.Path, "/auth/signout"):
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Error("unexpected request during authentication check")
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			t.Setenv("FIXTURE_PAT_NAME", "fixture-name")
			t.Setenv("FIXTURE_PAT_SECRET", "fixture-credential")
			path := filepath.Join(t.TempDir(), "config.yaml")
			configuration := fmt.Sprintf("version: 1\ndefault_environment: good\nenvironments:\n  good:\n    url: %s\n    api_version: 3.25\n    auth:\n      type: pat\n      pat_name_env: FIXTURE_PAT_NAME\n      pat_secret_env: FIXTURE_PAT_SECRET\n", server.URL)
			if err := os.WriteFile(path, []byte(configuration), 0o600); err != nil {
				t.Fatal(err)
			}
			var result strings.Builder
			code := app.Run(t.Context(), []string{"auth", "check", "--env", "good", "--json"}, &result, app.Options{ConfigPath: path, HTTPClient: server.Client(), Stderr: io.Discard})
			if discoveries.Load() != 1 {
				t.Errorf("discoveries=%d; want one per command", discoveries.Load())
			}
			if scenario.unsupported {
				if code != 1 || signins.Load() != 0 || !strings.Contains(result.String(), "3.6") || !strings.Contains(result.String(), "3.5") {
					t.Error("unsupported server must fail with both versions before sign-in")
				}
				return
			}
			if code != 0 || signins.Load() != 1 || !strings.Contains(result.String(), `"rest_api_version":"`+scenario.selected+`"`) {
				t.Error("authentication check must report the negotiated REST version")
			}
		})
	}
}
