package app

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestPublicationProcessDeathRetainsAcceptedRecoveryAtLinkageBoundary(t *testing.T) {
	for _, beforeLink := range []bool{true, false} {
		t.Run(fmt.Sprintf("before-link=%t", beforeLink), func(t *testing.T) {
			ctx, cancel := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancel()
			fixture := &publicationStatusFixture{jobDestination: true}
			submitted, observeStarted := make(chan struct{}), make(chan struct{})
			releaseSubmission, releaseObservation := make(chan struct{}), make(chan struct{})
			finishSubmission := sync.OnceFunc(func() { close(releaseSubmission) })
			finishObservation := sync.OnceFunc(func() { close(releaseObservation) })
			var posts, observations atomic.Int32
			fixture.server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks") {
					if posts.Add(1) == 1 {
						close(submitted)
					}
					select {
					case <-releaseSubmission:
					case <-r.Context().Done():
						return
					}
					w.Header().Set("X-Tableau-Request-Id", "boundary-acceptance-request")
				}
				if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-recovery") && observations.Add(1) == 1 && !beforeLink {
					close(observeStarted)
					select {
					case <-releaseObservation:
					case <-r.Context().Done():
						return
					}
				}
				fixture.handle(w, r)
			}))
			t.Cleanup(func() {
				finishSubmission()
				finishObservation()
				fixture.server.Close()
			})
			runtime, _ := datasourceLifecycleRuntime(t, fixture.server)
			file := filepath.Join(t.TempDir(), "Sales.twb")
			if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
				t.Fatal(err)
			}
			options := publicationWorkerTestOptions(runtime.configPath, fixture.server, t)
			started := make(chan *publicationBoundaryProcess, 1)
			options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
				worker, err := startPublicationBoundaryProcess(directory, id, options.JobDirectory, fixture.server.Certificate().Raw)
				if err != nil {
					return err
				}
				t.Cleanup(func() { worker.stop(t) })
				started <- worker
				return awaitPublicationWorkerTestStart(ctx, directory, id, worker.exited)
			}
			args := []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}
			if beforeLink {
				args = append(args, "--no-wait")
			}
			type result struct {
				exit int
				text string
			}
			published := make(chan result, 1)
			go func() {
				var output strings.Builder
				exit := Run(ctx, args, &output, options)
				published <- result{exit, output.String()}
			}()
			var worker *publicationBoundaryProcess
			select {
			case worker = <-started:
			case early := <-published:
				t.Fatalf("publication ended before starting child: %+v", early)
			case <-ctx.Done():
				t.Fatal("publication child did not start")
			}
			publicationBoundarySignal(t, ctx, submitted, "native submission")
			store := operationrun.Store{Directory: options.OperationDirectory}
			var finishLink func()
			if beforeLink {
				select {
				case response := <-published:
					if response.exit != 0 || operationIDFromOutput(t, response.text) != worker.id {
						t.Fatalf("no-wait response=%+v", response)
					}
				case <-ctx.Done():
					t.Fatal("no-wait invocation did not return")
				}
				finishLink = holdPublicationBoundaryRecord(t, ctx, store, worker.id)
			}
			finishSubmission()
			receiptStore := jobmonitor.Store{Directory: options.JobDirectory}
			receipt, receiptPath := waitPublicationBoundaryReceipt(t, ctx, receiptStore)
			if receipt.Observation.RequestID != "boundary-acceptance-request" || receipt.Observation.Status != "pending" {
				t.Fatalf("durable acceptance=%+v", receipt)
			}
			if !beforeLink {
				publicationBoundarySignal(t, ctx, observeStarted, "first job observation")
				linked, err := store.ReadContext(ctx, worker.id)
				if err != nil || len(linked.ReceiptPaths) != 1 || linked.ReceiptPaths[0] != receiptPath {
					t.Fatalf("receipt was not linked before observation: %+v err=%v", linked, err)
				}
			}
			wantObservations := int32(0)
			if !beforeLink {
				wantObservations = 1
			}
			if observations.Load() != wantObservations {
				t.Fatalf("observations before process death=%d, want %d", observations.Load(), wantObservations)
			}
			worker.stop(t)
			if finishLink != nil {
				finishLink()
			}
			finishObservation()
			if !beforeLink {
				select {
				case response := <-published:
					if response.exit == 0 {
						t.Fatalf("interrupted foreground publication reported success: %s", response.text)
					}
				case <-ctx.Done():
					t.Fatal("foreground invocation did not observe child death")
				}
			}
			alive, err := store.Alive(worker.id)
			if err != nil || alive {
				t.Fatalf("terminated child still owns operation lease: alive=%t err=%v", alive, err)
			}
			retained, err := receiptStore.ReadPath(receiptPath)
			if err != nil || retained.Observation.ID != receipt.Observation.ID || retained.Observation.RequestID != receipt.Observation.RequestID || retained.Observation.Status != "pending" {
				t.Fatalf("process death lost durable accepted receipt: %+v err=%v", retained, err)
			}
			if lease, _, err := store.Lease(worker.id, os.Getpid()); !errors.Is(err, operationrun.ErrWorkerInterrupted) {
				if lease != nil {
					_ = lease.Release()
				}
				t.Fatalf("interrupted publication became replayable: %v", err)
			}
			var inspectionOutput strings.Builder
			if exit := Run(ctx, []string{"job", "inspect", "--operation-id", worker.id, "--json", "--full"}, &inspectionOutput, options); exit != 0 {
				t.Fatalf("operation inspection exit=%d output=%s", exit, inspectionOutput.String())
			}
			var inspection map[string]any
			if err := json.Unmarshal([]byte(inspectionOutput.String()), &inspection); err != nil {
				t.Fatal(err)
			}
			if inspection["status"] != "interrupted" {
				t.Fatalf("child death lost interrupted status: %#v", inspection)
			}
			var exactJob strings.Builder
			if exit := Run(ctx, []string{"job", "inspect", "--environment", "production", "--id", "job-recovery", "--json"}, &exactJob, options); exit != 0 || !json.Valid([]byte(exactJob.String())) || !strings.Contains(exactJob.String(), "job-recovery") || !strings.Contains(exactJob.String(), "succeeded") {
				t.Fatalf("exact accepted job is not recoverable: exit=%d output=%s", exit, exactJob.String())
			}
			if posts.Load() != 1 || fixture.publishes.Load() != 1 {
				t.Fatalf("recovery resubmitted publication: requests=%d accepted=%d", posts.Load(), fixture.publishes.Load())
			}
			jobs, ok := inspection["accepted_jobs"].([]any)
			if !ok || len(jobs) != 1 {
				t.Fatalf("operation recovery omitted durable accepted receipt at linkage boundary: %#v", inspection)
			}
			job, ok := jobs[0].(map[string]any)
			if !ok || job["tableau_job_id"] != "job-recovery" || job["status"] != "succeeded" || job["workbook_luid"] != "wb-new" {
				t.Fatalf("operation recovery lost accepted outcome: %#v", jobs)
			}
		})
	}
}

func publicationBoundarySignal(t *testing.T, ctx context.Context, ready <-chan struct{}, phase string) {
	t.Helper()
	select {
	case <-ready:
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %s", phase)
	}
}

func holdPublicationBoundaryRecord(t *testing.T, ctx context.Context, store operationrun.Store, id string) func() {
	t.Helper()
	held, release, done := make(chan struct{}), make(chan struct{}), make(chan struct{})
	finish := sync.OnceFunc(func() { close(release) })
	t.Cleanup(finish)
	go func() {
		defer close(done)
		_, _ = store.Update(id, func(record *operationrun.Record) error {
			if len(record.ReceiptPaths) != 0 {
				t.Error("receipt already linked before native acceptance")
			}
			close(held)
			select {
			case <-release:
			case <-ctx.Done():
			}
			return errors.New("test releases record unchanged")
		})
	}()
	publicationBoundarySignal(t, ctx, held, "operation update lock")
	return func() {
		finish()
		publicationBoundarySignal(t, ctx, done, "operation update release")
	}
}

func waitPublicationBoundaryReceipt(t *testing.T, ctx context.Context, store jobmonitor.Store) (jobmonitor.Receipt, string) {
	t.Helper()
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		receipt, path, err := store.FindByJobID(ctx, "job-recovery", "production", "team-site")
		if err == nil {
			return receipt, path
		}
		select {
		case <-ticker.C:
		case <-ctx.Done():
			t.Fatalf("accepted receipt did not become durable: %v", err)
		}
	}
}

type publicationBoundaryProcess struct {
	id      string
	command *exec.Cmd
	done    chan struct{}
	exited  chan publicationWorkerTestExit
	output  *boundedPublicationWorkerTestOutput
}

func startPublicationBoundaryProcess(directory, id, jobDirectory string, certificate []byte) (*publicationBoundaryProcess, error) {
	executable, err := os.Executable()
	if err != nil {
		return nil, err
	}
	child := exec.Command(executable, "-test.run=^TestPublicationWorkerProcessHelper$", "--", directory, id, jobDirectory)
	child.Env = append(os.Environ(), "TADX_TEST_WORKER_CERT="+base64.StdEncoding.EncodeToString(certificate))
	worker := &publicationBoundaryProcess{id: id, command: child, done: make(chan struct{}), exited: make(chan publicationWorkerTestExit, 1), output: new(boundedPublicationWorkerTestOutput)}
	child.Stdout, child.Stderr = worker.output, worker.output
	if err := child.Start(); err != nil {
		return nil, err
	}
	go func() {
		err := child.Wait()
		worker.exited <- publicationWorkerTestExit{code: child.ProcessState.ExitCode(), waitErr: err, output: worker.output.String()}
		close(worker.done)
	}()
	return worker, nil
}

func (w *publicationBoundaryProcess) stop(t *testing.T) {
	t.Helper()
	select {
	case <-w.done:
		return
	default:
	}
	if err := w.command.Process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		t.Errorf("stop exact publication child: %v", err)
	}
	select {
	case <-w.done:
	case <-time.After(5 * time.Second):
		t.Errorf("publication child did not exit: %s", w.output.String())
	}
}
