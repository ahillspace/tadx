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
	"testing"

	"github.com/ahillspace/tadx/internal/jobmonitor"
)

func TestAutomaticPublicationBulkPersistsBeforePooledObservation(t *testing.T) {
	for _, test := range []struct {
		kind        string
		unavailable bool
	}{{"workbook", false}, {"datasource", false}, {"workbook", true}, {"datasource", true}} {
		kind := test.kind
		t.Run(fmt.Sprintf("%s/unavailable=%v", kind, test.unavailable), func(t *testing.T) {
			jobDirectory := t.TempDir()
			posts, reads := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/users/user-1"):
					io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
				case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
					io.WriteString(w, `{"errors":[],"warnings":[]}`)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
					posts++
					if r.URL.Query().Get("asJob") != "true" {
						t.Error("supported async lifecycle was not automatic")
					}
					w.WriteHeader(http.StatusAccepted)
					fmt.Fprintf(w, `<tsResponse><job id="job-%d" type="Publish%s" progress="0" finishCode="1"/></tsResponse>`, posts, map[string]string{"workbook": "Workbook", "datasource": "Datasource"}[kind])
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/jobs/job-"):
					reads++
					if posts != 2 {
						t.Errorf("monitoring blocked later submission: posts=%d", posts)
					}
					files, _ := filepath.Glob(filepath.Join(jobDirectory, "*.json"))
					receipts := 0
					for _, path := range files {
						if strings.HasPrefix(filepath.Base(path), "active-") {
							continue
						}
						data, _ := os.ReadFile(path)
						var receipt jobmonitor.Receipt
						if json.Unmarshal(data, &receipt) == nil && receipt.Observation.ID != "" {
							receipts++
							if !receipt.PoolAfter.Equal(receipt.AcceptedAt) {
								t.Error("bulk acceptance did not enter pool immediately")
							}
						}
					}
					if receipts != 2 {
						t.Errorf("receipts before monitoring=%d", receipts)
					}
					child := ""
					if test.unavailable {
						child = fmt.Sprintf(`<%s id="published-%s"/>`, kind, filepath.Base(r.URL.Path))
					}
					fmt.Fprintf(w, `<tsResponse><job id="%s" type="Publish%s" progress="100" finishCode="0">%s</job></tsResponse>`, filepath.Base(r.URL.Path), map[string]string{"workbook": "Workbook", "datasource": "Datasource"}[kind], child)
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/"+kind+"s/published-"):
					http.Error(w, "destination not yet visible", http.StatusNotFound)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
					count, row := 0, ""
					if reads > 0 {
						count = 1
						name := "One"
						if strings.Contains(r.URL.Query().Get("filter"), "Two") {
							name = "Two"
						}
						row = fmt.Sprintf(`<%s id="published-%s" name="%s"><project id="project-1" name="Analytics"/></%s>`, kind, name, name, kind)
					}
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="%d"/><%ss>%s</%ss></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"), count, kind, row, kind)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
					http.Error(w, "unexpected", 400)
				}
			}))
			defer server.Close()
			runtime, _ := datasourceLifecycleRuntime(t, server)
			directory := t.TempDir()
			extension, content := ".twb", "<workbook/>"
			if kind == "datasource" {
				extension, content = ".tds", "<datasource/>"
			}
			args := []string{"content", kind, "publish", "--environment", "production", "--project-id", "project-1", "--json"}
			for _, name := range []string{"One", "Two"} {
				path := filepath.Join(directory, name+extension)
				if err := os.WriteFile(path, []byte(content), 0600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--file", path)
			}
			if kind == "datasource" {
				args = append(args, "--create")
			}
			var out, progress strings.Builder
			exit := Run(context.Background(), args, &out, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), MutationsEnabled: true, JobDirectory: jobDirectory, Stderr: &progress})
			if test.unavailable {
				if exit == 0 || posts != 2 || !strings.Contains(out.String(), `"outcome":"confirmed"`) || strings.Contains(out.String(), "Run without --preview") {
					t.Fatalf("exit=%d posts=%d out=%s", exit, posts, out.String())
				}
				for _, id := range []string{"job-1", "job-2"} {
					saved, _, err := (jobmonitor.Store{Directory: jobDirectory}).FindByJobID(t.Context(), id, "production", "")
					if err != nil || saved.Observation.Status != "succeeded" || saved.Observation.ResourceID == "" || saved.Verification != "destination_unavailable" {
						t.Fatalf("saved=%+v err=%v", saved, err)
					}
				}
				return
			}
			if exit != 0 {
				t.Fatalf("exit=%d output=%s progress=%s", exit, out.String(), progress.String())
			}
			if posts != 2 || reads != 2 || !strings.Contains(out.String(), "published-One") || !strings.Contains(out.String(), "published-Two") || strings.Contains(out.String(), "Run without --preview") || !strings.Contains(progress.String(), "accepted") {
				t.Fatalf("posts=%d reads=%d output=%s progress=%s", posts, reads, out.String(), progress.String())
			}
		})
	}
}
