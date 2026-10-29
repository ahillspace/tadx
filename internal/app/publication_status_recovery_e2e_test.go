package app

import (
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

	"github.com/ahillspace/tadx/internal/jobmonitor"
)

// TestPublicationStatusRecoveryThroughCLI starts from operation records and
// job receipts written by a v1.0.0 --no-wait publish that Tableau accepted as
// a job. Publish no longer creates jobs, but operations started by v1.0.x
// must still decode and recover through job inspect --operation-id.
func TestPublicationStatusRecoveryThroughCLI(t *testing.T) {
	t.Run("workbook pending then success confirms destination once", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{jobDestination: true})
		operationID, options := loadLegacyPublication(t, fixture, "single")

		first := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, first, "succeeded", "confirmed", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 || fixture.publishes.Load() != 0 {
			t.Fatalf("first recovery requests: jobs=%d destinations=%d publishes=%d, want 1, 1, 0", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}

		second := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, second, "succeeded", "confirmed", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 || fixture.publishes.Load() != 0 {
			t.Fatalf("repeated recovery requests: jobs=%d destinations=%d publishes=%d, want no duplicate reads or write", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}
	})

	t.Run("empty name index stays succeeded and later resolves without republish", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{jobDestination: false})
		operationID, options := loadLegacyPublication(t, fixture, "single")

		pending := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, pending, "succeeded", "destination_pending", "")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 0 || fixture.publishes.Load() != 0 {
			t.Fatalf("pending recovery requests: jobs=%d destinations=%d publishes=%d, want 1, 0, 0", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}

		stillPending := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, stillPending, "succeeded", "destination_pending", "")
		if fixture.jobGets.Load() != 1 || fixture.publishes.Load() != 0 {
			t.Fatalf("repeated pending recovery republished or polled job: jobs=%d publishes=%d", fixture.jobGets.Load(), fixture.publishes.Load())
		}

		fixture.indexVisible.Store(true)
		resolved := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, resolved, "succeeded", "confirmed", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.publishes.Load() != 0 {
			t.Fatalf("later index recovery changed accepted identity: jobs=%d publishes=%d", fixture.jobGets.Load(), fixture.publishes.Load())
		}
	})

	t.Run("mismatched destination remains visible", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{jobDestination: true, mismatchedDestination: true})
		operationID, options := loadLegacyPublication(t, fixture, "single")

		result := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationItem(t, result, "succeeded", "destination_mismatch", "wb-new")
		if fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 || fixture.publishes.Load() != 0 {
			t.Fatalf("mismatch recovery requests: jobs=%d destinations=%d publishes=%d, want 1, 1, 0", fixture.jobGets.Load(), fixture.destinationGets.Load(), fixture.publishes.Load())
		}
	})

	t.Run("submission failure and accepted success preserve partial failure", func(t *testing.T) {
		fixture := newPublicationStatusFixture(t, publicationStatusFixtureOptions{batch: true, jobDestination: true})
		operationID, options := loadLegacyPublication(t, fixture, "batch")

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
		if fixture.publishes.Load() != 0 || fixture.jobGets.Load() != 1 || fixture.destinationGets.Load() != 1 {
			t.Fatalf("mixed recovery requests: publishes=%d jobs=%d destinations=%d, want 0, 1, 1", fixture.publishes.Load(), fixture.jobGets.Load(), fixture.destinationGets.Load())
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
	fixture.server = tableauFixtureServer(t, http.HandlerFunc(fixture.handle))
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
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
		_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="project-1" name="Analytics"/><project id="project-other" name="Other"/></projects></tsResponse>`)
	case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
		if f.indexVisible.Load() {
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><workbooks><workbook id="wb-new" name="Sales"><project id="project-1" name="Analytics"/><owner id="user-1"/></workbook></workbooks></tsResponse>`)
		} else {
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><workbooks/></tsResponse>`)
		}
	case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
		f.publishes.Add(1)
		http.Error(w, "recovery must not publish", http.StatusBadRequest)
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

// loadLegacyPublication installs a saved v1.0.0 operation record and its job
// receipt, bound to this test's fixture server, configuration, and directories.
func loadLegacyPublication(t *testing.T, fixture *publicationStatusFixture, name string) (string, Options) {
	t.Helper()
	runtime, _ := datasourceLifecycleRuntime(t, fixture.server)
	options := publicationWorkerTestOptions(runtime.configPath, fixture.server, t)
	source := filepath.Join(t.TempDir(), "Sales.twb")
	if err := os.WriteFile(source, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	replacer := strings.NewReplacer(
		"{{SERVER}}", jsonStringContent(t, fixture.server.URL),
		"{{CONFIG_PATH}}", jsonStringContent(t, runtime.configPath),
		"{{SOURCE_PATH}}", jsonStringContent(t, source),
		"{{JOB_DIRECTORY}}", jsonStringContent(t, options.JobDirectory),
		"{{WORKING_DIRECTORY}}", jsonStringContent(t, t.TempDir()),
	)
	files, err := filepath.Glob(filepath.Join("testdata", "legacy-publication", name, "*.json"))
	if err != nil || len(files) != 2 {
		t.Fatalf("legacy fixture files = %v, err = %v", files, err)
	}
	// Receipt file names hash the server URL, which differs per test server,
	// so receipts are renamed to the path the current store derives and the
	// operation record is rewritten to match.
	store := jobmonitor.Store{Directory: options.JobDirectory}
	renames := make([]string, 0, 2)
	runFile := ""
	for _, file := range files {
		base := filepath.Base(file)
		if strings.HasPrefix(base, "run-") {
			runFile = file
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		content := replacer.Replace(string(data))
		var receipt jobmonitor.Receipt
		if err := json.Unmarshal([]byte(content), &receipt); err != nil {
			t.Fatal(err)
		}
		path := store.Path(receipt)
		renames = append(renames, base, filepath.Base(path))
		if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	data, err := os.ReadFile(runFile)
	if err != nil {
		t.Fatal(err)
	}
	content := strings.NewReplacer(renames...).Replace(replacer.Replace(string(data)))
	operationID := strings.TrimSuffix(filepath.Base(runFile), ".json")
	if err := os.WriteFile(filepath.Join(options.OperationDirectory, filepath.Base(runFile)), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return operationID, options
}

func jsonStringContent(t *testing.T, value string) string {
	t.Helper()
	data, err := json.Marshal(value)
	if err != nil {
		t.Fatal(err)
	}
	return string(data[1 : len(data)-1])
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
