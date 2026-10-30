package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkbookDependencyPreviewLeavesWorkspaceUnchanged(t *testing.T) {
	for _, outcome := range []string{"success", "invalid workbook", "interrupted download", "canceled download"} {
		t.Run(outcome, func(t *testing.T) {
			workbook, _ := contentPackage(t, "source.twbx", "Book.twb", boundWorkbook, "Data/book.hyper", 1024)
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			server := tableauFixtureServer(t, http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				if diagnosticSignIn(writer, request) {
					return
				}
				writer.Header().Set("Content-Type", "application/xml")
				switch request.URL.Path {
				case "/api/3.29/sites/site-1/workbooks/wb-bound":
					_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-bound" name="Bound Book"><project id="project-1" name="Analytics"/></workbook></tsResponse>`)
				case "/api/3.29/sites/site-1/projects":
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Analytics"/></projects></tsResponse>`)
				case "/api/3.29/sites/site-1/datasources":
					_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1" totalAvailable="1"/><datasources><datasource id="ds-1" name="Sales" contentUrl="Sales"><project id="project-1" name="Analytics"/></datasource></datasources></tsResponse>`)
				case "/api/3.29/sites/site-1/workbooks/wb-bound/content":
					switch outcome {
					case "invalid workbook":
						writer.Header().Set("Content-Disposition", `name="tableau_workbook"; filename="Book.twb"`)
						_, _ = io.WriteString(writer, "<workbook>")
					case "interrupted download":
						writer.Header().Set("Content-Disposition", `name="tableau_workbook"; filename="Book.twb"`)
						writer.Header().Set("Content-Length", "1048576")
						_, _ = io.WriteString(writer, "partial")
					case "canceled download":
						cancel()
						<-request.Context().Done()
					default:
						servePackage(t, writer, workbook, "Book.twbx")
					}
				default:
					t.Errorf("unexpected preview request %s %s", request.Method, request.URL.Path)
					http.Error(writer, "unexpected request", http.StatusNotFound)
				}
			}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			workspace := createNamedWorkspace(t, options.ConfigPath, "work")
			before := acquisitionSnapshot(t, workspace)
			var output strings.Builder
			exit := app.Run(ctx, []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "work", "--id", "wb-bound", "--include-pds", "--preview"}, &output, options)
			if outcome == "success" {
				if exit != 0 || !strings.Contains(output.String(), "status: preview") || !strings.Contains(output.String(), "ds-1") {
					t.Fatalf("dependency preview failed: exit=%d\n%s", exit, output.String())
				}
			} else if exit == 0 {
				t.Fatalf("preview unexpectedly accepted %s: %s", outcome, output.String())
			}
			if after := acquisitionSnapshot(t, workspace); !reflect.DeepEqual(before, after) {
				t.Fatalf("preview changed workspace: before=%v after=%v", before, after)
			}
		})
	}
}

func TestPullStagingFailureRendersWorkspaceRelativePaths(t *testing.T) {
	for _, jsonOutput := range []bool{false, true} {
		t.Run(fmt.Sprintf("json=%t", jsonOutput), func(t *testing.T) {
			server := tableauFixtureServer(t, streamingDatasourceServer(t, map[string]string{"ds-1": ""}))
			defer server.Close()
			options := cacheResilienceOptions(t, server)
			workspace := createNamedWorkspace(t, options.ConfigPath, "work")
			if err := os.Mkdir(filepath.Join(workspace, ".tadx", "tmp.lock"), 0o700); err != nil {
				t.Fatal(err)
			}
			args := []string{"content", "datasource", "pull", "--id", "ds-1", "--workspace", "work", "--environment", "production"}
			if jsonOutput {
				args = append(args, "--json")
			}
			var output strings.Builder
			if exit := app.Run(t.Context(), args, &output, options); exit == 0 {
				t.Fatal("expected staging lock failure")
			}
			for _, private := range []string{workspace, filepath.ToSlash(workspace), strings.Trim(strconv.Quote(workspace), `"`)} {
				if strings.Contains(output.String(), private) {
					t.Fatalf("staging error exposed workspace root: %s", strings.ReplaceAll(output.String(), private, "<workspace>"))
				}
			}
			if !strings.Contains(output.String(), ".tadx/tmp.lock") {
				t.Fatalf("staging error lost its relative path: %s", output.String())
			}
		})
	}
}
