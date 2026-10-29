package app_test

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

// streamingDatasourceServer serves each datasource in packages from its
// packaged file. A datasource mapped to "" sends part of a declared body and
// ends the response, so the client sees the connection drop mid-download.
func streamingDatasourceServer(t *testing.T, packages map[string]string) http.Handler {
	t.Helper()
	return http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		path := request.URL.Path
		switch {
		case strings.HasSuffix(path, "/auth/signin"):
			_, _ = io.WriteString(writer, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(path, "/projects"):
			_, _ = io.WriteString(writer, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
		case strings.HasSuffix(path, "/content"):
			luid := filepath.Base(strings.TrimSuffix(path, "/content"))
			source, ok := packages[luid]
			if !ok {
				http.Error(writer, "unexpected datasource", http.StatusNotFound)
				return
			}
			writer.Header().Set("Content-Disposition", fmt.Sprintf(`name="tableau_datasource"; filename="%s.tdsx"`, luid))
			if source == "" {
				writer.Header().Set("Content-Length", "1048576")
				_, _ = io.WriteString(writer, "partial package")
				return
			}
			file, err := os.Open(source)
			if err != nil {
				t.Error(err)
				return
			}
			defer file.Close()
			info, _ := file.Stat()
			writer.Header().Set("Content-Length", fmt.Sprint(info.Size()))
			_, _ = io.Copy(writer, file)
		case strings.Contains(path, "/datasources/"):
			luid := filepath.Base(path)
			_, _ = fmt.Fprintf(writer, `<tsResponse><datasource id="%s" name="Sales %s"><project id="project-1" name="Analytics"/></datasource></tsResponse>`, luid, luid)
		default:
			http.Error(writer, "metadata unavailable", http.StatusNotFound)
		}
	})
}

// datasourcePackage writes a .tdsx whose stored extract makes it extractBytes
// larger than its definition, and returns its path and size.
func datasourcePackage(t *testing.T, extractBytes int64) (string, int64) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "source.tdsx")
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	definition, err := archive.Create("Sales.tds")
	if err != nil {
		t.Fatal(err)
	}
	_, _ = io.WriteString(definition, `<datasource><connection class="hyper"/></datasource>`)
	extract, err := archive.CreateHeader(&zip.FileHeader{Name: "Data/Extracts/sales.hyper", Method: zip.Store})
	if err != nil {
		t.Fatal(err)
	}
	chunk := make([]byte, 1024*1024)
	for written := int64(0); written < extractBytes; {
		part := min(int64(len(chunk)), extractBytes-written)
		if _, err := extract.Write(chunk[:part]); err != nil {
			t.Fatal(err)
		}
		written += part
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	return path, info.Size()
}

func pulledPayloadSizes(t *testing.T, workspace string) map[string]int64 {
	t.Helper()
	sizes := map[string]int64{}
	root := filepath.Join(workspace, "artifacts", "datasource")
	entries, _ := os.ReadDir(root)
	for _, entry := range entries {
		matches, _ := filepath.Glob(filepath.Join(root, entry.Name(), "*.tdsx"))
		for _, match := range matches {
			info, err := os.Stat(match)
			if err != nil {
				t.Fatal(err)
			}
			sizes[strings.TrimSuffix(filepath.Base(match), ".tdsx")] = info.Size()
		}
	}
	return sizes
}

func assertNoDownloadStaging(t *testing.T, workspace string) {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(workspace, ".tadx", "tmp"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("download staging left behind: %v", entries)
	}
}

func TestDatasourcePullStreamsContentLargerThanTheBufferedResponseLimit(t *testing.T) {
	source, size := datasourcePackage(t, 256*1024*1024+1)
	server := tableauFixtureServer(t, streamingDatasourceServer(t, map[string]string{"ds-1": source}))
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	workspace := createNamedWorkspace(t, options.ConfigPath, "work")

	var output strings.Builder
	if exit := app.Run(context.Background(), []string{"content", "datasource", "pull", "--environment", "production", "--workspace", "work", "--id", "ds-1"}, &output, options); exit != 0 {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	if sizes := pulledPayloadSizes(t, workspace); sizes["ds-1"] != size {
		t.Fatalf("pulled payload sizes = %v, want ds-1 = %d", sizes, size)
	}
	assertNoDownloadStaging(t, workspace)
}

func TestDatasourceBatchPullStreamsEachItemAndDiscardsAFailedDownload(t *testing.T) {
	first, firstSize := datasourcePackage(t, 3<<20)
	second, secondSize := datasourcePackage(t, 5<<20)
	server := tableauFixtureServer(t, streamingDatasourceServer(t, map[string]string{"ds-1": first, "ds-2": second, "ds-3": ""}))
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	workspace := createNamedWorkspace(t, options.ConfigPath, "work")

	var output strings.Builder
	exit := app.Run(context.Background(), []string{"content", "datasource", "pull", "--environment", "production", "--workspace", "work", "--id", "ds-1", "--id", "ds-2", "--id", "ds-3"}, &output, options)
	if exit == 0 || !strings.Contains(output.String(), "failed: 1") || !strings.Contains(output.String(), "id: datasource.pull.download") || !strings.Contains(output.String(), "unexpected EOF") {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	sizes := pulledPayloadSizes(t, workspace)
	if len(sizes) != 2 || sizes["ds-1"] != firstSize || sizes["ds-2"] != secondSize {
		t.Fatalf("pulled payload sizes = %v", sizes)
	}
	assertNoDownloadStaging(t, workspace)
}
