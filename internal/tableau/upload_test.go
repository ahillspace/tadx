package tableau

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestFileUploadRejectsOversizedUploadSessionResponses(t *testing.T) {
	oversized := `<tsResponse><fileUpload uploadSessionId="upload-1"/>` + strings.Repeat(" ", uploadResponseLimit) + `</tsResponse>`
	for _, test := range []struct {
		name      string
		operation string
	}{
		{name: "initiate", operation: "workbook.upload.initiate"},
		{name: "append", operation: "workbook.upload.append"},
	} {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
				_, _ = io.Copy(io.Discard, request.Body)
				body := `<tsResponse><fileUpload uploadSessionId="upload-1"/></tsResponse>`
				status := http.StatusOK
				if request.Method == http.MethodPost {
					status = http.StatusCreated
				}
				if strings.HasSuffix(test.operation, "initiate") == (request.Method == http.MethodPost) {
					body = oversized
				}
				writer.WriteHeader(status)
				_, _ = io.WriteString(writer, body)
			}))
			defer server.Close()

			snapshot := testSnapshot(t, "payload")
			defer snapshot.Close()
			upload := FileUpload{Transport: NewTransport(server.Client(), "3.29", nil), Session: testSession{token: "session-token"}, ServerURL: server.URL, Operation: "workbook", Filename: "Sales.twbx", ChunkSize: 1024}
			if _, err := upload.Upload(context.Background(), snapshot); err == nil || !strings.Contains(err.Error(), test.operation) {
				t.Fatalf("Upload error = %v, want %s response limit failure", err, test.operation)
			}
		})
	}
}

func testSnapshot(t *testing.T, content string) *PublishSnapshot {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload.twbx")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	digest := sha256.Sum256([]byte(content))
	snapshot, err := SnapshotPublishPayload(context.Background(), path, int64(len(content)), "sha256:"+hex.EncodeToString(digest[:]))
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func TestFileUploadRejectsNonpositiveChunkSize(t *testing.T) {
	for _, size := range []int64{0, -1} {
		if _, err := (FileUpload{ChunkSize: size}).Upload(t.Context(), nil); err == nil || err.Error() != "upload chunk size must be positive" {
			t.Fatalf("chunk size %d: error = %v", size, err)
		}
	}
}
