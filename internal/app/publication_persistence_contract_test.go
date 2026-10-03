package app

import (
	"context"
	"encoding/json"
	"errors"
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

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestAsyncPublicationReceiptPersistenceFailurePreservesAcceptedIdentity(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource"} {
		for _, full := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/full=%t", kind, full), func(t *testing.T) {
				fixture := newPublicationPersistenceFixture(t, kind)
				runtime, _ := datasourceLifecycleRuntime(t, fixture.server)
				file := publicationPersistenceNativeFile(t, kind)
				jobDirectory := filepath.Join(t.TempDir(), "blocked-receipts")
				if err := os.WriteFile(jobDirectory, []byte("receipt storage is not a directory"), 0o600); err != nil {
					t.Fatal(err)
				}
				options := withSiteMutationConsent(t, Options{
					ConfigPath: runtime.configPath, HTTPClient: fixture.server.Client(),
					JobDirectory: jobDirectory, OperationDirectory: t.TempDir(), Stderr: io.Discard,
				}, true)
				args := publicationPersistenceArgs(kind, file)
				if full {
					args = append(args, "--full")
				}
				var output strings.Builder
				exit := Run(t.Context(), args, &output, options)
				if exit == 0 || fixture.publishes.Load() != 1 || fixture.jobGets.Load() != 0 {
					t.Fatalf("exit=%d submissions=%d observations=%d output=%s", exit, fixture.publishes.Load(), fixture.jobGets.Load(), output.String())
				}
				var result struct {
					Output struct {
						Result struct {
							Status    string `json:"status"`
							JobID     string `json:"tableau_job_id"`
							RequestID string `json:"tableau_request_id"`
						} `json:"result"`
					} `json:"output"`
					Error struct {
						Phase            string `json:"phase"`
						Outcome          string `json:"outcome"`
						JobID            string `json:"tableau_job_id"`
						RequestID        string `json:"tableau_request_id"`
						Retryable        *bool  `json:"retryable"`
						CorrectiveAction string `json:"corrective_action"`
					} `json:"error"`
				}
				if err := json.Unmarshal([]byte(output.String()), &result); err != nil {
					t.Fatalf("publication output is not JSON: %v\n%s", err, output.String())
				}
				if result.Output.Result.JobID != fixture.jobID() || result.Error.JobID != fixture.jobID() {
					t.Errorf("accepted job identity lost: %s", output.String())
				}
				if result.Error.RequestID != fixture.requestID() {
					t.Errorf("accepted request identity lost from the error contract: %s", output.String())
				}
				if full && result.Output.Result.RequestID != fixture.requestID() {
					t.Errorf("accepted request identity lost from full result: %s", output.String())
				}
				if result.Output.Result.Status != "pending" || result.Error.Outcome != "unknown" || result.Error.Phase != "persistence" {
					t.Errorf("receipt failure must preserve pending remote outcome and identify persistence phase: %s", output.String())
				}
				if result.Error.Retryable == nil || *result.Error.Retryable || !strings.Contains(result.Error.CorrectiveAction, "Do not repeat publication") {
					t.Errorf("receipt failure must retain non-retryable recovery guidance: %s", output.String())
				}
			})
		}
	}
}

func TestPublicationAcceptedReceiptPersistenceFailureBeforeObservation(t *testing.T) {
	for _, noWait := range []bool{false, true} {
		t.Run(fmt.Sprintf("no-wait=%t", noWait), func(t *testing.T) {
			directory := filepath.Join(t.TempDir(), "blocked-receipts")
			if err := os.WriteFile(directory, []byte("receipt storage is not a directory"), 0o600); err != nil {
				t.Fatal(err)
			}
			publication := &publication{
				runtime: &runtimeDependencies{publicationExecution: &publicationExecution{noWait: noWait}},
				store:   jobmonitor.Store{Directory: directory},
				base: jobmonitor.Receipt{
					Version: 1, Operation: "workbook.publish", Environment: "production",
					Server: "https://example.test", Site: "team-site", SiteID: "site-1",
					CoordinationKey: "fixture-coordination",
				},
			}
			receipt, _, err := publication.accepted(t.Context(), "accepted-job", "accepted-request", "PublishWorkbook")
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.Phase != errs.PhasePersistence || failure.Outcome != errs.OutcomeUnknown || failure.TableauJobID != "accepted-job" || failure.TableauRequestID != "accepted-request" {
				t.Fatalf("acceptance persistence error=%#v", err)
			}
			storageFailure, ok := errors.AsType[*os.PathError](failure.Cause)
			if !ok || storageFailure.Op != "mkdir" || storageFailure.Path != directory {
				t.Fatalf("underlying error=%#v, want blocked receipt directory failure", failure.Cause)
			}
			if receipt.Observation.ID != "accepted-job" || receipt.Observation.RequestID != "accepted-request" || receipt.Observation.Status != "pending" || receipt.ManualOnly != noWait {
				t.Fatalf("acceptance receipt=%+v", receipt)
			}
		})
	}
}

func TestDatasourcePublicationDelayedDestinationRecoveryThroughCLI(t *testing.T) {
	fixture := newPublicationPersistenceFixture(t, "datasource")
	runtime, _ := datasourceLifecycleRuntime(t, fixture.server)
	options := publicationWorkerTestOptions(runtime.configPath, fixture.server, t)
	done := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchInProcessPublicationWorkerTest(ctx, directory, id, options, done)
	}
	args := append(publicationPersistenceArgs("datasource", publicationPersistenceNativeFile(t, "datasource")), "--no-wait")
	var output strings.Builder
	if exit := Run(t.Context(), args, &output, options); exit != 0 {
		t.Fatalf("no-wait publication exit=%d output=%s", exit, output.String())
	}
	operationID := operationIDFromOutput(t, output.String())
	select {
	case code := <-done:
		if code != 0 {
			t.Fatalf("publication worker exit=%d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("publication worker did not finish")
	}
	record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
	if err != nil || record.Phase != operationrun.PhaseRemotePending {
		t.Fatalf("operation=%+v err=%v, want durable pending acceptance", record, err)
	}
	if fixture.publishes.Load() != 1 || fixture.jobGets.Load() != 0 {
		t.Fatalf("no-wait submissions=%d observations=%d, want 1 and 0", fixture.publishes.Load(), fixture.jobGets.Load())
	}

	for range 2 {
		result := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationPersistenceDatasource(t, result, fixture.jobID(), "destination_pending", "")
		if fixture.publishes.Load() != 1 || fixture.jobGets.Load() != 1 {
			t.Fatalf("pending recovery submissions=%d observations=%d, want no resubmission or terminal-job repoll", fixture.publishes.Load(), fixture.jobGets.Load())
		}
	}
	fixture.indexVisible.Store(true)
	for range 2 {
		result := inspectPublicationOperationThroughCLI(t, operationID, options)
		assertPublicationPersistenceDatasource(t, result, fixture.jobID(), "confirmed", "datasource-created")
		if fixture.publishes.Load() != 1 || fixture.jobGets.Load() != 1 {
			t.Fatalf("confirmed recovery submissions=%d observations=%d, want unchanged accepted job", fixture.publishes.Load(), fixture.jobGets.Load())
		}
	}
}

func assertPublicationPersistenceDatasource(t *testing.T, result map[string]any, jobID, verification, id string) {
	t.Helper()
	items := publicationStatusItems(t, result)
	if result["status"] != "succeeded" || len(items) != 1 {
		t.Fatalf("recovery must preserve successful job: %#v", result)
	}
	item := items[0]
	if item["status"] != "succeeded" || item["verification"] != verification || item["tableau_job_id"] != jobID {
		t.Fatalf("recovery item=%#v, want succeeded %s for %s", item, verification, jobID)
	}
	if id == "" {
		if _, present := item["datasource_luid"]; present {
			t.Fatalf("index delay fabricated destination identity: %#v", item)
		}
	} else if item["datasource_luid"] != id {
		t.Fatalf("recovery identity=%#v, want %s", item, id)
	}
}

type publicationPersistenceFixture struct {
	server       *httptest.Server
	kind         string
	publishes    atomic.Int32
	jobGets      atomic.Int32
	indexVisible atomic.Bool
}

func (f *publicationPersistenceFixture) jobID() string     { return f.kind + "-accepted-job" }
func (f *publicationPersistenceFixture) requestID() string { return f.kind + "-acceptance-request" }

func newPublicationPersistenceFixture(t *testing.T, kind string) *publicationPersistenceFixture {
	t.Helper()
	f := &publicationPersistenceFixture{kind: kind}
	jobType := map[string]string{"workbook": "PublishWorkbook", "datasource": "PublishDatasource"}[kind]
	mux := http.NewServeMux()
	mux.HandleFunc("POST /api/{version}/auth/signin", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
	})
	mux.HandleFunc("GET /api/{version}/sites/site-1/users/user-1", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
	})
	mux.HandleFunc("GET /api/{version}/sites/site-1/projects", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
	})
	mux.HandleFunc("POST /api/{version}/sites/site-1/workbooks/validateWorkbook", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
	})
	mux.HandleFunc("GET /api/{version}/sites/site-1/"+kind+"s", func(w http.ResponseWriter, _ *http.Request) {
		if f.indexVisible.Load() {
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><%ss><%s id="%s-created" name="Native"><project id="project-1" name="Analytics"/></%s></%ss></tsResponse>`, kind, kind, kind, kind, kind)
		} else {
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><%ss/></tsResponse>`, kind)
		}
	})
	mux.HandleFunc("POST /api/{version}/sites/site-1/"+kind+"s", func(w http.ResponseWriter, r *http.Request) {
		f.publishes.Add(1)
		if r.URL.Query().Get("asJob") != "true" {
			t.Error("publication must request asynchronous acceptance")
		}
		w.Header().Set("X-Tableau-Request-Id", f.requestID())
		w.WriteHeader(http.StatusAccepted)
		_, _ = fmt.Fprintf(w, `<tsResponse><job id="%s" type="%s" progress="0" finishCode="1"/></tsResponse>`, f.jobID(), jobType)
	})
	mux.HandleFunc("GET /api/{version}/sites/site-1/jobs/"+f.jobID(), func(w http.ResponseWriter, _ *http.Request) {
		f.jobGets.Add(1)
		_, _ = fmt.Fprintf(w, `<tsResponse><job id="%s" type="%s" progress="100" finishCode="0"/></tsResponse>`, f.jobID(), jobType)
	})
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected fixture request: %s %s", r.Method, r.URL.Path)
		http.Error(w, "unexpected request", http.StatusNotFound)
	})
	f.server = httptest.NewTLSServer(mux)
	t.Cleanup(f.server.Close)
	return f
}

func publicationPersistenceNativeFile(t *testing.T, kind string) string {
	t.Helper()
	ext := map[string]string{"workbook": ".twb", "datasource": ".tds"}[kind]
	path := filepath.Join(t.TempDir(), "Native"+ext)
	if err := os.WriteFile(path, []byte("<"+kind+"/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func publicationPersistenceArgs(kind, file string) []string {
	args := []string{"content", kind, "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}
	if kind == "datasource" {
		args = append(args, "--create")
	}
	return args
}
