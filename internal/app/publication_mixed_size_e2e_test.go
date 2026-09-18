package app

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

const mixedPublicationUploadThreshold = 64 * 1024 * 1024

type mixedPublicationBatch struct {
	Status    string `json:"status"`
	Total     int    `json:"total"`
	Succeeded int    `json:"succeeded"`
	Failed    int    `json:"failed"`
	Items     []struct {
		Selector string `json:"selector"`
		Status   string `json:"status"`
	} `json:"items"`
}

func TestDatasourcePublicationMixedSizeBatchUsesExactUploadsAndIndependentOutcomes(t *testing.T) {
	directory := t.TempDir()
	inlinePath := filepath.Join(directory, "inline.tds")
	if err := os.WriteFile(inlinePath, []byte("<datasource><connection/></datasource>"), 0o600); err != nil {
		t.Fatal(err)
	}
	chunkedPath := filepath.Join(directory, "chunked.tdsx")
	writeChunkedDatasource(t, chunkedPath)
	inlineBytes, err := os.ReadFile(inlinePath)
	if err != nil {
		t.Fatal(err)
	}
	chunkedBytes, err := os.ReadFile(chunkedPath)
	if err != nil {
		t.Fatal(err)
	}
	if int64(len(chunkedBytes)) <= mixedPublicationUploadThreshold {
		t.Fatalf("chunked fixture size = %d, threshold = %d", len(chunkedBytes), mixedPublicationUploadThreshold)
	}

	var (
		mu            sync.Mutex
		paths         []string
		counts        = make(map[string]int)
		inlinePayload []byte
		chunkedUpload []byte
		sequenceIDs   []string
	)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		mu.Lock()
		paths = append(paths, request.Method+" "+request.URL.RequestURI())
		counts[request.Method+" "+request.URL.Path]++
		mu.Unlock()

		switch {
		case strings.HasSuffix(request.URL.Path, "/auth/signin"):
			writer.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(writer, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(request.URL.Path, "/users/user-1"):
			_, _ = io.WriteString(writer, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize"))
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/datasources/ds-large":
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-large" name="chunked"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/datasources"):
			_, _ = fmt.Fprintf(writer, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><datasources/></tsResponse>`, request.URL.Query().Get("pageNumber"), request.URL.Query().Get("pageSize"))
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/fileUploads":
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-large"/></tsResponse>`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/3.29/sites/site-1/fileUploads/upload-large":
			payload := readMixedPublicationPart(t, request, "tableau_file")
			mu.Lock()
			chunkedUpload = append(chunkedUpload, payload...)
			sequenceIDs = append(sequenceIDs, request.URL.Query().Get("sequenceID"))
			mu.Unlock()
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-large"/></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			if request.URL.Query().Get("uploadSessionId") == "" {
				payload := readMixedPublicationPart(t, request, "tableau_datasource")
				mu.Lock()
				inlinePayload = append(inlinePayload, payload...)
				mu.Unlock()
				writer.Header().Set("X-Tableau-Request-Id", "inline-rejected")
				http.Error(writer, "fixture rejects the inline item", http.StatusBadRequest)
				return
			}
			if request.URL.Query().Get("uploadSessionId") != "upload-large" || request.URL.Query().Get("datasourceType") != "tdsx" {
				t.Errorf("chunked publish query = %s", request.URL.RawQuery)
			}
			writer.Header().Set("X-Tableau-Request-Id", "chunked-accepted")
			writer.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(writer, `<tsResponse><job id="job-large" type="PublishDatasource" progress="0" finishCode="1"/></tsResponse>`)
		case request.Method == http.MethodGet && request.URL.Path == "/api/3.29/sites/site-1/jobs/job-large":
			writer.Header().Set("X-Tableau-Request-Id", "chunked-observed")
			_, _ = io.WriteString(writer, `<tsResponse><job id="job-large" type="PublishDatasource" progress="100" finishCode="0"><datasource id="ds-large" name="chunked"><project id="project-1"/></datasource></job></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL)
			http.Error(writer, "unexpected fixture request", http.StatusNotFound)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)

	args := []string{"content", "datasource", "publish", "--environment", "production", "--project-id", "project-1", "--create", "--json", "--file", inlinePath, "--file", chunkedPath}
	var out, progress strings.Builder
	started := time.Now()
	exit := Run(context.Background(), args, &out, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), MutationsEnabled: true, JobDirectory: runtime.jobDirectory, Stderr: &progress})
	elapsed := time.Since(started)

	var result mixedPublicationBatch
	if err := json.Unmarshal([]byte(out.String()), &result); err != nil {
		t.Fatalf("decode batch output: %v\nexit=%d\noutput=%s\nprogress=%s", err, exit, out.String(), progress.String())
	}
	if exit == 0 || result.Status != "partial_failure" || result.Total != 2 || result.Succeeded != 1 || result.Failed != 1 {
		t.Fatalf("exit=%d result=%+v output=%s", exit, result, out.String())
	}
	if strings.Contains(out.String(), `"status":""`) {
		t.Fatalf("failed submission fabricated an empty result: %s", out.String())
	}
	if len(result.Items) != 2 || result.Items[0].Selector != "file="+inlinePath || result.Items[0].Status != "failed" || result.Items[1].Selector != "file="+chunkedPath || result.Items[1].Status != "succeeded" {
		t.Fatalf("per-item statuses = %#v output=%s", result.Items, out.String())
	}

	mu.Lock()
	gotInline, gotChunked, gotSequences := append([]byte(nil), inlinePayload...), append([]byte(nil), chunkedUpload...), append([]string(nil), sequenceIDs...)
	gotPaths, gotCounts := append([]string(nil), paths...), make(map[string]int, len(counts))
	for key, count := range counts {
		gotCounts[key] = count
	}
	mu.Unlock()
	if !bytes.Equal(gotInline, inlineBytes) {
		t.Fatalf("inline payload changed: got %d bytes, want %d", len(gotInline), len(inlineBytes))
	}
	if !bytes.Equal(gotChunked, chunkedBytes) {
		t.Fatalf("chunked payload changed: got %d bytes, want %d", len(gotChunked), len(chunkedBytes))
	}
	if len(gotSequences) != 2 || gotSequences[0] != "1" || gotSequences[1] != "2" {
		t.Fatalf("upload sequence IDs = %#v", gotSequences)
	}
	if gotCounts["POST /api/3.29/auth/signin"] != 1 || gotCounts["GET /api/3.29/sites/site-1/users/user-1"] != 1 {
		t.Fatalf("authentication request counts = %#v", gotCounts)
	}
	if gotCounts["POST /api/3.29/sites/site-1/datasources"] != 2 || gotCounts["POST /api/3.29/sites/site-1/fileUploads"] != 1 || gotCounts["PUT /api/3.29/sites/site-1/fileUploads/upload-large"] != 2 {
		t.Fatalf("publication request counts = %#v", gotCounts)
	}
	if strings.Count(progress.String(), "status: accepted") != 1 {
		t.Fatalf("accepted progress count = %d, progress=%s", strings.Count(progress.String(), "status: accepted"), progress.String())
	}
	t.Logf("mixed-size publication elapsed=%s requests=%d sign-ins=%d paths=%v", elapsed, len(gotPaths), gotCounts["POST /api/3.29/auth/signin"], gotPaths)
}

func writeChunkedDatasource(t *testing.T, path string) {
	t.Helper()
	file, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	archive := zip.NewWriter(file)
	definition := &zip.FileHeader{Name: "definition.tds", Method: zip.Store}
	part, err := archive.CreateHeader(definition)
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := io.WriteString(part, "<datasource/>"); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	filler := &zip.FileHeader{Name: "payload.bin", Method: zip.Store}
	part, err = archive.CreateHeader(filler)
	if err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if _, err := part.Write(bytes.Repeat([]byte{'x'}, mixedPublicationUploadThreshold+1024)); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := archive.Close(); err != nil {
		_ = file.Close()
		t.Fatal(err)
	}
	if err := file.Close(); err != nil {
		t.Fatal(err)
	}
}

func readMixedPublicationPart(t *testing.T, request *http.Request, wanted string) []byte {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || mediaType != "multipart/mixed" {
		t.Fatalf("multipart content type = %q, err = %v", request.Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(request.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			t.Fatalf("multipart part %q not found", wanted)
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		if strings.Contains(part.Header.Get("Content-Disposition"), `name="`+wanted+`"`) {
			payload, readErr := io.ReadAll(part)
			if readErr != nil {
				t.Fatalf("read %s payload: %v", wanted, readErr)
			}
			return payload
		}
	}
}
