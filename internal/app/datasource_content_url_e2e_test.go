package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDatasourcePublishReportsTableauSuffixedContentURLThroughCLI(t *testing.T) {
	for _, test := range []struct {
		name, siteRole string
	}{
		{"synchronous publish", "Creator"},
		{"asynchronous publish", "SiteAdministratorCreator"},
	} {
		t.Run(test.name, func(t *testing.T) {
			created := `<datasource id="ds-new" name="Sales" contentUrl="SalesMargin_17932075825460"><project id="project-1" name="Analytics"/><owner id="user-1"/></datasource>`
			server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				switch {
				case strings.HasSuffix(request.URL.Path, "/auth/signin"):
					writer.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(request.URL.Path, "/users/user-1"):
					_, _ = fmt.Fprintf(writer, `<tsResponse><user id="user-1" name="publisher" siteRole="%s"/></tsResponse>`, test.siteRole)
				case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/projects"):
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
				case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-new":
					_, _ = io.WriteString(writer, `<tsResponse>`+created+`</tsResponse>`)
				case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/datasources"):
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><datasources/></tsResponse>`)
				case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/datasources":
					if test.siteRole == "Creator" {
						writer.WriteHeader(http.StatusCreated)
						_, _ = io.WriteString(writer, `<tsResponse>`+created+`</tsResponse>`)
						return
					}
					writer.WriteHeader(http.StatusAccepted)
					_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" type="PublishDatasource" progress="0" finishCode="1"/></tsResponse>`)
				case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/jobs/job-1":
					_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" type="PublishDatasource" progress="100" finishCode="0"><datasource id="ds-new" name="Sales"><project id="project-1"/></datasource></job></tsResponse>`)
				default:
					t.Errorf("unexpected request %s %s", request.Method, request.URL)
					http.Error(writer, "unexpected fixture request", http.StatusNotFound)
				}
			}))
			defer server.Close()
			runtime, _ := datasourceLifecycleRuntime(t, server)
			file := filepath.Join(t.TempDir(), "Sales.tds")
			definition := `<datasource><repository-location derived-from='https://tableau.example.com/t/source-site/datasources/SalesMargin?rev=1.0' id='SalesMargin' path='/t/source-site/datasources' revision='1.0' site='source-site' /><connection class='postgres'/></datasource>`
			if err := os.WriteFile(file, []byte(definition), 0o600); err != nil {
				t.Fatal(err)
			}

			var out strings.Builder
			args := []string{"content", "datasource", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--create"}
			if exit := Run(context.Background(), args, &out, withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), JobDirectory: runtime.jobDirectory}, true)); exit != 0 {
				t.Fatalf("exit = %d, output = %s", exit, out.String())
			}
			for _, want := range []string{"content_url: SalesMargin_17932075825460", "renamed_from: SalesMargin", "--datasource-binding SalesMargin=ds-new"} {
				if !strings.Contains(out.String(), want) {
					t.Fatalf("output missing %q:\n%s", want, out.String())
				}
			}
		})
	}
}
