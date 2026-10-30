package app_test

import (
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestNativeWorkbookPublishPreviewStreamsLargeDefinition(t *testing.T) {
	server := tableauFixtureServer(t, publishFixture(t,
		func(http.ResponseWriter, *http.Request) { t.Error("preview submitted a workbook") },
		func(string, []string) string { t.Error("preview searched datasources"); return datasourcePage() },
		func(string) string { t.Error("preview read a datasource"); return "" },
	))
	defer server.Close()
	file := filepath.Join(t.TempDir(), "large.twb")
	payload, err := os.Create(file)
	if err != nil {
		t.Fatal(err)
	}
	defer payload.Close()
	if _, err := io.WriteString(payload, "<workbook><worksheets>"); err != nil {
		t.Fatal(err)
	}
	block := strings.Repeat("<worksheet/>", 1024)
	for range 17*1024*1024/len(block) + 1 {
		if _, err := io.WriteString(payload, block); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := io.WriteString(payload, "</worksheets></workbook>"); err != nil {
		t.Fatal(err)
	}
	if err := payload.Close(); err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	args := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--preview"}
	if exit := app.Run(t.Context(), args, &output, unmappedOptions(t, server.URL, server.Client())); exit != 0 {
		t.Fatalf("large native workbook preview failed: exit=%d\n%s", exit, output.String())
	}
}
