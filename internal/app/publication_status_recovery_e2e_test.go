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

func TestPublicationStatusRecoveryThroughCLI(t *testing.T) {
	t.Run("workbook pending then success confirms destination once", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{jobDestination: true})
		operationID, options := startNoWaitWorkbookPublication(t, fixture)

		first := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, first, "succeeded", "confirmed", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 || fixture.publishes.Load() != 1 {
			t.Fatalf("first recovery requests: jobs=%d destinations=%d publishes=%d, want 1, 1, 1", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}

		second := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, second, "succeeded", "confirmed", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 || fixture.publishes.Load() != 1 {
			t.Fatalf("repeated recovery requests: jobs=%d destinations=%d publishes=%d, want no duplicate reads or write", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}
	})

	t.Run("empty name index stays succeeded and later resolves without republish", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{jobDestination: false})
		operationID, options := startNoWaitWorkbookPublication(t, fixture)

		pending := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, pending, "succeeded", "destination_pending", "")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 0 || fixture.publishes.Load() != 1 {
			t.Fatalf("pending recovery requests: jobs=%d destinations=%d publishes=%d, want 1, 0, 1", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}

		stillPending := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, stillPending, "succeeded", "destination_pending", "")
		if fixture.jobGets.Load() != 1 || fixture.publishes.Load() != 1 {
			t.Fatalf("repeated pending recovery republished or polled job: jobs=%d publishes=%d", fixture.jobGets.Load(), fixture.publishes.Load())
		}

		fixture.indexVisible.Store(true)
		resolved := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, resolved, "succeeded", "confirmed", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.publishes.Load() != 1 {
			t.Fatalf("later index recovery changed accepted identity: jobs=%d publishes=%d", fixture.jobGets.Load(), fixture.publishes.Load())
		}
	})

	t.Run("mismatched destination remains visible", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{jobDestination: true, mismatchedDestination: true})
		operationID, options := startNoWaitWorkbookPublication(t, fixture)

		result := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, result, "succeeded", "destination_mismatch", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 || fixture.publishes.Load() != 1 {
			t.Fatalf("mismatch recovery requests: jobs=%d destinations=%d publishes=%d, want 1, 1, 1", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}
	})

	t.Run("submission failure and accepted success preserve partial failure", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{batch: true, jobDestination: true})
		operationID, options := startNoWaitWorkbookBatchPublication(t, fixture)

		result := inspectPublicationOperationThroughCLI(t, operationID, options)
		if result["status"] != "partial_failure" {
			t.Fatalf("operation status = %#v, want partial_failure", result["status"])
		}
		items := publicationStatusItems(t, result)
		if len(items) != 2 {
			t.Fatalf("operation items = %#v, want two outcomes", items)
		}
		if items[0]["status"] != "failed" || items[1]["status"] != "succeeded" {
			t.Fatalf("mixed item statuses = %#v, want failed and succeeded", items)
		}
		acceptedResult, ok := items[1]["result"].(map[string]any)
		if !ok || acceptedResult["verification"] != "confirmed" || acceptedResult["workbook_luid"] != "wb-batch" {
			t.Fatalf("accepted item = %#v, want confirmed wb-batch", items[1])
		}
		if fixture.publishes.Load() != 2 || fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 {
			t.Fatalf("mixed recovery requests: publishes=%d jobs=%d destinations=%d, want 2, 1, 1", fixture.publishes.Load(), fixture.jobGets.Load(), fixture.destinationGets.Load())
		}
	})
}

type publicationStatusFixtureOptions struct {
	batch                 bool
	jobDestination        bool
	mismatchedDestination bool
}

type publicationStatusFixture struct {
	server                *httptest.Server
	batch                 bool
	jobDestination        bool
	mismatchedDestination bool
	indexVisible          atomic.Bool
	jobGets               atomic.Int32
	destinationGets       atomic.Int32
	publishes             atomic.Int32
}

func newPublicationStatusFixture(t *testing.T, options publicationStatusFixtureOptions) *publicationStatusFixture {
	t.Helper()
	fixture := &publicationStatusFixture{
		batch:                 options.batch,
		jobDestination:        options.jobDestination,
		mismatchedDestination: options.mismatchedDestination,
	}
	fixture.server = httptest.NewTLSServer(http.HandlerFunc(fixture.handle))
	t.Cleanup(fixture.server.Close)
	return fixture
}

func (f *publicationStatusFixture) handle(w http.ResponseWriter, r *http.Request) {
	jobID := "job-recovery"
	workbookID := "wb-new"
	workbookName := "Sales"
	projectID := "project-1"
	if f.batch {
		jobID, workbookID, workbookName = "job-batch", "wb-batch", "Second"
	}
	if f.mismatchedDestination {
		workbookName, projectID = "Actual Sales", "project-other"
	}

	switch {
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/auth/signin"):
		_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/users/user-1"):
		_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
		_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="project-1" name="Analytics"/><project id="project-other" name="Other"/></projects></tsResponse>`)
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
		_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
		if f.indexVisible.Load() {
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><workbooks><workbook id="wb-new" name="Sales"><project id="project-1" name="Analytics"/><owner id="user-1"/></workbook></workbooks></tsResponse>`)
		} else {
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><workbooks/></tsResponse>`)
		}
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
		count := f.publishes.Add(1)
		if f.batch && count == 1 {
			w.WriteHeader(http.StatusBadRequest)
			_, _ = io.WriteString(w, `<tsResponse><error code="400000"><summary>forced submission failure</summary><detail>first item rejected</detail></error></tsResponse>`)
			return
		}
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `<tsResponse><job id="%s" type="PublishWorkbook" progress="0" finishCode="1"/></tsResponse>`, jobID)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/"+jobID):
		f.jobGets.Add(1)
		if f.jobDestination {
			_, _ = fmt.Fprintf(w, `<tsResponse><job id="%s" type="PublishWorkbook" progress="100" finishCode="0"><workbook id="%s"/></job></tsResponse>`, jobID, workbookID)
		} else {
			_, _ = fmt.Fprintf(w, `<tsResponse><job id="%s" type="PublishWorkbook" progress="100" finishCode="0"/></tsResponse>`, jobID)
		}
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks/"+workbookID):
		f.destinationGets.Add(1)
		_, _ = fmt.Fprintf(w, `<tsResponse><workbook id="%s" name="%s"><project id="%s" name="Analytics"/></workbook></tsResponse>`, workbookID, workbookName, projectID)
	default:
		http.Error(w, "unexpected Tableau request", http.StatusBadRequest)
	}
}

func startNoWaitWorkbookPublication(t *testing.T, fixture *publicationStatusFixture) (string, Options) {
	t.Helper()
	runtime, _ := datasourceLifecycleRuntime(t, fixture.server)
	file := filepath.Join(t.TempDir(), "Sales.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, fixture.server, t)
	done := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchInProcessPublicationWorkerTest(ctx, directory, id, options, done)
	}
	var output strings.Builder
	args := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--no-wait", "--json"}
	if exit := Run(t.Context(), args, &output, options); exit != 0 {
		t.Fatalf("no-wait publish exit=%d output=%s", exit, output.String())
	}
	operationID := operationIDFromOutput(t, output.String())
	select {
	case code := <-done:
		if code != 0 {
			record, _ := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
			t.Fatalf("worker exit=%d full result=%s", code, record.FullResult)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("publication worker did not finish")
	}
	record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
	if err != nil || record.Phase != operationrun.PhaseRemotePending {
		t.Fatalf("finished no-wait record=%+v err=%v, want remote pending accepted job", record, err)
	}
	return operationID, options
}

func startNoWaitWorkbookBatchPublication(t *testing.T, fixture *publicationStatusFixture) (string, Options) {
	t.Helper()
	runtime, workspace := datasourceLifecycleRuntime(t, fixture.server)
	manager := artifact.NewWorkbookManager(nil)
	for _, item := range []struct{ id, name string }{{"source-a", "First"}, {"source-b", "Second"}} {
		_, err := manager.Pull(t.Context(), artifact.WorkbookPull{
			Workspace: workspace,
			Filename:  item.name + ".twb",
			Content:   []byte("<workbook/>"),
			Metadata: artifact.WorkbookMetadata{
				Kind: "workbook", Name: item.name, TableauID: item.id,
				SourceServerOrigin: fixture.server.URL, SourceSiteLUID: "site-1",
				SourceEnvironment: "production", SourceSite: "team-site",
				SourceProjectID: "project-1", SourceProjectName: "Analytics",
			},
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	options := publicationWorkerTestOptions(runtime.configPath, fixture.server, t)
	done := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchInProcessPublicationWorkerTest(ctx, directory, id, options, done)
	}
	var output strings.Builder
	args := []string{"content", "workbook", "publish", "--workspace", "analytics", "--environment", "production", "--project-id", "project-1", "--id", "source-a", "--id", "source-b", "--overwrite", "--no-wait", "--json"}
	if exit := Run(t.Context(), args, &output, options); exit != 0 {
		t.Fatalf("mixed no-wait command exit=%d output=%s", exit, output.String())
	}
	operationID := operationIDFromOutput(t, output.String())
	select {
	case code := <-done:
		if code == 0 {
			t.Fatal("mixed submission worker unexpectedly succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("mixed submission worker did not finish")
	}
	record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
	if err != nil || record.Phase != operationrun.PhaseRemotePending {
		t.Fatalf("finished mixed record=%+v err=%v, want remote pending accepted job", record, err)
	}
	return operationID, options
}

func inspectPublicationOperationThroughCLI(t *testing.T, operationID string, options Options) map[string]any {
	t.Helper()
	var output strings.Builder
	args := []string{"job", "inspect", "--operation-id", operationID, "--json"}
	if exit := Run(t.Context(), args, &output, options); exit != 0 {
		t.Fatalf("operation inspect exit=%d output=%s", exit, output.String())
	}
	var result map[string]any
	if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
		t.Fatalf("operation inspect output is not JSON: %v\n%s", err, output.String())
	}
	return result
}

func operationIDFromOutput(t *testing.T, output string) string {
	t.Helper()
	var result map[string]any
	if err := json.Unmarshal([]byte(output), &result); err != nil {
		t.Fatalf("publication output is not JSON: %v\n%s", err, output)
	}
	id, _ := result["operation_id"].(string)
	if id == "" {
		t.Fatalf("publication output omitted operation_id: %s", output)
	}
	return id
}

func assertPublicationItem(t *testing.T, result map[string]any, status, verification, resourceID string) {
	t.Helper()
	if result["status"] != status {
		t.Fatalf("operation status = %#v, want %q; result=%#v", result["status"], status, result)
	}
	items := publicationStatusItems(t, result)
	if len(items) != 1 {
		t.Fatalf("operation items = %#v, want one item", items)
	}
	item := items[0]
	if item["status"] != status || item["verification"] != verification {
		t.Fatalf("operation item = %#v, want status=%q verification=%q", item, status, verification)
	}
	if resourceID == "" {
		if _, present := item["workbook_luid"]; present {
			t.Fatalf("pending item unexpectedly has destination identity: %#v", item)
		}
	} else if item["workbook_luid"] != resourceID {
		t.Fatalf("operation item identity = %#v, want %q", item["workbook_luid"], resourceID)
	}
}

func publicationStatusItems(t *testing.T, result map[string]any) []map[string]any {
	t.Helper()
	if raw, ok := result["items"].([]any); ok {
		items := make([]map[string]any, len(raw))
		for index, value := range raw {
			item, ok := value.(map[string]any)
			if !ok {
				t.Fatalf("operation item %d has type %T: %#v", index, value, value)
			}
			items[index] = item
		}
		return items
	}
	if item, ok := result["result"].(map[string]any); ok {
		return []map[string]any{item}
	}
	t.Fatalf("operation result omitted items: %#v", result)
	return nil
}
