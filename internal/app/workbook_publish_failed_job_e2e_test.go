package app

import (
	"context"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/artifact"
)

// failedJobWorkbook references one published datasource whose content URL is
// absent from the destination site.
const failedJobWorkbook = `<?xml version='1.0' encoding='utf-8' ?>
<workbook version='18.1'>
  <datasources>
    <datasource caption='Sales &amp; Margin' inline='true' name='sqlproxy.0abc' version='18.1'>
      <repository-location derived-from='https://tableau.example.com/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' />
      <connection channel='https' class='sqlproxy' dbname='SalesMargin' directory='dataserver' port='443' server='tableau.example.com'>
      </connection>
    </datasource>
  </datasources>
</workbook>
`

func TestWorkbookPublishExplainsFailedJobThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name  string
		batch bool
	}{
		{"single publish", false},
		{"batch publish", true},
	} {
		t.Run(test.name, func(t *testing.T) {
			var submissions atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch {
				case strings.HasSuffix(request.URL.Path, "/auth/signin"):
					writer.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(request.URL.Path, "/users/user-1"):
					_, _ = io.WriteString(writer, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
				case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/projects"):
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
				case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/datasources"):
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><datasources><datasource id="ds-name" name="Sales &amp; Margin" contentUrl="SalesMargin_1" updatedAt="2026-01-02T03:04:05Z"><project id="project-1" name="Analytics"/><owner id="user-1"/></datasource></datasources></tsResponse>`)
				case strings.HasSuffix(request.URL.Path, "/validateWorkbook"):
					_, _ = io.WriteString(writer, `{"errors":[],"warnings":[]}`)
				case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/workbooks"):
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><workbooks/></tsResponse>`)
				case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/workbooks"):
					submissions.Add(1)
					writer.WriteHeader(http.StatusAccepted)
					_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" mode="Asynchronous" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
				case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/jobs/job-1"):
					_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" mode="Asynchronous" type="PublishWorkbook" progress="100" finishCode="1"><statusNotes/></job></tsResponse>`)
				default:
					t.Errorf("unexpected request %s %s", request.Method, request.URL)
					http.Error(writer, "unexpected fixture request", http.StatusNotFound)
				}
			}))
			defer server.Close()
			runtime, workspace := datasourceLifecycleRuntime(t, server)
			args := []string{"content", "workbook", "publish", "--environment", "production", "--project-id", "project-1"}
			if test.batch {
				manager := artifact.NewWorkbookManager(nil)
				for _, item := range []struct{ id, name string }{{"source-a", "First"}, {"source-b", "Second"}} {
					if _, err := manager.Pull(t.Context(), artifact.WorkbookPull{
						Workspace: workspace, Filename: item.name + ".twb", Content: []byte(failedJobWorkbook),
						Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: item.name, TableauID: item.id, SourceServerOrigin: server.URL, SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "team-site", SourceProjectID: "project-1", SourceProjectName: "Analytics"},
					}); err != nil {
						t.Fatal(err)
					}
				}
				args = append(args, "--workspace", "analytics", "--id", "source-a", "--id", "source-b")
			} else {
				file := filepath.Join(t.TempDir(), "Finance.twb")
				if err := os.WriteFile(file, []byte(failedJobWorkbook), 0o600); err != nil {
					t.Fatal(err)
				}
				args = append(args, "--file", file)
			}

			var out strings.Builder
			if exit := Run(context.Background(), args, &out, withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), JobDirectory: runtime.jobDirectory}, true)); exit == 0 {
				t.Fatalf("failed publish job succeeded:\n%s", out.String())
			}
			want := []string{"Your workbook contains 1 data source(s) that do not map to a published data source on this Tableau Site.", "LUID ds-name", "--datasource-binding SalesMargin=ds-name", "not_applied", "job-1"}
			for _, want := range want {
				if strings.Count(out.String(), want) < int(submissions.Load()) {
					t.Fatalf("output missing %q for %d submissions:\n%s", want, submissions.Load(), out.String())
				}
			}
			if strings.Contains(out.String(), "workbook.publish.monitor") {
				t.Fatalf("generic job failure remained:\n%s", out.String())
			}
		})
	}
}
