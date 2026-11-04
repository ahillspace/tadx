package app

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/artifact"
	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/operationrun"
	workspacecore "github.com/ahillspace/tadx/internal/workspace"
	"github.com/spf13/cobra"
)

func TestPublicationHandoffReleasesForegroundCredentials(t *testing.T) {
	for _, scenario := range []struct{ kind, selection, credentials string }{
		{"workbook", "batch", "environment"},
		{"datasource", "batch", "environment"},
		{"flow", "batch", "environment"},
		{"workbook", "multi-artifact", "environment"},
		{"workbook", "no-wait", "environment"},
		{"workbook", "single", "environment"},
		{"workbook", "batch", "stored-shared"},
		{"workbook", "batch", "stored-unsaved"},
	} {
		t.Run(scenario.kind+"/"+scenario.selection+"/"+scenario.credentials, func(t *testing.T) {
			// Isolate the credential cache on Windows and XDG platforms.
			t.Setenv("XDG_CACHE_HOME", t.TempDir())
			t.Setenv("LocalAppData", t.TempDir())
			var signins, writes atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					signins.Add(1)
					_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/projects"):
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber=%q pageSize=%q totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
				case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
					_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/"+scenario.kind+"s"):
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber=%q pageSize=%q totalAvailable="0"/><%ss/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"), scenario.kind)
				case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/"+scenario.kind+"s"):
					body, _ := io.ReadAll(r.Body)
					name := "Native"
					for _, candidate := range []string{"First", "Second"} {
						if strings.Contains(string(body), `name="`+candidate+`"`) {
							name = candidate
						}
					}
					writes.Add(1)
					w.WriteHeader(http.StatusCreated)
					_, _ = fmt.Fprintf(w, `<tsResponse><%s id="created-%s" name=%q><project id="project-1"/></%s></tsResponse>`, scenario.kind, name, name, scenario.kind)
				case r.Method == http.MethodGet && strings.Contains(r.URL.Path, "/created-"):
					_, name, _ := strings.Cut(r.URL.Path, "/created-")
					_, _ = fmt.Fprintf(w, `<tsResponse><%s id="created-%s" name=%q><project id="project-1" name="Analytics"/></%s></tsResponse>`, scenario.kind, name, name, scenario.kind)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			runtime, workspace := datasourceLifecycleRuntime(t, server)
			options := publicationWorkerTestOptions(runtime.configPath, server, t)
			if scenario.credentials != "environment" {
				publicationHandoffStoredPAT(t, &options, scenario.credentials == "stored-unsaved")
			}
			args := []string{"content", scenario.kind, "publish", "--environment", "production", "--project-id", "project-1", "--json"}
			if scenario.kind == "datasource" {
				args = append(args, "--create")
			}
			wantWrites := int32(1)
			if scenario.selection == "multi-artifact" {
				wantWrites = 2
				args = append(args, "--workspace", "analytics")
				for _, name := range []string{"First", "Second"} {
					pulled, err := artifact.NewWorkbookManager(nil).Pull(t.Context(), artifact.WorkbookPull{
						Workspace: workspace, Filename: name + ".twb", PayloadPath: PayloadFile(t, []byte("<workbook/>")),
						Metadata: artifact.WorkbookMetadata{Kind: "workbook", Name: name, TableauID: "source-" + name,
							SourceServerOrigin: server.URL, SourceSiteLUID: "site-1", SourceEnvironment: "production", SourceSite: "team-site",
							SourceProjectID: "project-1", SourceProjectName: "Analytics"},
					})
					if err != nil {
						t.Fatal(err)
					}
					root, err := workspacecore.CanonicalRoot(workspace)
					if err != nil {
						t.Fatal(err)
					}
					relative, err := filepath.Rel(root, pulled.ArtifactPath)
					if err != nil {
						t.Fatal(err)
					}
					args = append(args, "--artifact", filepath.ToSlash(relative))
				}
			} else {
				extension, content := ".twb", "<workbook/>"
				if scenario.kind == "datasource" {
					extension, content = ".tds", "<datasource/>"
				} else if scenario.kind == "flow" {
					extension, content = ".tfl", `{"nodes":{}}`
				}
				file := filepath.Join(t.TempDir(), "Native"+extension)
				if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
					t.Fatal(err)
				}
				if scenario.selection == "single" {
					args = append(args, "--file", file)
				} else {
					data, err := json.Marshal(map[string]any{"items": []map[string]string{{"file": file}}})
					if err != nil {
						t.Fatal(err)
					}
					batch := filepath.Join(t.TempDir(), "batch.json")
					if err := os.WriteFile(batch, data, 0o600); err != nil {
						t.Fatal(err)
					}
					args = append(args, "--batch-file", batch)
				}
			}
			if scenario.selection == "no-wait" {
				args = append(args, "--no-wait")
			}
			workerCtx, cancelWorker := context.WithTimeout(t.Context(), 30*time.Second)
			defer cancelWorker()
			done := make(chan int, 1)
			var operationID string
			options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
				operationID = id
				if scenario.selection != "single" && signins.Load() != 1 {
					t.Errorf("foreground preparation sign-ins=%d, want one", signins.Load())
				}
				exited := make(chan publicationWorkerTestExit, 1)
				go func() {
					code := runPublicationWorker(workerCtx, directory, id, options)
					exited <- publicationWorkerTestExit{code: code}
					done <- code
				}()
				return awaitPublicationWorkerTestStart(ctx, directory, id, exited)
			}
			var writesBeforeCleanup int32
			options.publicationResult = func(any, bool, int) error {
				writesBeforeCleanup = writes.Load()
				return nil
			}
			// This deadline bounds a regression; success requires actual completion
			// while the foreground runtime is still open, without detachment.
			ctx, cancel := context.WithTimeout(t.Context(), 10*time.Second)
			defer cancel()
			var output strings.Builder
			code := Run(ctx, args, &output, options)
			if operationID == "" {
				t.Fatalf("worker was not launched: exit=%d output=%s", code, output.String())
			}
			select {
			case workerExit := <-done:
				if workerExit != 0 {
					record, _ := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
					t.Fatalf("worker exit=%d result=%s", workerExit, record.FullResult)
				}
			case <-time.After(5 * time.Second):
				t.Fatal("worker did not finish")
			}
			record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(operationID)
			if err != nil || code != 0 || record.Phase != operationrun.PhaseCompleted || writes.Load() != wantWrites {
				t.Fatalf("exit=%d writes=%d record=%+v err=%v output=%s", code, writes.Load(), record, err, output.String())
			}
			if scenario.selection != "no-wait" && (writesBeforeCleanup != wantWrites || record.Detached || ctx.Err() != nil) {
				t.Fatalf("publication did not complete before foreground cleanup: writes=%d detached=%t context=%v output=%s", writesBeforeCleanup, record.Detached, ctx.Err(), output.String())
			}
			wantSignins := int32(2)
			if scenario.selection == "single" || scenario.credentials == "stored-shared" {
				wantSignins = 1
			}
			if signins.Load() != wantSignins {
				t.Fatalf("sign-ins=%d, want %d", signins.Load(), wantSignins)
			}
		})
	}
}

type publicationHandoffUnsavedStore struct{ *fakePATStore }

func (*publicationHandoffUnsavedStore) SaveSession(context.Context, coreauth.CredentialReference, coreauth.CredentialTarget, coreauth.SavedSession) error {
	return errors.New("fixture session persistence failure")
}

func publicationHandoffStoredPAT(t *testing.T, options *Options, unsaved bool) {
	t.Helper()
	t.Setenv("PROD_PAT_NAME", "")
	t.Setenv("PROD_PAT_SECRET", "")
	reference := coreauth.CredentialReference("cred_66666666666666666666666666666666")
	configuration, err := config.Load(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	environment := configuration.Environments["production"]
	environment.Auth.CredentialRef = string(reference)
	configuration.Environments["production"] = environment
	if err := config.Save(options.ConfigPath, configuration); err != nil {
		t.Fatal(err)
	}
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{
		reference: {Name: "fixture-pat", Secret: "fixture-secret", Source: coreauth.CredentialSourceOSKeyring},
	}}
	options.PATStore = store
	if unsaved {
		options.PATStore = &publicationHandoffUnsavedStore{store}
	}
}

func TestPublicationHandoffFailureStartsNothing(t *testing.T) {
	for _, scenario := range []string{"closed-sessions", "canceled"} {
		t.Run(scenario, func(t *testing.T) {
			runtime := &runtimeDependencies{configPath: filepath.Join(t.TempDir(), "config.yaml")}
			defer runtime.Close()
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			if scenario == "closed-sessions" {
				if err := runtime.commandSessions().Close(); err != nil {
					t.Fatal(err)
				}
			} else {
				cancel()
			}
			command := &cobra.Command{Annotations: map[string]string{"tadx.capability": "workbook.publish"}}
			command.SetContext(ctx)
			var launches int
			options := Options{OperationDirectory: t.TempDir(), WorkerLauncher: func(context.Context, string, string) error {
				launches++
				return errors.New("unexpected worker launch")
			}}
			err := beginPublicationWorker(command, runtime, &lastCapture{}, nil, true, options)
			failure, ok := errors.AsType[*errs.Error](err)
			if !ok || failure.ID != "operation.worker.handoff" || failure.Phase != errs.PhaseSetup || failure.Outcome != errs.OutcomeNotAttempted {
				t.Fatalf("failure=%+v, want a not-attempted handoff error", err)
			}
			if scenario == "canceled" && !errors.Is(err, context.Canceled) {
				t.Fatalf("cancellation was lost: %v", err)
			}
			if launches != 0 || len(operationJSONFiles(t, options.OperationDirectory)) != 0 {
				t.Fatalf("launches=%d records=%v, want no launch or accepted record", launches, operationJSONFiles(t, options.OperationDirectory))
			}
		})
	}
}
