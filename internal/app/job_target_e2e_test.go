package app_test

import (
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/value"
)

func TestJobRecoveryUsesCanonicalServerAndExactSite(t *testing.T) {
	for _, tc := range []struct {
		name, selected, saved, site, siteID string
		same                                bool
	}{
		{"default port and host case", "https://TABLEAU.example.test:443/Tableau/", "https://tableau.example.test/Tableau", "", "site-1", true},
		{"base path case", "https://tableau.example.test/Tableau", "https://tableau.example.test/tableau", "", "site-1", false},
		{"different port", "https://tableau.example.test", "https://tableau.example.test:8443", "", "site-1", false},
		{"different site URL", "https://tableau.example.test", "https://tableau.example.test", "other-site", "site-1", false},
		{"different site LUID", "https://tableau.example.test", "https://tableau.example.test", "", "other-id", false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			reads := 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if strings.HasSuffix(r.URL.Path, "/serverinfo") {
					io.WriteString(w, `<tsResponse><serverInfo><productVersion>2026.2</productVersion><restApiVersion>3.29</restApiVersion></serverInfo></tsResponse>`)
					return
				}
				if strings.HasSuffix(r.URL.Path, "/auth/signin") {
					io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
					return
				}
				if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/sites/site-1/jobs/job-target") {
					reads++
					io.WriteString(w, `<tsResponse><job id="job-target" type="RefreshExtract" progress="100" finishCode="0"/></tsResponse>`)
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusNotFound)
			}))
			defer server.Close()
			options := diagnosticOptions(t, server)
			if _, err := config.Update(options.ConfigPath, false, func(cfg config.Config) (config.Config, error) {
				environment := cfg.Environments["test"]
				environment.URL = tc.selected
				cfg.Environments["test"] = environment
				return cfg, nil
			}); err != nil {
				t.Fatal(err)
			}
			endpoint, err := url.Parse(server.URL)
			if err != nil {
				t.Fatal(err)
			}
			client := *server.Client()
			client.Transport = targetFixtureTransport{endpoint: endpoint, base: client.Transport}
			options.HTTPClient = &client
			options.JobDirectory = t.TempDir()
			store := jobmonitor.Store{Directory: options.JobDirectory}
			path, err := store.Save(t.Context(), jobmonitor.Receipt{
				Version: 1, Operation: "job.wait", Environment: "test", Server: tc.saved, Site: tc.site, SiteID: tc.siteID,
				ConfigPath: options.ConfigPath, CoordinationKey: "fixture-coordination", TrackingStartedAt: time.Now().UTC(),
				Observation: value.JobStatus{ID: "job-target", Type: "RefreshExtract", Status: "pending"},
			})
			if err != nil {
				t.Fatal(err)
			}
			var output strings.Builder
			code := app.Run(t.Context(), []string{"job", "wait", "--receipt", path}, &output, options)
			if tc.same {
				if code != 0 || reads != 1 || !strings.Contains(output.String(), "status: succeeded") {
					t.Fatalf("equivalent target rejected: code=%d reads=%d output=%s", code, reads, output.String())
				}
			} else if code == 0 || reads != 0 || !strings.Contains(output.String(), "job.wait.target") {
				t.Fatalf("distinct target accepted: code=%d reads=%d output=%s", code, reads, output.String())
			}
		})
	}
}
