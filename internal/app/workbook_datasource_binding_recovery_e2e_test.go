package app_test

import (
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkbookPublishRecoversWithExactContentURLDespiteDuplicateCaptions(t *testing.T) {
	var uploads atomic.Int32
	server := tableauFixtureServer(t, publishFixture(t,
		func(writer http.ResponseWriter, request *http.Request) {
			body, err := io.ReadAll(request.Body)
			if err != nil {
				t.Errorf("read upload: %v", err)
				http.Error(writer, "read upload", http.StatusBadRequest)
				return
			}
			payload := string(body)
			if uploads.Add(1) == 1 {
				rejectMissingDatasource("Orders")(writer, request)
				return
			}
			for _, want := range []string{"dbname='OrdersTarget'", "dbname='OrdersArchive'"} {
				if !strings.Contains(payload, want) {
					t.Errorf("upload missing %q:\n%s", want, payload)
				}
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><workbook id="wb-bound" name="Finance"><project id="project-1"/></workbook></tsResponse>`)
		},
		func(field string, _ []string) string {
			if field == "name" {
				return datasourcePage(`<datasource id="ds-target" name="Orders" contentUrl="OrdersTarget"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource>`)
			}
			return datasourcePage()
		},
		func(luid string) string {
			if luid != "ds-target" {
				t.Errorf("unexpected target %q", luid)
			}
			return `<tsResponse><datasource id="ds-target" name="Orders" contentUrl="OrdersTarget"><project id="project-1" name="Finance"/><owner id="user-1"/></datasource></tsResponse>`
		},
	))
	defer server.Close()
	options := unmappedOptions(t, server.URL, server.Client())
	contents := workbookWithDatasources(datasourceReference{"Orders", "Orders"}, datasourceReference{"Orders", "OrdersArchive"})
	file := writeWorkbook(t, contents)
	publish := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1"}

	var rejected strings.Builder
	if exit := app.Run(t.Context(), publish, &rejected, options); exit == 0 {
		t.Fatalf("unmapped publish succeeded:\n%s", rejected.String())
	}
	generated := regexp.MustCompile(`--datasource-binding (Orders=[^\s]+)`).FindStringSubmatch(rejected.String())
	if len(generated) != 2 || generated[1] != "Orders=ds-target" {
		t.Fatalf("missing exact content URL recovery binding:\n%s", rejected.String())
	}
	bound := append(publish, "--datasource-binding", generated[1])
	var preview strings.Builder
	if exit := app.Run(t.Context(), append(bound, "--preview"), &preview, options); exit != 0 {
		t.Fatalf("generated binding preview exit = %d:\n%s", exit, preview.String())
	}
	if uploads.Load() != 1 {
		t.Fatalf("preview uploaded: %d uploads", uploads.Load())
	}
	var published strings.Builder
	if exit := app.Run(t.Context(), bound, &published, options); exit != 0 {
		t.Fatalf("generated binding publish exit = %d:\n%s", exit, published.String())
	}
	if uploads.Load() != 2 || !strings.Contains(published.String(), "workbook_luid: wb-bound") {
		t.Fatalf("bound publish uploads = %d:\n%s", uploads.Load(), published.String())
	}
	if after, err := os.ReadFile(file); err != nil || string(after) != contents {
		t.Fatalf("publish changed source workbook: %v", err)
	}
}
