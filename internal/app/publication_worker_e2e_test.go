package app

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestNoWaitPublicationReturnsDuringSubmissionAndNeverPolls(t *testing.T) {
	for _, scenario := range []string{"workbook", "datasource", "flow", "workbook-process"} {
		kind := strings.TrimSuffix(scenario, "-process")
		t.Run(scenario, func(t *testing.T) {
			release := make(chan struct{})
			var reads, writes atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/users/user-1"):
					io.WriteString(w, `<tsResponse><user id="user-1" name="publisher" siteRole="SiteAdministratorCreator"/></tsResponse>`)
				case strings.HasSuffix(r.URL.Path, "/projects"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
				case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
					io.WriteString(w, `{"errors":[],"warnings":[]}`)
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/jobs/"):
					reads.Add(1)
					fmt.Fprintf(w, `<tsResponse><job id="job-1" type="Publish%s" progress="100" finishCode="0"><%s id="created-1"/></job></tsResponse>`, map[string]string{"workbook": "Workbook", "datasource": "Datasource"}[kind], kind)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
					writes.Add(1)
					<-release
					if kind == "flow" {
						w.WriteHeader(http.StatusCreated)
						fmt.Fprint(w, `<tsResponse><flow id="created-1" name="Native"><project id="project-1"/></flow></tsResponse>`)
						return
					}
					if r.URL.Query().Get("asJob") != "true" {
						t.Error("server job not requested")
					}
					w.WriteHeader(http.StatusAccepted)
					fmt.Fprintf(w, `<tsResponse><job id="job-1" type="Publish%s" progress="0" finishCode="1"/></tsResponse>`, map[string]string{"workbook": "Workbook", "datasource": "Datasource"}[kind])
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><%ss/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"), kind)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/created-1"):
					fmt.Fprintf(w, `<tsResponse><%s id="created-1" name="Native"><project id="project-1" name="Analytics"/></%s></tsResponse>`, kind, kind)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					http.Error(w, "unexpected", 400)
				}
			}))
			defer server.Close()
			runtime, _ := datasourceLifecycleRuntime(t, server)
			ext, content := ".twb", "<workbook/>"
			if kind == "datasource" {
				ext, content = ".tds", "<datasource/>"
			}
			if kind == "flow" {
				ext, content = ".tfl", `{"nodes":{}}`
			}
			file := filepath.Join(t.TempDir(), "Native"+ext)
			if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
				t.Fatal(err)
			}
			options := withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), PublicationWorkers: true, OperationDirectory: t.TempDir(), JobDirectory: t.TempDir(), Stderr: io.Discard}, true)
			done := make(chan int, 1)
			options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
				if strings.HasSuffix(scenario, "-process") {
					executable, err := os.Executable()
					if err != nil {
						return err
					}
					child := exec.Command(executable, "-test.run=^TestPublicationWorkerProcessHelper$", "--", directory, id, options.JobDirectory)
					child.Env = append(os.Environ(), "TADX_TEST_WORKER_CERT="+base64.StdEncoding.EncodeToString(server.Certificate().Raw))
					if err := child.Start(); err != nil {
						return err
					}
					exited := make(chan int, 1)
					go func() {
						code := 0
						if child.Wait() != nil {
							code = 1
						}
						exited <- code
						done <- code
					}()
					return awaitPublicationWorkerTestStart(ctx, directory, id, exited)
				}
				go func() { done <- runPublicationWorker(context.Background(), directory, id, options) }()
				return nil
			}
			args := []string{"content", kind, "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--no-wait", "--json"}
			if kind == "datasource" {
				args = append(args, "--create")
			}
			var output strings.Builder
			// The process launcher separates test-binary initialization from the
			// app handshake. Holding the write proves detachment after that handshake.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			exit := Run(ctx, args, &output, options)
			close(release)
			if exit != 0 {
				t.Fatalf("exit=%d output=%s", exit, output.String())
			}
			var receipt struct {
				ID    string `json:"operation_id"`
				Check string `json:"check_status"`
			}
			if err := json.Unmarshal([]byte(output.String()), &receipt); err != nil || receipt.ID == "" || !strings.Contains(receipt.Check, "job inspect --operation-id "+receipt.ID) {
				t.Fatalf("receipt=%s err=%v", output.String(), err)
			}
			select {
			case code := <-done:
				if code != 0 {
					record, _ := (operationrun.Store{Directory: options.OperationDirectory}).Read(receipt.ID)
					t.Fatalf("worker exit=%d output=%s", code, record.FullResult)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not finish submission")
			}
			if reads.Load() != 0 || writes.Load() != 1 {
				t.Fatalf("reads=%d writes=%d", reads.Load(), writes.Load())
			}
			record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(receipt.ID)
			if err != nil || record.FinishedAt.IsZero() {
				t.Fatalf("record=%+v err=%v", record, err)
			}
			if kind != "flow" && (record.Phase != operationrun.PhaseRemotePending || !strings.Contains(string(record.FullResult), "job-1")) {
				t.Fatalf("accepted job lost: %+v", record)
			}
		})
	}
}

const publicationWorkerTestStartLimit = 15 * time.Second

func awaitPublicationWorkerTestStart(ctx context.Context, directory, id string, exited <-chan int) error {
	ctx, cancel := context.WithTimeout(ctx, publicationWorkerTestStartLimit)
	defer cancel()
	store := operationrun.Store{Directory: directory}
	return awaitPublicationWorkerTestStartState(ctx, exited, func(ctx context.Context) (operationrun.Record, error) {
		return store.ReadContext(ctx, id)
	})
}

func awaitPublicationWorkerTestStartState(ctx context.Context, exited <-chan int, read func(context.Context) (operationrun.Record, error)) error {
	ticker := time.NewTicker(10 * time.Millisecond)
	defer ticker.Stop()
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		record, err := read(ctx)
		if err != nil {
			return err
		}
		if !record.StartedAt.IsZero() {
			return nil
		}
		select {
		case code := <-exited:
			record, err := read(ctx)
			if err == nil && !record.StartedAt.IsZero() {
				return nil
			}
			return fmt.Errorf("publication worker test process exited before durable startup acknowledgement: exit %d", code)
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
		}
	}
}

func TestAwaitPublicationWorkerTestStartState(t *testing.T) {
	t.Run("waits for durable acknowledgement", func(t *testing.T) {
		firstRead := make(chan struct{})
		releaseStart := make(chan struct{})
		var reads atomic.Int32
		read := func(ctx context.Context) (operationrun.Record, error) {
			if reads.Add(1) == 1 {
				close(firstRead)
				return operationrun.Record{}, nil
			}
			select {
			case <-releaseStart:
				return operationrun.Record{StartedAt: time.Now()}, nil
			case <-ctx.Done():
				return operationrun.Record{}, ctx.Err()
			}
		}
		result := make(chan error, 1)
		go func() {
			result <- awaitPublicationWorkerTestStartState(t.Context(), make(chan int), read)
		}()
		<-firstRead
		select {
		case err := <-result:
			t.Fatalf("wait returned before durable acknowledgement: %v", err)
		default:
		}
		close(releaseStart)
		if err := <-result; err != nil {
			t.Fatal(err)
		}
	})

	t.Run("reports exit before acknowledgement", func(t *testing.T) {
		exited := make(chan int, 1)
		exited <- 7
		err := awaitPublicationWorkerTestStartState(t.Context(), exited, func(context.Context) (operationrun.Record, error) {
			return operationrun.Record{}, nil
		})
		if err == nil || !strings.Contains(err.Error(), "exit 7") {
			t.Fatalf("error = %v, want premature exit", err)
		}
	})

	t.Run("accepts acknowledgement concurrent with exit", func(t *testing.T) {
		exited := make(chan int, 1)
		exited <- 0
		var reads atomic.Int32
		err := awaitPublicationWorkerTestStartState(t.Context(), exited, func(context.Context) (operationrun.Record, error) {
			if reads.Add(1) == 1 {
				return operationrun.Record{}, nil
			}
			return operationrun.Record{StartedAt: time.Now()}, nil
		})
		if err != nil {
			t.Fatalf("concurrent durable acknowledgement rejected: %v", err)
		}
	})

	t.Run("honors cancellation", func(t *testing.T) {
		ctx, cancel := context.WithCancel(t.Context())
		cancel()
		err := awaitPublicationWorkerTestStartState(ctx, make(chan int), func(context.Context) (operationrun.Record, error) {
			return operationrun.Record{}, nil
		})
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("error = %v, want context cancellation", err)
		}
	})
}

func TestPublicationWorkerProcessHelper(t *testing.T) {
	certificate := os.Getenv("TADX_TEST_WORKER_CERT")
	if certificate == "" {
		return
	}
	der, err := base64.StdEncoding.DecodeString(certificate)
	if err != nil {
		t.Fatal(err)
	}
	cert, err := x509.ParseCertificate(der)
	if err != nil {
		t.Fatal(err)
	}
	roots := x509.NewCertPool()
	roots.AddCert(cert)
	client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{RootCAs: roots, MinVersion: tls.VersionTLS12}}}
	args := os.Args[len(os.Args)-3:]
	code := Run(context.Background(), []string{"__publication-worker", args[0], args[1]}, io.Discard, Options{HTTPClient: client, JobDirectory: args[2], Stderr: io.Discard})
	if code != 0 {
		t.Fatalf("worker exit=%d", code)
	}
}
