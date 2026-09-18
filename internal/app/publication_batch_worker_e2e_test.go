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
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestPublicationWorkerDefaultWaitsForAcceptedJob(t *testing.T) {
	var writes, reads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/users/user-1"):
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><workbooks/></tsResponse>`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			writes.Add(1)
			if r.URL.Query().Get("asJob") != "true" {
				t.Errorf("publish query = %s, want asJob=true", r.URL.RawQuery)
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `<tsResponse><job id="job-default" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-default"):
			reads.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><job id="job-default" type="PublishWorkbook" progress="100" finishCode="0"><workbook id="workbook-default"/></job></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks/workbook-default"):
			_, _ = io.WriteString(w, `<tsResponse><workbook id="workbook-default" name="Default"><project id="project-1" name="Analytics"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected Tableau request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Default.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	done := make(chan int, 1)
	options.WorkerLauncher = func(_ context.Context, directory, id string) error {
		go func() { done <- runPublicationWorker(context.Background(), directory, id, options) }()
		return nil
	}

	var output strings.Builder
	if exit := Run(t.Context(), []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &output, options); exit != 0 {
		t.Fatalf("default wait exit=%d output=%s", exit, output.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatalf("default wait output is not JSON: %v\n%s", err, output.String())
	}
	operationID, _ := result["operation_id"].(string)
	if operationID == "" || result["status"] != "succeeded" {
		t.Fatalf("default wait result = %#v", result)
	}
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("default wait worker exit=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("default wait worker did not finish")
	}
	if writes.Load() != 1 || reads.Load() == 0 {
		t.Fatalf("writes=%d job reads=%d, want one write and completion polling", writes.Load(), reads.Load())
	}
	record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
	if err != nil || record.Phase != operationrun.PhaseCompleted || record.ExitCode == nil || *record.ExitCode != 0 {
		t.Fatalf("default wait record=%+v err=%v", record, err)
	}
}

func TestPublicationWorkerRepeatedIDBatchPersistsMixedFailureAndPendingUnderOneOperation(t *testing.T) {
	var writes, reads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/users/user-1"):
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><workbooks/></tsResponse>`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			if writes.Add(1) == 1 {
				w.WriteHeader(http.StatusBadRequest)
				_, _ = io.WriteString(w, `<tsResponse><error code="400000"><summary>forced batch failure</summary><detail>first item rejected</detail></error></tsResponse>`)
				return
			}
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `<tsResponse><job id="job-batch-pending" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/jobs/"):
			reads.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><job id="job-batch-pending" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
		default:
			t.Errorf("unexpected Tableau request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	runtime, workspace := datasourceLifecycleRuntime(t, server)
	manager := artifact.NewWorkbookManager(nil)
	for _, item := range []struct{ id, name string }{{"source-a", "First"}, {"source-b", "Second"}} {
		if _, err := manager.Pull(t.Context(), artifact.WorkbookPull{
			Workspace: workspace,
			Filename:  item.name + ".twb",
			Content:   []byte("<workbook/>"),
			Metadata: artifact.WorkbookMetadata{
				Kind: "workbook", Name: item.name, TableauID: item.id,
				SourceServerOrigin: server.URL, SourceSiteLUID: "site-1",
				SourceEnvironment: "production", SourceSite: "team-site",
				SourceProjectID: "project-1", SourceProjectName: "Analytics",
			},
		}); err != nil {
			t.Fatal(err)
		}
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	done := make(chan int, 1)
	options.WorkerLauncher = func(_ context.Context, directory, id string) error {
		go func() { done <- runPublicationWorker(context.Background(), directory, id, options) }()
		return nil
	}

	args := []string{"content", "workbook", "publish", "--workspace", "analytics", "--environment", "production", "--project-id", "project-1", "--id", "source-a", "--id", "source-b", "--overwrite", "--no-wait", "--json"}
	var output strings.Builder
	if exit := Run(t.Context(), args, &output, options); exit != 0 {
		t.Fatalf("batch no-wait exit=%d output=%s", exit, output.String())
	}
	var receipt struct {
		ID string `json:"operation_id"`
	}
	if err := json.Unmarshal([]byte(output.String()), &receipt); err != nil || receipt.ID == "" {
		t.Fatalf("batch receipt=%s err=%v", output.String(), err)
	}
	select {
	case code := <-done:
		if code == 0 {
			t.Fatal("mixed failure batch unexpectedly succeeded")
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mixed failure batch worker did not finish")
	}
	if writes.Load() != 2 || reads.Load() != 0 {
		t.Fatalf("writes=%d job reads=%d, want two submissions and no no-wait polling", writes.Load(), reads.Load())
	}
	record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(receipt.ID)
	if err != nil {
		t.Fatal(err)
	}
	if record.Phase != operationrun.PhaseRemotePending || record.ExitCode == nil || *record.ExitCode == 0 {
		t.Fatalf("mixed batch record=%+v", record)
	}
	var aggregate map[string]any
	if err := json.Unmarshal(record.FullResult, &aggregate); err != nil {
		t.Fatalf("full result is not JSON: %v\n%s", err, record.FullResult)
	}
	if aggregate["status"] != "partial_failure" || aggregate["failed"] != float64(1) || aggregate["pending"] != float64(1) {
		t.Fatalf("aggregate=%#v, want one mixed operation", aggregate)
	}
	if strings.Contains(string(record.FullResult), "operation_id") {
		t.Fatalf("per-item aggregate invented another operation identity: %s", record.FullResult)
	}
	if len(operationJSONFiles(t, options.OperationDirectory)) != 1 {
		t.Fatalf("operation files = %v, want one operation record", operationJSONFiles(t, options.OperationDirectory))
	}
}

func TestPublicationWorkerCutoffStopsForegroundWaitWithoutCancellingHeldSubmission(t *testing.T) {
	submissionStarted := make(chan struct{})
	releaseSubmission := make(chan struct{})
	var writes, reads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/users/user-1"):
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><workbooks/></tsResponse>`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			writes.Add(1)
			close(submissionStarted)
			<-releaseSubmission
			w.WriteHeader(http.StatusAccepted)
			_, _ = io.WriteString(w, `<tsResponse><job id="job-cutoff" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
		case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/jobs/"):
			reads.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><job id="job-cutoff" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`)
		default:
			t.Errorf("unexpected Tableau request: %s %s?%s", r.Method, r.URL.Path, r.URL.RawQuery)
			http.Error(w, "unexpected request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Cutoff.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	done := make(chan int, 1)
	options.WorkerLauncher = func(_ context.Context, directory, id string) error {
		store := operationrun.Store{Directory: directory}
		if _, err := store.Update(id, func(record *operationrun.Record) error {
			record.RequestedAt = time.Now().UTC().Add(-publicationWaitLimit - time.Second)
			return nil
		}); err != nil {
			return err
		}
		go func() { done <- runPublicationWorker(context.Background(), directory, id, options) }()
		return nil
	}

	type runResult struct {
		code   int
		output string
	}
	results := make(chan runResult, 1)
	go func() {
		var output strings.Builder
		results <- runResult{code: Run(t.Context(), []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &output, options), output: output.String()}
	}()
	select {
	case <-submissionStarted:
	case <-time.After(5 * time.Second):
		t.Fatal("worker did not reach the held submission")
	}
	var result runResult
	select {
	case result = <-results:
	case <-time.After(5 * time.Second):
		t.Fatal("foreground wait did not stop at the shared cutoff")
	}
	if result.code != 0 {
		t.Fatalf("cutoff exit=%d output=%s", result.code, result.output)
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(result.output), &receipt); err != nil {
		t.Fatalf("cutoff output is not JSON: %v\n%s", err, result.output)
	}
	operationID, _ := receipt["operation_id"].(string)
	if operationID == "" || receipt["waiting_stopped"] != true || !strings.Contains(fmt.Sprint(receipt["check_status"]), "job inspect --operation-id "+operationID) {
		t.Fatalf("cutoff receipt=%#v", receipt)
	}
	close(releaseSubmission)
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("cutoff worker exit=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("cutoff worker did not finish after submission release")
	}
	if writes.Load() != 1 || reads.Load() != 0 {
		t.Fatalf("writes=%d job reads=%d, want one held submission and no remote polls", writes.Load(), reads.Load())
	}
	record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
	if err != nil || record.Phase != operationrun.PhaseRemotePending || record.FinishedAt.IsZero() {
		t.Fatalf("cutoff record=%+v err=%v", record, err)
	}
}

func publicationWorkerTestOptions(configPath string, server *httptest.Server, t *testing.T) Options {
	t.Helper()
	return Options{
		ConfigPath:         configPath,
		HTTPClient:         server.Client(),
		MutationsEnabled:   true,
		PublicationWorkers: true,
		OperationDirectory: t.TempDir(),
		JobDirectory:       t.TempDir(),
		Stderr:             io.Discard,
	}
}

func operationJSONFiles(t *testing.T, directory string) []string {
	t.Helper()
	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	var files []string
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".json" && strings.Contains(entry.Name(), "-") {
			files = append(files, fmt.Sprintf("%s/%s", directory, entry.Name()))
		}
	}
	return files
}
