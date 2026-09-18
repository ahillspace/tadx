package app_test

import (
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/value"
)

func TestJobInspectCancelAndWaitUseExactRemoteJobs(t *testing.T) {
	var gets, cancels, pendingCancels, signins, observedGets atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/auth/signin"):
			signins.Add(1)
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-1"):
			gets.Add(1)
			if cancels.Load() > 0 {
				_, _ = io.WriteString(w, `<tsResponse><job id="job-1" type="RefreshExtract" progress="100" finishCode="2"/></tsResponse>`)
				return
			}
			_, _ = io.WriteString(w, `<tsResponse><job id="job-1" type="RefreshExtract" progress="50" finishCode="0"/></tsResponse>`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/jobs/job-1"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read cancellation body: %v", err)
			}
			if len(body) != 0 {
				t.Errorf("cancellation body = %q, want empty", body)
			}
			cancels.Add(1)
			w.Header().Set("X-Tableau-Request-Id", "cancel-request-1")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-pending-cancel"):
			_, _ = io.WriteString(w, `<tsResponse><job id="job-pending-cancel" type="RefreshExtract" progress="50" finishCode="0"/></tsResponse>`)
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/jobs/job-pending-cancel"):
			body, err := io.ReadAll(r.Body)
			if err != nil {
				t.Errorf("read pending cancellation body: %v", err)
			}
			if len(body) != 0 {
				t.Errorf("pending cancellation body = %q, want empty", body)
			}
			pendingCancels.Add(1)
			w.Header().Set("X-Tableau-Request-Id", "pending-cancel-request")
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-wait"):
			gets.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><job id="job-wait" type="RefreshExtract" progress="100" finishCode="0"/></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-publication"):
			_, _ = io.WriteString(w, `<tsResponse><job id="job-publication" type="PublishWorkbook" progress="50" finishCode="0"/></tsResponse>`)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/jobs/job-observe"):
			if observedGets.Add(1) == 1 {
				_, _ = io.WriteString(w, `<tsResponse><job id="job-observe" type="RefreshExtract" progress="50" finishCode="0"/></tsResponse>`)
				return
			}
			_, _ = io.WriteString(w, `<tsResponse><job id="job-observe" type="RefreshExtract" progress="100" finishCode="0"/></tsResponse>`)
		default:
			t.Errorf("unexpected job request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	configPath := writePhaseOneConfigWithSite(t, server.URL, "team-site")
	options := app.Options{ConfigPath: configPath, HTTPClient: server.Client(), MutationsEnabled: true}

	var inspect strings.Builder
	if code := app.Run(t.Context(), []string{"job", "inspect", "--environment", "production", "--id", "job-1"}, &inspect, options); code != 0 || !strings.Contains(inspect.String(), "status: running") || !strings.Contains(inspect.String(), "job-1") {
		t.Fatalf("inspect code=%d output=%s", code, inspect.String())
	}
	var inspectBatch strings.Builder
	if code := app.Run(t.Context(), []string{"job", "inspect", "--environment", "production", "--id", "job-1", "--id", "job-observe"}, &inspectBatch, options); code != 0 || !strings.Contains(inspectBatch.String(), "total: 2") || !strings.Contains(inspectBatch.String(), "job-observe") {
		t.Fatalf("inspect batch code=%d output=%s", code, inspectBatch.String())
	}

	beforePreview := cancels.Load()
	var preview strings.Builder
	if code := app.Run(t.Context(), []string{"job", "cancel", "--environment", "production", "--id", "job-1", "--preview"}, &preview, options); code != 0 || cancels.Load() != beforePreview || !strings.Contains(preview.String(), "status: preview") {
		t.Fatalf("preview code=%d cancels=%d/%d output=%s", code, cancels.Load(), beforePreview, preview.String())
	}

	var cancel strings.Builder
	if code := app.Run(t.Context(), []string{"job", "cancel", "--environment", "production", "--id", "job-1"}, &cancel, options); code != 0 || cancels.Load() != beforePreview+1 || !strings.Contains(cancel.String(), "status: cancelled") || !strings.Contains(cancel.String(), "confirmed: true") || !strings.Contains(cancel.String(), "cancel-request-1") {
		t.Fatalf("cancel code=%d cancels=%d output=%s", code, cancels.Load(), cancel.String())
	}

	var unsupported strings.Builder
	if code := app.Run(t.Context(), []string{"job", "cancel", "--environment", "production", "--id", "job-publication"}, &unsupported, options); code == 0 || !strings.Contains(unsupported.String(), "job.cancel.unsupported") || cancels.Load() != beforePreview+1 {
		t.Fatalf("unsupported cancel code=%d cancels=%d output=%s", code, cancels.Load(), unsupported.String())
	}
	var pendingCancel strings.Builder
	if code := app.Run(t.Context(), []string{"job", "cancel", "--environment", "production", "--id", "job-pending-cancel"}, &pendingCancel, options); code != 0 || pendingCancels.Load() != 1 || !strings.Contains(pendingCancel.String(), "inspect the exact job again") || strings.Contains(pendingCancel.String(), "job wait") {
		t.Fatalf("pending cancel code=%d requests=%d output=%s", code, pendingCancels.Load(), pendingCancel.String())
	}

	jobDirectory := t.TempDir()
	store := jobmonitor.Store{Directory: jobDirectory}
	if _, err := store.Save(t.Context(), jobmonitor.Receipt{
		Version: 1, Operation: "workbook.publish", Environment: "production", Server: server.URL + "/", Site: "team-site", SiteID: "site-1", ConfigPath: configPath, CoordinationKey: "fixture-coordination", AcceptedAt: time.Now().UTC(), NextCheck: time.Now().UTC().Add(time.Hour), ReadFailures: 3, LastReadError: "job status could not be read", Observation: value.JobStatus{ID: "job-wait", Type: "RefreshExtract", Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}
	var recovered strings.Builder
	waitOptions := options
	waitOptions.JobDirectory = jobDirectory
	if code := app.Run(t.Context(), []string{"job", "wait", "--environment", "production", "--site", "team-site", "--id", "job-wait"}, &recovered, waitOptions); code != 0 || !strings.Contains(recovered.String(), "status: succeeded") || !strings.Contains(recovered.String(), "job-wait") {
		t.Fatalf("wait code=%d output=%s", code, recovered.String())
	}
	if signins.Load() == 0 || gets.Load() < 4 {
		t.Fatalf("signins=%d gets=%d, expected authenticated exact-job observations", signins.Load(), gets.Load())
	}
	var observed strings.Builder
	if code := app.Run(t.Context(), []string{"job", "wait", "--environment", "production", "--site", "team-site", "--id", "job-observe"}, &observed, waitOptions); code != 0 || !strings.Contains(observed.String(), "status: succeeded") || !strings.Contains(observed.String(), "job-observe") {
		t.Fatalf("observation wait code=%d output=%s", code, observed.String())
	}
	tracked, _, err := store.FindByJobID(t.Context(), "job-observe", "production", "team-site")
	if err != nil || tracked.TrackingStartedAt.IsZero() || !tracked.AcceptedAt.IsZero() {
		t.Fatalf("observation receipt=%+v err=%v; expected tracking timestamp without accepted timestamp", tracked, err)
	}

	if _, err := store.Register(t.Context(), jobmonitor.Receipt{
		Version: 1, Operation: "workbook.publish", Environment: "production", Server: server.URL, Site: "team-site", SiteID: "site-1", ConfigPath: configPath, CoordinationKey: "fixture-coordination", AcceptedAt: time.Now().UTC(), Observation: value.JobStatus{ID: "job-mismatch", Type: "RefreshExtract", Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}
	configuration, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(configPath, []byte(strings.Replace(string(configuration), "site_content_url: \"team-site\"", "site_content_url: \"other-site\"", 1)), 0o600); err != nil {
		t.Fatal(err)
	}
	var mismatch strings.Builder
	if code := app.Run(t.Context(), []string{"job", "wait", "--id", "job-mismatch"}, &mismatch, waitOptions); code == 0 || !strings.Contains(mismatch.String(), "job-mismatch") || !strings.Contains(mismatch.String(), "output:") || !strings.Contains(mismatch.String(), filepath.Base(configPath)) {
		t.Fatalf("mismatch code=%d output=%s", code, mismatch.String())
	}
}

func TestJobHelpAdvertisesExactControls(t *testing.T) {
	var inspect strings.Builder
	if code := app.Run(context.Background(), []string{"job", "inspect", "--help"}, &inspect, app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}); code != 0 {
		t.Fatalf("inspect help code=%d output=%s", code, inspect.String())
	}
	if strings.Contains(inspect.String(), "--repeat") || strings.Contains(inspect.String(), "--interval") {
		t.Fatalf("inspect help advertises polling controls: %s", inspect.String())
	}
	var wait strings.Builder
	if code := app.Run(context.Background(), []string{"job", "wait", "--help"}, &wait, app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}); code != 0 || !strings.Contains(wait.String(), "begin exact observation") {
		t.Fatalf("wait help code=%d output=%s", code, wait.String())
	}
	var output strings.Builder
	if code := app.Run(context.Background(), []string{"job", "cancel", "--help"}, &output, app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")}); code != 0 {
		t.Fatalf("help code=%d output=%s", code, output.String())
	}
	for _, want := range []string{"Usage: tadx job cancel [flags]", "--id <job-luid>", "--preview"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("help missing %q: %s", want, output.String())
		}
	}
}
