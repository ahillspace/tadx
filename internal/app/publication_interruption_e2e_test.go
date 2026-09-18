package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/jobmonitor"
)

func TestInterruptedPublicationPreservesAcceptedJobThroughCLI(t *testing.T) {
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	jobDirectory := t.TempDir()
	var writes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			w.Header().Set("Content-Type", "application/json")
			io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/users/user-1"):
			io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			writes.Add(1)
			w.WriteHeader(http.StatusAccepted)
			io.WriteString(w, `<tsResponse><job id="accepted-job" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/jobs/accepted-job"):
			files, _ := filepath.Glob(filepath.Join(jobDirectory, "*.json"))
			found := false
			for _, path := range files {
				data, _ := os.ReadFile(path)
				var receipt jobmonitor.Receipt
				if json.Unmarshal(data, &receipt) == nil && receipt.Observation.ID == "accepted-job" {
					found = true
				}
			}
			if !found {
				t.Error("job status requested before durable acceptance")
			}
			cancel()
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><workbooks/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Accepted.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0600); err != nil {
		t.Fatal(err)
	}
	var out, progress strings.Builder
	exit := Run(ctx, []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &out, withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), JobDirectory: jobDirectory, Stderr: &progress}, true))
	if exit == 0 || writes.Load() != 1 || !strings.Contains(out.String(), `"tableau_job_id":"accepted-job"`) || !strings.Contains(out.String(), `"phase":"verification"`) || strings.Contains(out.String(), "Run without --preview") {
		t.Fatalf("exit=%d writes=%d output=%s", exit, writes.Load(), out.String())
	}
	saved, path, err := (jobmonitor.Store{Directory: jobDirectory}).FindByJobID(t.Context(), "accepted-job", "production", "")
	if err != nil || saved.Observation.Status != "pending" || saved.SourcePath != filepath.ToSlash(file) || saved.ConfigPath != runtime.configPath || !strings.Contains(progress.String(), filepath.ToSlash(path)) {
		t.Fatalf("receipt=%+v path=%s err=%v progress=%s", saved, path, err, progress.String())
	}
}
