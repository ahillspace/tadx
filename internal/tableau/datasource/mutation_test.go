package datasource_test

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"mime"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/tableau"
	tableaudatasource "github.com/ahillspace/tadx/internal/tableau/datasource"
)

// uploadedDatasourceBytes extracts the native payload part from a multipart/mixed publish body.
func uploadedDatasourceBytes(t *testing.T, request *http.Request) []byte {
	t.Helper()
	mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
	if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
		t.Fatalf("content type = %q, err = %v", request.Header.Get("Content-Type"), err)
	}
	reader := multipart.NewReader(request.Body, params["boundary"])
	for {
		part, err := reader.NextPart()
		if err == io.EOF {
			return nil
		}
		if err != nil {
			t.Fatalf("read multipart part: %v", err)
		}
		disposition := part.Header.Get("Content-Disposition")
		if strings.Contains(disposition, `name="tableau_datasource"`) {
			data, err := io.ReadAll(part)
			if err != nil {
				t.Fatalf("read payload part: %v", err)
			}
			return data
		}
	}
}

func publishFixture(t *testing.T, content string) (string, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "Sales.tds")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(content))
	return path, "sha256:" + hex.EncodeToString(digest[:])
}

func TestClientPublishesComposedDatasourceWithExactParentReferences(t *testing.T) {
	path, fingerprint := publishFixture(t, `<datasource/>`)
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/3.29/sites/site-1/datasources" || request.URL.Query().Get("overwrite") != "true" {
			t.Fatalf("request = %s %s?%s", request.Method, request.URL.Path, request.URL.RawQuery)
		}
		body, _ := io.ReadAll(request.Body)
		text := string(body)
		if strings.Count(text, "<parentDataSourceUrls>") != 2 || !strings.Contains(text, "<parentDataSourceUrls>parent-a</parentDataSourceUrls>") || !strings.Contains(text, "<parentDataSourceUrls>parent-b</parentDataSourceUrls>") {
			t.Fatalf("body = %s", text)
		}
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-new" name="Sales" contentUrl="Sales_1"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	prepared, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(`<datasource/>`)), ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishOverwrite, ParentDataSourceURLs: []string{"parent-a", "parent-b"}})
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Commit(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	if result.DatasourceLUID != "ds-new" || result.Status != "succeeded" || result.ContentURL != "Sales_1" {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientPublishesExactlyTheFingerprintedBytes(t *testing.T) {
	content := `<datasource><column name="Sales"/></datasource>`
	path, fingerprint := publishFixture(t, content)
	var uploaded []byte
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodPost || request.URL.Path != "/api/3.29/sites/site-1/datasources" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		uploaded = uploadedDatasourceBytes(t, request)
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-new" name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	prepared, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(content)), ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishCreate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256(uploaded)
	if got := "sha256:" + hex.EncodeToString(digest[:]); got != fingerprint {
		t.Fatalf("uploaded fingerprint = %s, expected %s (uploaded %q)", got, fingerprint, uploaded)
	}
}

func TestClientUploadsFingerprintedSnapshotDespitePostPrepareMutation(t *testing.T) {
	content := "abcdefgh"
	path, fingerprint := publishFixture(t, content)
	var uploaded []byte
	appended := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/fileUploads":
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1"/></tsResponse>`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/3.29/sites/site-1/fileUploads/upload-1":
			appended++
			mediaType, params, err := mime.ParseMediaType(request.Header.Get("Content-Type"))
			if err != nil || !strings.HasPrefix(mediaType, "multipart/") {
				t.Fatalf("append content type = %q, err = %v", request.Header.Get("Content-Type"), err)
			}
			reader := multipart.NewReader(request.Body, params["boundary"])
			for {
				part, err := reader.NextPart()
				if err == io.EOF {
					break
				}
				if err != nil {
					t.Fatalf("append part: %v", err)
				}
				if strings.Contains(part.Header.Get("Content-Disposition"), `name="tableau_file"`) {
					data, err := io.ReadAll(part)
					if err != nil {
						t.Fatalf("read append part: %v", err)
					}
					uploaded = append(uploaded, data...)
				}
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1"/></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-new" name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
		default:
			t.Fatalf("unexpected request = %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetUploadThreshold(1)
	client.SetUploadChunkSize(4)
	prepared, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(content)), ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishCreate})
	if err != nil {
		t.Fatal(err)
	}
	// Mutating the managed artifact after Prepare must not change what Commit uploads.
	if err := os.WriteFile(path, []byte("ZZZZZZZZ"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if string(uploaded) != content {
		t.Fatalf("uploaded %q, expected %q", uploaded, content)
	}
	digest := sha256.Sum256(uploaded)
	if got := "sha256:" + hex.EncodeToString(digest[:]); got != fingerprint {
		t.Fatalf("uploaded fingerprint = %s, expected %s", got, fingerprint)
	}
}

func TestClientRejectsDatasourcePublishWhenFingerprintMismatches(t *testing.T) {
	content := `<datasource/>`
	path, _ := publishFixture(t, content)
	client := tableaudatasource.NewClient(tableau.NewTransport(http.DefaultClient, "3.29", nil), session{}, "https://example.invalid")
	_, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(content)), ExpectedFingerprint: "sha256:0000000000000000000000000000000000000000000000000000000000000000", Mode: tableaudatasource.PublishCreate})
	if err == nil || !strings.Contains(err.Error(), "changed after planning") {
		t.Fatalf("error = %v, want fingerprint mismatch rejection", err)
	}
}

func TestClientRejectsComposedPublishBeforeREST329(t *testing.T) {
	path, fingerprint := publishFixture(t, `<datasource/>`)
	client := tableaudatasource.NewClient(tableau.NewTransport(http.DefaultClient, "3.28", nil), session{}, "https://example.invalid")
	_, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(`<datasource/>`)), ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishCreate, ParentDataSourceURLs: []string{"parent-a"}})
	if err == nil || !strings.Contains(err.Error(), "3.29") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientPublishesLargeDatasourceThroughOrderedUploadSession(t *testing.T) {
	path, fingerprint := publishFixture(t, "abcd")
	appendCount := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/fileUploads":
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1"/></tsResponse>`)
		case request.Method == http.MethodPut && request.URL.Path == "/api/3.29/sites/site-1/fileUploads/upload-1":
			appendCount++
			if request.URL.Query().Get("sequenceID") != string(rune('0'+appendCount)) {
				t.Fatalf("append query = %v", request.URL.Query())
			}
			writer.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(writer, `<tsResponse><fileUpload uploadSessionId="upload-1"/></tsResponse>`)
		case request.Method == http.MethodPost && request.URL.Path == "/api/3.29/sites/site-1/datasources":
			if request.URL.Query().Get("uploadSessionId") != "upload-1" || request.URL.Query().Get("datasourceType") != "tds" {
				t.Fatalf("publish query = %v", request.URL.Query())
			}
			writer.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-new" name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
		default:
			t.Fatalf("unexpected request = %s %s", request.Method, request.URL.String())
		}
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	client.SetUploadThreshold(1)
	client.SetUploadChunkSize(2)
	prepared, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: 4, ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishCreate})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := prepared.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if appendCount != 2 {
		t.Fatalf("append count = %d", appendCount)
	}
}

func TestClientRejectsReplaceBeforeREST325(t *testing.T) {
	path, fingerprint := publishFixture(t, `<datasource/>`)
	client := tableaudatasource.NewClient(tableau.NewTransport(http.DefaultClient, "3.24", nil), session{}, "https://example.invalid")
	_, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(`<datasource/>`)), ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishReplace})
	if err == nil || !strings.Contains(err.Error(), "3.25") {
		t.Fatalf("error = %v", err)
	}
}

func TestClientDeletesExactDatasourceOnlyOnEmpty204(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Method != http.MethodDelete || request.URL.Path != "/api/3.29/sites/site-1/datasources/ds-1" {
			t.Fatalf("request = %s %s", request.Method, request.URL.Path)
		}
		writer.Header().Set("X-Tableau-Request-Id", "delete-request")
		writer.WriteHeader(http.StatusNoContent)
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	result, err := client.Delete(context.Background(), "ds-1")
	if err != nil {
		t.Fatal(err)
	}
	if result.Status != "succeeded" || result.DatasourceLUID != "ds-1" || result.TableauRequestID != "delete-request" {
		t.Fatalf("result = %#v", result)
	}
}

func TestClientReportsUnknownWhenDatasourcePublishReturnsAJob(t *testing.T) {
	path, fingerprint := publishFixture(t, `<datasource/>`)
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		requests++
		if request.URL.Query().Has("asJob") {
			t.Errorf("publish query = %s", request.URL.RawQuery)
		}
		writer.Header().Set("Content-Type", "application/xml")
		writer.Header().Set("X-Tableau-Request-Id", "accepted-request")
		writer.WriteHeader(http.StatusAccepted)
		_, _ = io.WriteString(writer, `<tsResponse><job id="job-1" type="PublishDatasource" progress="0" finishCode="1"/></tsResponse>`)
	}))
	defer server.Close()
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	prepared, err := client.Prepare(context.Background(), tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(`<datasource/>`)), ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishCreate})
	if err != nil {
		t.Fatal(err)
	}
	result, err := prepared.Commit(context.Background())
	if err == nil || result.Status != "unknown" || result.TableauRequestID != "accepted-request" || requests != 1 {
		t.Fatalf("result = %#v, requests = %d, error = %v", result, requests, err)
	}
}

func TestPreparedDatasourcePublishKeepsSnapshotUntilCommitOrClose(t *testing.T) {
	content := `<datasource/>`
	path, fingerprint := publishFixture(t, content)
	publishes := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		publishes++
		writer.WriteHeader(http.StatusCreated)
		_, _ = io.WriteString(writer, `<tsResponse><datasource id="ds-new" name="Sales"><project id="project-1" name="Analytics"/></datasource></tsResponse>`)
	}))
	defer server.Close()
	snapshots := func() int {
		matches, err := filepath.Glob(filepath.Join(filepath.Dir(path), ".tadx-datasource-publish-snapshot-*"))
		if err != nil {
			t.Fatal(err)
		}
		return len(matches)
	}
	client := tableaudatasource.NewClient(tableau.NewTransport(server.Client(), "3.29", nil), session{}, server.URL)
	request := tableaudatasource.PublishRequest{Name: "Sales", ProjectLUID: "project-1", Filename: "Sales.tds", ContentPath: path, ContentSize: int64(len(content)), ExpectedFingerprint: fingerprint, Mode: tableaudatasource.PublishCreate}

	prepared, err := client.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if snapshots() != 1 {
		t.Fatal("prepared publish released its snapshot before Commit")
	}
	if _, err := prepared.Commit(context.Background()); err != nil {
		t.Fatal(err)
	}
	if snapshots() != 0 || prepared.Close() != nil {
		t.Fatal("Commit did not release the snapshot")
	}

	abandoned, err := client.Prepare(context.Background(), request)
	if err != nil {
		t.Fatal(err)
	}
	if err := abandoned.Close(); err != nil || snapshots() != 0 {
		t.Fatalf("Close() error = %v, snapshots = %d", err, snapshots())
	}
	if _, err := abandoned.Commit(context.Background()); err == nil || publishes != 1 {
		t.Fatalf("Commit after Close error = %v, publishes = %d", err, publishes)
	}
}
