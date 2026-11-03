package app_test

import (
	"bytes"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestBatchDispatchRejectsConsentRevokedDuringAuthoritativePreparation(t *testing.T) {
	var configPath string
	var reads, writes atomic.Int32
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users":
			reads.Add(1)
			_, err := config.Update(configPath, false, func(current config.Config) (config.Config, error) {
				environment, err := current.ResolveEnvironment("test")
				if err != nil {
					return current, err
				}
				return current, current.SetMutationSetting(environment, false)
			})
			if err != nil {
				t.Errorf("revoke consent during preparation: %v", err)
				http.Error(w, "configuration update failed", 500)
				return
			}
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><users><user id="user-1" name="analyst@example.test" siteRole="Viewer"/></users></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-1":
			fmt.Fprint(w, `<tsResponse><user id="user-1" name="analyst@example.test" siteRole="Viewer"/></tsResponse>`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site-1/users/user-1":
			writes.Add(1)
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	opts := diagnosticOptions(t, server)
	configPath = opts.ConfigPath
	batch := filepath.Join(t.TempDir(), "items.json")
	if err := os.WriteFile(batch, []byte(`{"items":[{"username":"analyst@example.test"}]}`), 0o600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := app.Run(t.Context(), []string{"admin", "user", "delete", "--environment", "test", "--batch-file", batch, "--json", "--full"}, &output, opts)
	current, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := current.ResolveEnvironment("test")
	if err != nil {
		t.Fatal(err)
	}
	consent, err := current.MutationSetting(environment)
	if err != nil || consent == nil || *consent {
		t.Fatalf("consent was not revoked: %v, %v", consent, err)
	}
	if code == 0 || reads.Load() != 1 || writes.Load() != 0 {
		t.Fatalf("dispatch used revoked consent: code=%d preparation_reads=%d mutation_requests=%d output=%s", code, reads.Load(), writes.Load(), output.String())
	}
}
