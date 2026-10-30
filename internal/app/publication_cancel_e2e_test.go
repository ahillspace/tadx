package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/operationrun"
)

func TestForegroundCancellationStopsRealWorkerBeforeSubmission(t *testing.T) {
	submitted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var writes atomic.Int32
	requestCanceled := make(chan struct{})
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.Copy(io.Discard, r.Body)
			close(submitted)
			select {
			case <-r.Context().Done():
				close(requestCanceled)
				return
			case <-release:
			}
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.Copy(io.Discard, r.Body)
			writes.Add(1)
			select {
			case <-r.Context().Done():
				close(requestCanceled)
				return
			case <-release:
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1"/></workbook></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><workbooks/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/created-diagnostic"):
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1" name="Analytics"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Native.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	workerDone := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchPublicationWorkerProcessTest(ctx, directory, id, options.JobDirectory, server.Certificate().Raw, workerDone)
	}
	parentCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type foregroundResult struct {
		code   int
		output string
	}
	foregroundDone := make(chan foregroundResult, 1)
	started := time.Now()
	go func() {
		var output strings.Builder
		code := Run(parentCtx, []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &output, options)
		foregroundDone <- foregroundResult{code, output.String()}
	}()
	select {
	case <-submitted:
	case got := <-foregroundDone:
		t.Fatalf("default returned before held submission: exit=%d output=%s", got.code, got.output)
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not reach held submission")
	}
	select {
	case got := <-foregroundDone:
		t.Fatalf("default did not wait for blocked worker: exit=%d output=%s", got.code, got.output)
	case <-time.After(250 * time.Millisecond):
	}
	cancel()
	var result foregroundResult
	select {
	case result = <-foregroundDone:
	case <-time.After(5 * time.Second):
		t.Fatal("foreground did not react to explicit context cancellation")
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(result.output), &receipt); err != nil {
		t.Fatalf("parent output: %v %s", err, result.output)
	}
	receipt, _ = receipt["output"].(map[string]any)
	id, _ := receipt["operation_id"].(string)
	if result.code == 0 || id == "" || receipt["waiting_stopped"] != true {
		t.Fatalf("cancel result: exit=%d output=%s", result.code, result.output)
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	select {
	case <-workerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
	control, err := store.ReadCancel(id)
	if err != nil || control.RequestedAt.IsZero() || control.AcknowledgedAt.IsZero() {
		t.Fatalf("control=%+v err=%v", control, err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("worker request was not canceled")
	}
	if writes.Load() != 0 {
		t.Fatalf("writes=%d", writes.Load())
	}
	_ = started
}

func TestLocalCancelBypassesRemotePolicyAndTargetResolution(t *testing.T) {
	directory := t.TempDir()
	store := operationrun.Store{Directory: directory}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	options := overviewOptions(t, t.TempDir())
	options.OperationDirectory = directory
	options.ConfigPath = filepath.Join(t.TempDir(), "missing.yaml")
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"job.cancel": true}}
	var output strings.Builder
	code := Run(t.Context(), []string{"job", "cancel", "--operation-id", record.ID, "--json"}, &output, options)
	if code != 0 || !strings.Contains(output.String(), "cancellation_requested") {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
	control, err := store.ReadCancel(record.ID)
	if err != nil || control.RequestedAt.IsZero() || !control.AcknowledgedAt.IsZero() {
		t.Fatalf("control=%+v err=%v", control, err)
	}
	output.Reset()
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"job.inspect": true}}
	if code := Run(t.Context(), []string{"job", "inspect", "--operation-id", record.ID, "--json"}, &output, options); code != 0 || !strings.Contains(output.String(), "cancellation_requested") {
		t.Fatalf("inspection code=%d output=%s", code, output.String())
	}
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{}}
	output.Reset()
	if code := Run(t.Context(), []string{"job", "cancel", "--operation-id", record.ID, "--json"}, &output, options); code == 0 || !strings.Contains(output.String(), "policy.denied") {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
}

func TestCancellationDuringAdmittedSubmissionPreservesSuccess(t *testing.T) {
	submitted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var writes atomic.Int32
	var workerRequestCanceled atomic.Bool
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.Copy(io.Discard, r.Body)
			if writes.Add(1) == 1 {
				close(submitted)
			}
			select {
			case <-r.Context().Done():
				workerRequestCanceled.Store(true)
				return
			case <-release:
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1"/></workbook></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><workbooks/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/created-diagnostic"):
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1" name="Analytics"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Native.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	workerDone := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchPublicationWorkerProcessTest(ctx, directory, id, options.JobDirectory, server.Certificate().Raw, workerDone)
	}
	parentCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type foregroundResult struct {
		code   int
		output string
	}
	foregroundDone := make(chan foregroundResult, 1)
	started := time.Now()
	go func() {
		var output strings.Builder
		code := Run(parentCtx, []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &output, options)
		foregroundDone <- foregroundResult{code, output.String()}
	}()
	select {
	case <-submitted:
	case got := <-foregroundDone:
		t.Fatalf("default returned before held submission: exit=%d output=%s", got.code, got.output)
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not reach held submission")
	}
	select {
	case got := <-foregroundDone:
		t.Fatalf("default did not wait for blocked worker: exit=%d output=%s", got.code, got.output)
	case <-time.After(250 * time.Millisecond):
	}
	cancel()
	var result foregroundResult
	select {
	case result = <-foregroundDone:
	case <-time.After(5 * time.Second):
		t.Fatal("foreground did not react to explicit context cancellation")
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(result.output), &receipt); err != nil {
		t.Fatalf("parent output: %v %s", err, result.output)
	}
	receipt, _ = receipt["output"].(map[string]any)
	id, _ := receipt["operation_id"].(string)
	if result.code == 0 || id == "" || receipt["waiting_stopped"] != true {
		t.Fatalf("cancel result: exit=%d output=%s", result.code, result.output)
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	before, err := store.Read(id)
	if err != nil || !before.FinishedAt.IsZero() {
		t.Fatalf("worker already finished while submission held: phase=%s finished=%v err=%v", before.Phase, before.FinishedAt, err)
	}
	if time.Since(before.RequestedAt) > time.Minute {
		t.Fatal("diagnostic unexpectedly near the20mincutoff")
	}
	select {
	case code := <-workerDone:
		t.Fatalf("worker exited before admitted submission completed: code=%d", code)
	default:
	}
	if workerRequestCanceled.Load() {
		t.Fatal("worker HTTP request was canceled with parent")
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case code := <-workerDone:
		if code != 0 {
			t.Fatalf("worker exit after release=%d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not complete after controlled release")
	}
	after, err := store.Read(id)
	if err != nil || after.Phase != operationrun.PhaseCompleted || after.FinishedAt.IsZero() || !strings.Contains(string(after.FullResult), "created-diagnostic") {
		t.Fatalf("final phase=%s finished=%v err=%v result=%s", after.Phase, after.FinishedAt, err, after.FullResult)
	}
	if writes.Load() != 1 || workerRequestCanceled.Load() {
		t.Fatalf("writes=%d worker request canceled=%v", writes.Load(), workerRequestCanceled.Load())
	}
	_ = started
}

func TestLocalCancelStopsRealWorkerBeforeSubmission(t *testing.T) {
	submitted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var writes atomic.Int32
	requestCanceled := make(chan struct{})
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.Copy(io.Discard, r.Body)
			close(submitted)
			select {
			case <-r.Context().Done():
				close(requestCanceled)
				return
			case <-release:
			}
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.Copy(io.Discard, r.Body)
			writes.Add(1)
			select {
			case <-r.Context().Done():
				close(requestCanceled)
				return
			case <-release:
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1"/></workbook></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><workbooks/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/created-diagnostic"):
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1" name="Analytics"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Native.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	workerDone := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchPublicationWorkerProcessTest(ctx, directory, id, options.JobDirectory, server.Certificate().Raw, workerDone)
	}
	parentCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type foregroundResult struct {
		code   int
		output string
	}
	foregroundDone := make(chan foregroundResult, 1)
	started := time.Now()
	go func() {
		var output strings.Builder
		code := Run(parentCtx, []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--no-wait", "--json"}, &output, options)
		foregroundDone <- foregroundResult{code, output.String()}
	}()
	var result foregroundResult
	select {
	case result = <-foregroundDone:
	case <-time.After(10 * time.Second):
		t.Fatal("no-wait did not return")
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(result.output), &receipt); err != nil {
		t.Fatal(err)
	}
	id, _ := receipt["operation_id"].(string)
	if result.code != 0 || id == "" {
		t.Fatalf("no-wait: %d %s", result.code, result.output)
	}
	select {
	case <-submitted:
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not reach validation")
	}
	var cancelOutput strings.Builder
	if code := Run(t.Context(), []string{"job", "cancel", "--operation-id", id, "--json"}, &cancelOutput, options); code != 0 || !strings.Contains(cancelOutput.String(), "cancellation_requested") {
		t.Fatalf("local cancellation: code=%d output=%s", code, cancelOutput.String())
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	select {
	case <-workerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
	control, err := store.ReadCancel(id)
	if err != nil || control.RequestedAt.IsZero() || control.AcknowledgedAt.IsZero() {
		t.Fatalf("control=%+v err=%v", control, err)
	}
	select {
	case <-requestCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("worker request was not canceled")
	}
	if writes.Load() != 0 {
		t.Fatalf("writes=%d", writes.Load())
	}
	_ = started
}

func TestForegroundDeadlineDetachesWithoutRequestingCancellation(t *testing.T) {
	submitted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var writes atomic.Int32
	var workerRequestCanceled atomic.Bool
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.Copy(io.Discard, r.Body)
			if writes.Add(1) == 1 {
				close(submitted)
			}
			select {
			case <-r.Context().Done():
				workerRequestCanceled.Store(true)
				return
			case <-release:
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1"/></workbook></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><workbooks/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/created-diagnostic"):
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1" name="Analytics"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Native.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	workerDone := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchPublicationWorkerProcessTest(ctx, directory, id, options.JobDirectory, server.Certificate().Raw, workerDone)
	}
	parentCtx, cancel := context.WithTimeout(t.Context(), 3*time.Second)
	defer cancel()
	type foregroundResult struct {
		code   int
		output string
	}
	foregroundDone := make(chan foregroundResult, 1)
	started := time.Now()
	go func() {
		var output strings.Builder
		code := Run(parentCtx, []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &output, options)
		foregroundDone <- foregroundResult{code, output.String()}
	}()
	select {
	case <-submitted:
	case got := <-foregroundDone:
		t.Fatalf("default returned before held submission: exit=%d output=%s", got.code, got.output)
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not reach held submission")
	}
	select {
	case got := <-foregroundDone:
		t.Fatalf("default did not wait for blocked worker: exit=%d output=%s", got.code, got.output)
	case <-time.After(250 * time.Millisecond):
	}
	var result foregroundResult
	select {
	case result = <-foregroundDone:
	case <-time.After(5 * time.Second):
		t.Fatal("foreground did not react to explicit context cancellation")
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(result.output), &receipt); err != nil {
		t.Fatalf("parent output: %v %s", err, result.output)
	}
	id, _ := receipt["operation_id"].(string)
	if result.code != 0 || id == "" || receipt["waiting_stopped"] != true {
		t.Fatalf("cancel result: exit=%d output=%s", result.code, result.output)
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	control, err := store.ReadCancel(id)
	if err != nil || !control.RequestedAt.IsZero() {
		t.Fatalf("deadline requested cancellation: %+v err=%v", control, err)
	}
	before, err := store.Read(id)
	if err != nil || !before.FinishedAt.IsZero() {
		t.Fatalf("worker already finished while submission held: phase=%s finished=%v err=%v", before.Phase, before.FinishedAt, err)
	}
	if time.Since(before.RequestedAt) > time.Minute {
		t.Fatal("diagnostic unexpectedly near the20mincutoff")
	}
	select {
	case code := <-workerDone:
		t.Fatalf("worker exited before admitted submission completed: code=%d", code)
	default:
	}
	if workerRequestCanceled.Load() {
		t.Fatal("worker HTTP request was canceled with parent")
	}

	releaseOnce.Do(func() { close(release) })
	select {
	case code := <-workerDone:
		if code != 0 {
			t.Fatalf("worker exit after release=%d", code)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not complete after controlled release")
	}
	after, err := store.Read(id)
	if err != nil || after.Phase != operationrun.PhaseCompleted || after.FinishedAt.IsZero() || !strings.Contains(string(after.FullResult), "created-diagnostic") {
		t.Fatalf("final phase=%s finished=%v err=%v result=%s", after.Phase, after.FinishedAt, err, after.FullResult)
	}
	if writes.Load() != 1 || workerRequestCanceled.Load() {
		t.Fatalf("writes=%d worker request canceled=%v", writes.Load(), workerRequestCanceled.Load())
	}
	_ = started
}

func TestLocalCancelReturnsRetainedTerminalOutcome(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	lease, _, err := store.Lease(record.ID, os.Getpid())
	if err != nil {
		t.Fatal(err)
	}
	_, err = lease.Update(func(r *operationrun.Record) error {
		r.Phase = operationrun.PhaseCompleted
		r.ExitCode = new(0)
		r.FinishedAt = time.Now().UTC()
		r.CompactResult = json.RawMessage(`{"status":"succeeded","workbook_luid":"saved-id"}`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := lease.Release(); err != nil {
		t.Fatal(err)
	}
	options := overviewOptions(t, t.TempDir())
	options.OperationDirectory = store.Directory
	var output strings.Builder
	code := Run(t.Context(), []string{"job", "cancel", "--operation-id", record.ID, "--json"}, &output, options)
	if code != 0 || !strings.Contains(output.String(), "already_finished") || !strings.Contains(output.String(), "saved-id") {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
}

func TestInterruptUsesTerminalRecordAfterIdempotentRequest(t *testing.T) {
	for _, exit := range []int{0, 1} {
		t.Run(fmt.Sprint(exit), func(t *testing.T) {
			store := operationrun.Store{Directory: t.TempDir()}
			stale, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
			if err != nil {
				t.Fatal(err)
			}
			if err := store.PrepareCancel(stale.ID); err != nil {
				t.Fatal(err)
			}
			if _, err := store.RequestCancel(stale.ID); err != nil {
				t.Fatal(err)
			}
			_, err = store.Update(stale.ID, func(r *operationrun.Record) error {
				r.Phase = operationrun.PhaseCompleted
				if exit != 0 {
					r.Phase = operationrun.PhaseFailed
				}
				r.ExitCode = new(exit)
				r.FinishedAt = time.Now().UTC()
				r.CompactResult = json.RawMessage(`{"status":"retained","workbook_luid":"saved-id"}`)
				return nil
			})
			if err != nil {
				t.Fatal(err)
			}
			capture := &lastCapture{}
			err = interruptPublication(store, stale, capture)
			if (err != nil) != (exit != 0) {
				t.Fatalf("exit=%d err=%v", exit, err)
			}
			out, ok := capture.value.(publicationRunOutput)
			if !ok || out.record.FinishedAt.IsZero() || !strings.Contains(string(out.record.CompactResult), "saved-id") {
				t.Fatalf("capture=%#v", capture.value)
			}
		})
	}
}

func TestInspectReportsCorruptControlAndPreservesSavedOutcome(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish"})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	_, err = store.Update(record.ID, func(r *operationrun.Record) error {
		r.Phase = operationrun.PhaseCompleted
		r.ExitCode = new(0)
		r.FinishedAt = time.Now().UTC()
		r.CompactResult = json.RawMessage(`{"status":"succeeded","workbook_luid":"saved-id"}`)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(store.Directory, record.ID+".json.control.json"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := overviewOptions(t, t.TempDir())
	options.OperationDirectory = store.Directory
	var output strings.Builder
	code := Run(t.Context(), []string{"job", "inspect", "--operation-id", record.ID, "--json"}, &output, options)
	if code != 0 || !strings.Contains(output.String(), "saved-id") || !strings.Contains(output.String(), "decode operation control") {
		t.Fatalf("code=%d output=%s", code, output.String())
	}
}

func TestWorkerControlReadFailureStopsWorkAndIsRetained(t *testing.T) {
	submitted := make(chan struct{})
	release := make(chan struct{})
	var releaseOnce sync.Once
	var writes atomic.Int32
	requestCanceled := make(chan struct{})
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.Copy(io.Discard, r.Body)
			close(submitted)
			select {
			case <-r.Context().Done():
				close(requestCanceled)
				return
			case <-release:
			}
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = io.Copy(io.Discard, r.Body)
			writes.Add(1)
			select {
			case <-r.Context().Done():
				close(requestCanceled)
				return
			case <-release:
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1"/></workbook></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><workbooks/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/created-diagnostic"):
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-diagnostic" name="Native"><project id="project-1" name="Analytics"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			http.Error(w, "unexpected", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	defer releaseOnce.Do(func() { close(release) })
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Native.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	workerDone := make(chan int, 1)
	options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
		return launchPublicationWorkerProcessTest(ctx, directory, id, options.JobDirectory, server.Certificate().Raw, workerDone)
	}
	parentCtx, cancel := context.WithCancel(t.Context())
	defer cancel()
	type foregroundResult struct {
		code   int
		output string
	}
	foregroundDone := make(chan foregroundResult, 1)
	started := time.Now()
	go func() {
		var output strings.Builder
		code := Run(parentCtx, []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--no-wait", "--json"}, &output, options)
		foregroundDone <- foregroundResult{code, output.String()}
	}()
	var result foregroundResult
	select {
	case result = <-foregroundDone:
	case <-time.After(10 * time.Second):
		t.Fatal("no-wait did not return")
	}
	var receipt map[string]any
	if err := json.Unmarshal([]byte(result.output), &receipt); err != nil {
		t.Fatal(err)
	}
	id, _ := receipt["operation_id"].(string)
	if result.code != 0 || id == "" {
		t.Fatalf("no-wait: %d %s", result.code, result.output)
	}
	select {
	case <-submitted:
	case <-time.After(20 * time.Second):
		t.Fatal("worker did not reach validation")
	}
	store := operationrun.Store{Directory: options.OperationDirectory}
	if err := os.WriteFile(filepath.Join(store.Directory, id+".json.control.json"), []byte("invalid"), 0o600); err != nil {
		t.Fatal(err)
	}

	select {
	case <-workerDone:
	case <-time.After(10 * time.Second):
		t.Fatal("worker did not stop after cancellation")
	}
	record, err := store.Read(id)
	if err != nil || !strings.Contains(record.Activity, "decode operation control") {
		t.Fatalf("activity=%q err=%v", record.Activity, err)
	}

	select {
	case <-requestCanceled:
	case <-time.After(5 * time.Second):
		t.Fatal("worker request was not canceled")
	}
	if writes.Load() != 0 {
		t.Fatalf("writes=%d", writes.Load())
	}
	_ = started
}

func TestWorkerRemovesTemporaryBatchAfterRun(t *testing.T) {
	store := operationrun.Store{Directory: t.TempDir()}
	record, err := store.Create(operationrun.Request{Operation: "workbook.publish", Args: []string{"unknown-command", "--batch-file", "original.json"}, BatchData: json.RawMessage(`{"items":[]}`), ConfigPath: filepath.Join(t.TempDir(), "config.yaml")})
	if err != nil {
		t.Fatal(err)
	}
	if err := store.PrepareCancel(record.ID); err != nil {
		t.Fatal(err)
	}
	_ = runPublicationWorker(t.Context(), store.Directory, record.ID, Options{JobDirectory: t.TempDir(), Stderr: io.Discard})
	if _, err := os.Stat(filepath.Join(store.Directory, record.ID+".batch.json")); !os.IsNotExist(err) {
		t.Fatalf("temporary batch remains: %v", err)
	}
	retained, err := store.Read(record.ID)
	if err != nil || string(retained.Request.BatchData) != `{"items":[]}` {
		t.Fatalf("retained batch=%s err=%v", retained.Request.BatchData, err)
	}
}

func TestWorkerStartupReadFailureKeepsOperationHandle(t *testing.T) {
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "unexpected", 500) }))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "input.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0o600); err != nil {
		t.Fatal(err)
	}
	options := publicationWorkerTestOptions(runtime.configPath, server, t)
	var id string
	options.WorkerLauncher = func(_ context.Context, directory, operationID string) error {
		id = operationID
		return os.WriteFile(filepath.Join(directory, id+".json"), []byte("invalid"), 0o600)
	}
	var output strings.Builder
	code := Run(t.Context(), []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &output, options)
	if code == 0 || id == "" || !strings.Contains(output.String(), id) || !strings.Contains(output.String(), "operation.worker.status_unknown") || !strings.Contains(output.String(), "job inspect --operation-id") {
		t.Fatalf("code=%d id=%s output=%s", code, id, output.String())
	}
}
