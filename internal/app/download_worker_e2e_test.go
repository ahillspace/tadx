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
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestNoWaitPullWorkerPersistsDetachedNativeArtifacts(t *testing.T) {
	for _, test := range []struct {
		kind string
		ext  string
		body string
	}{
		{kind: "workbook", ext: ".twb", body: `<workbook/>`},
		{kind: "datasource", ext: ".tds", body: `<datasource/>`},
		{kind: "flow", ext: ".tfl", body: `<flow/>`},
	} {
		t.Run(test.kind, func(t *testing.T) {
			fixture := newDownloadWorkerFixture(t, test.kind, "item-1", "", test.body, test.ext)
			defer fixture.close()
			runtime, workspace := datasourceLifecycleRuntime(t, fixture.server)
			options, workerDone := downloadWorkerOptions(runtime, fixture.server, t.TempDir(), t.TempDir())

			var output strings.Builder
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			exit := Run(ctx, []string{
				"content", test.kind, "pull", "--environment", "production", "--workspace", "analytics",
				"--id", "item-1", "--no-wait", "--json",
			}, &output, options)
			if exit != 0 {
				t.Fatalf("no-wait pull exit=%d output=%s", exit, output.String())
			}

			receipt := decodeDownloadReceipt(t, output.String())
			if receipt.OperationID == "" || !strings.Contains(receipt.CheckStatus, "job inspect --operation-id "+receipt.OperationID) {
				t.Fatalf("output=%s", output.String())
			}
			select {
			case <-fixture.contentStarted:
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not reach the held native download")
			}

			store := operationrun.Store{Directory: options.OperationDirectory}
			record, err := store.Read(receipt.OperationID)
			if err != nil {
				t.Fatal(err)
			}
			if !record.FinishedAt.IsZero() {
				t.Fatalf("worker completed before held download was released: %+v", record)
			}
			rawRecord, err := os.ReadFile(filepath.Join(options.OperationDirectory, receipt.OperationID+".json"))
			if err != nil {
				t.Fatal(err)
			}
			for _, secret := range []string{"pat-secret", "download-session-token"} {
				if strings.Contains(string(rawRecord), secret) {
					t.Fatalf("operation record leaked credential material %q", secret)
				}
			}

			fixture.release()
			if code := waitDownloadWorker(t, workerDone); code != 0 {
				record, _ = store.Read(receipt.OperationID)
				t.Fatalf("worker exit=%d record=%s", code, record.FullResult)
			}
			record, err = store.Read(receipt.OperationID)
			if err != nil || record.Phase != operationrun.PhaseCompleted || record.ExitCode == nil || *record.ExitCode != 0 {
				t.Fatalf("final record=%+v err=%v", record, err)
			}
			path := downloadArtifactPath(t, record)
			assertNativeDownload(t, workspace, path, test.kind, test.ext, test.body)
			if fixture.jobs.Load() != 0 || fixture.contents.Load() != 1 {
				t.Fatalf("remote job reads=%d content reads=%d", fixture.jobs.Load(), fixture.contents.Load())
			}
		})
	}
}

func TestDefaultPullWorkerWaitsForNativeArtifact(t *testing.T) {
	fixture := newDownloadWorkerFixture(t, "workbook", "item-1", "", `<workbook/>`, ".twb")
	defer fixture.close()
	runtime, workspace := datasourceLifecycleRuntime(t, fixture.server)
	options, workerDone := downloadWorkerOptions(runtime, fixture.server, t.TempDir(), t.TempDir())

	type result struct {
		exit   int
		output string
	}
	resultDone := make(chan result, 1)
	go func() {
		var output strings.Builder
		exit := Run(context.Background(), []string{
			"content", "workbook", "pull", "--environment", "production", "--workspace", "analytics", "--id", "item-1", "--json",
		}, &output, options)
		resultDone <- result{exit: exit, output: output.String()}
	}()
	select {
	case <-fixture.contentStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("pull did not reach the held native download")
	}
	select {
	case result := <-resultDone:
		t.Fatalf("default wait returned before release: %+v", result)
	case <-time.After(100 * time.Millisecond):
	}
	fixture.release()
	select {
	case result := <-resultDone:
		if result.exit != 0 {
			t.Fatalf("default wait exit=%d output=%s", result.exit, result.output)
		}
		receipt := decodeDownloadReceipt(t, result.output)
		if receipt.OperationID == "" {
			t.Fatalf("default wait did not return operation identity: %s", result.output)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("default wait did not return after release")
	}
	if code := waitDownloadWorker(t, workerDone); code != 0 {
		t.Fatalf("worker exit=%d", code)
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	records := downloadOperationRecords(t, store)
	if len(records) != 1 {
		t.Fatalf("operation records=%d want1", len(records))
	}
	assertNativeDownload(t, workspace, downloadArtifactPath(t, records[0]), "workbook", ".twb", `<workbook/>`)
	if fixture.jobs.Load() != 0 {
		t.Fatalf("remote job reads=%d", fixture.jobs.Load())
	}
}

func TestNoWaitPullWorkerProcessPersistsNativeArtifact(t *testing.T) {
	fixture := newDownloadWorkerFixture(t, "workbook", "item-1", "", `<workbook/>`, ".twb")
	defer fixture.close()
	runtime, workspace := datasourceLifecycleRuntime(t, fixture.server)
	options, workerDone := downloadWorkerOptions(runtime, fixture.server, t.TempDir(), t.TempDir())
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchPublicationWorkerProcessTest(ctx, directory, id, options.JobDirectory, fixture.server.Certificate().Raw, workerDone)
	}

	var output strings.Builder
	ctx, cancel := context.WithTimeout(t.Context(), 15*time.Second)
	defer cancel()
	exit := Run(ctx, []string{
		"content", "workbook", "pull", "--environment", "production", "--workspace", "analytics", "--id", "item-1", "--no-wait", "--json",
	}, &output, options)
	if exit != 0 {
		t.Fatalf("process worker exit=%d output=%s", exit, output.String())
	}
	receipt := decodeDownloadReceipt(t, output.String())
	select {
	case <-fixture.contentStarted:
	case <-time.After(8 * time.Second):
		t.Fatal("process worker did not reach the held native download")
	}
	fixture.release()
	if code := waitDownloadWorker(t, workerDone); code != 0 {
		t.Fatalf("process worker exit=%d", code)
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	record, err := store.Read(receipt.OperationID)
	if err != nil {
		t.Fatal(err)
	}
	assertNativeDownload(t, workspace, downloadArtifactPath(t, record), "workbook", ".twb", `<workbook/>`)
}

func TestNoWaitPullBatchUsesOneOperationForMixedOutcomes(t *testing.T) {
	fixture := newDownloadWorkerFixture(t, "workbook", "pending", "failed", `<workbook/>`, ".twb")
	defer fixture.close()
	runtime, workspace := datasourceLifecycleRuntime(t, fixture.server)
	options, workerDone := downloadWorkerOptions(runtime, fixture.server, t.TempDir(), t.TempDir())

	var output strings.Builder
	ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
	defer cancel()
	exit := Run(ctx, []string{
		"content", "workbook", "pull", "--environment", "production", "--workspace", "analytics",
		"--id", "failed", "--id", "pending", "--no-wait", "--json",
	}, &output, options)
	if exit != 0 {
		t.Fatalf("batch no-wait exit=%d output=%s", exit, output.String())
	}
	receipt := decodeDownloadReceipt(t, output.String())
	if receipt.OperationID == "" || !strings.Contains(receipt.CheckStatus, "job inspect --operation-id "+receipt.OperationID) {
		t.Fatalf("output=%s", output.String())
	}
	select {
	case <-fixture.contentStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("batch worker did not reach the held successful item")
	}
	fixture.release()
	if code := waitDownloadWorker(t, workerDone); code == 0 {
		t.Fatal("mixed batch unexpectedly succeeded")
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	records := downloadOperationRecords(t, store)
	if len(records) != 1 {
		t.Fatalf("operation records=%d want1", len(records))
	}
	record := records[0]
	if !strings.Contains(string(record.FullResult), "failed") || !strings.Contains(string(record.FullResult), "succeeded") {
		t.Fatalf("mixed outcomes were not retained: %s", record.FullResult)
	}
	assertNativeDownload(t, workspace, downloadArtifactPath(t, record), "workbook", ".twb", `<workbook/>`)
	if fixture.jobs.Load() != 0 || fixture.contents.Load() != 1 {
		t.Fatalf("remote job reads=%d content reads=%d", fixture.jobs.Load(), fixture.contents.Load())
	}
}

type downloadReceipt struct {
	OperationID string `json:"operation_id"`
	CheckStatus string `json:"check_status"`
}

func decodeDownloadReceipt(t *testing.T, output string) downloadReceipt {
	t.Helper()
	var receipt downloadReceipt
	if err := json.Unmarshal([]byte(output), &receipt); err != nil {
		t.Fatalf("decode operation receipt %q: %v", output, err)
	}
	return receipt
}

func downloadWorkerOptions(runtime *runtimeDependencies, server *httptest.Server, operationDirectory, jobDirectory string) (Options, chan int) {
	done := make(chan int, 1)
	options := Options{
		ConfigPath: runtime.configPath,
		HTTPClient: server.Client(),

		PublicationWorkers: true,
		OperationDirectory: operationDirectory,
		JobDirectory:       jobDirectory,
		Stderr:             io.Discard,
	}
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchInProcessPublicationWorkerTest(ctx, directory, id, options, done)
	}
	return options, done
}

func waitDownloadWorker(t *testing.T, done <-chan int) int {
	t.Helper()
	select {
	case code := <-done:
		return code
	case <-time.After(10 * time.Second):
		t.Fatal("download worker did not finish")
		return 1
	}
}

func downloadOperationRecords(t *testing.T, store operationrun.Store) []operationrun.Record {
	t.Helper()
	entries, err := os.ReadDir(store.Directory)
	if err != nil {
		t.Fatal(err)
	}
	var records []operationrun.Record
	for _, entry := range entries {
		if entry.IsDir() || filepath.Ext(entry.Name()) != ".json" || strings.Contains(entry.Name(), ".batch.") {
			continue
		}
		record, err := store.Read(strings.TrimSuffix(entry.Name(), ".json"))
		if err != nil {
			t.Fatal(err)
		}
		records = append(records, record)
	}
	return records
}

func downloadArtifactPath(t *testing.T, record operationrun.Record) string {
	t.Helper()
	for _, data := range [][]byte{record.FullResult, record.CompactResult} {
		if path := findDownloadArtifactPath(data); path != "" {
			return path
		}
	}
	t.Fatalf("operation result omitted artifact path: full=%s compact=%s", record.FullResult, record.CompactResult)
	return ""
}

func findDownloadArtifactPath(data []byte) string {
	var value any
	if json.Unmarshal(data, &value) != nil {
		return ""
	}
	var visit func(any) string
	visit = func(value any) string {
		switch value := value.(type) {
		case map[string]any:
			if path, ok := value["path"].(string); ok && strings.HasPrefix(filepath.ToSlash(path), "artifacts/") {
				return filepath.ToSlash(path)
			}
			for _, child := range value {
				if path := visit(child); path != "" {
					return path
				}
			}
		case []any:
			for _, child := range value {
				if path := visit(child); path != "" {
					return path
				}
			}
		}
		return ""
	}
	return visit(value)
}

func assertNativeDownload(t *testing.T, workspace, relativePath, kind, extension, body string) {
	t.Helper()
	if filepath.IsAbs(filepath.FromSlash(relativePath)) || !strings.HasPrefix(relativePath, "artifacts/"+kind+"/") {
		t.Fatalf("artifact path=%q", relativePath)
	}
	directory := filepath.Join(workspace, filepath.FromSlash(relativePath))
	if info, err := os.Stat(directory); err != nil || !info.IsDir() {
		t.Fatalf("artifact directory=%q info=%v err=%v", directory, info, err)
	}
	metadata, err := os.ReadFile(filepath.Join(directory, "metadata.json"))
	var metadataDocument struct {
		TableauID string `json:"tableau_id"`
	}
	decodeErr := json.Unmarshal(metadata, &metadataDocument)
	if err != nil || decodeErr != nil || metadataDocument.TableauID == "" {
		t.Fatalf("artifact metadata=%q read_err=%v decode_err=%v", metadata, err, decodeErr)
	}
	payload, err := os.ReadFile(filepath.Join(directory, filepath.Base(relativePath)+extension))
	if err != nil {
		matches, globErr := filepath.Glob(filepath.Join(directory, "*"+extension))
		if globErr != nil || len(matches) != 1 {
			t.Fatalf("payload path=%q err=%v matches=%v globErr=%v", relativePath, err, matches, globErr)
		}
		payload, err = os.ReadFile(matches[0])
	}
	if err != nil || string(payload) != body {
		t.Fatalf("payload=%q err=%v want=%q", payload, err, body)
	}
}

type downloadWorkerFixture struct {
	server         *httptest.Server
	releaseChannel chan struct{}
	contentStarted chan struct{}
	contents       atomic.Int32
	jobs           atomic.Int32
	releaseOnce    sync.Once
	startOnce      sync.Once
}

func newDownloadWorkerFixture(t *testing.T, kind, heldID, failedID, body, extension string) *downloadWorkerFixture {
	t.Helper()
	fixture := &downloadWorkerFixture{releaseChannel: make(chan struct{}), contentStarted: make(chan struct{})}
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"download-session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		if strings.Contains(r.URL.Path, "/jobs/") {
			fixture.jobs.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if strings.HasSuffix(r.URL.Path, "/projects") {
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Shared"/></projects></tsResponse>`)
			return
		}
		prefix := "/api/3.29/sites/site-1/" + kind + "s/"
		if strings.HasPrefix(r.URL.Path, prefix) {
			tail := strings.TrimPrefix(r.URL.Path, prefix)
			id := strings.TrimSuffix(tail, "/content")
			if id == failedID {
				w.WriteHeader(http.StatusNotFound)
				_, _ = io.WriteString(w, `<tsResponse><error code="404006"><summary>Not found</summary></error></tsResponse>`)
				return
			}
			if strings.HasSuffix(tail, "/content") {
				fixture.contents.Add(1)
				fixture.startOnce.Do(func() { close(fixture.contentStarted) })
				if id == heldID {
					<-fixture.releaseChannel
				}
				w.Header().Set("Content-Disposition", `attachment; filename="Example`+extension+`"`)
				_, _ = io.WriteString(w, body)
				return
			}
			_, _ = fmt.Fprintf(w, `<tsResponse><%s id="%s" name="Example" fileType="tfl"><project id="project-1" name="Shared"/></%s></tsResponse>`, kind, id, kind)
			return
		}
		if r.URL.Path == "/api/metadata/graphql" {
			w.WriteHeader(http.StatusForbidden)
			return
		}
		t.Errorf("unexpected download fixture request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected", http.StatusNotFound)
	}))
	return fixture
}

func (f *downloadWorkerFixture) release() {
	f.releaseOnce.Do(func() { close(f.releaseChannel) })
}

func (f *downloadWorkerFixture) close() {
	f.release()
	f.server.Close()
}
