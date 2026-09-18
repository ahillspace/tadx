package app_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/jobmonitor"
	"github.com/ahillspace/tadx/internal/value"
)

type crossSiteJobFixture struct {
	mu                     sync.Mutex
	currentToken           string
	signIns                int
	jobReads               int
	publicationPosts       int
	monitorReadStarted     chan struct{}
	monitorReadStartedOnce sync.Once
}

func newCrossSiteJobFixture() *crossSiteJobFixture {
	return &crossSiteJobFixture{monitorReadStarted: make(chan struct{})}
}

func (f *crossSiteJobFixture) signIn(site string) string {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.signIns++
	f.currentToken = fmt.Sprintf("cross-site-session-%d-%s", f.signIns, site)
	return f.currentToken
}

func (f *crossSiteJobFixture) authenticated(request *http.Request) bool {
	f.mu.Lock()
	defer f.mu.Unlock()
	return request.Header.Get("X-Tableau-Auth") == f.currentToken
}

func (f *crossSiteJobFixture) recordJobRead() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.jobReads++
	return f.jobReads
}

func (f *crossSiteJobFixture) snapshot() (signIns, jobReads, publicationPosts int) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.signIns, f.jobReads, f.publicationPosts
}

func TestJobWaitSurvivesIndependentCrossSiteAuthCheckWithoutResubmission(t *testing.T) {
	fixture := newCrossSiteJobFixture()
	server := httptest.NewTLSServer(http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		switch {
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/auth/signin"):
			var payload struct {
				Credentials struct {
					Site struct {
						ContentURL string `json:"contentUrl"`
					} `json:"site"`
				} `json:"credentials"`
			}
			if err := json.NewDecoder(request.Body).Decode(&payload); err != nil || payload.Credentials.Site.ContentURL == "" {
				http.Error(response, "invalid sign-in request", http.StatusBadRequest)
				return
			}
			site := payload.Credentials.Site.ContentURL
			token := fixture.signIn(site)
			response.Header().Set("Content-Type", "application/json")
			_, _ = fmt.Fprintf(response, `{"credentials":{"token":%q,"site":{"id":%q},"user":{"id":"user-1"}}}`, token, site+"-luid")
		case request.Method == http.MethodGet && strings.HasSuffix(request.URL.Path, "/jobs/job-cross-site"):
			if !strings.Contains(request.URL.Path, "/sites/site-a-luid/") {
				http.Error(response, "unexpected job site", http.StatusNotFound)
				return
			}
			read := fixture.recordJobRead()
			if !fixture.authenticated(request) {
				http.Error(response, "session invalidated", http.StatusUnauthorized)
				return
			}
			status := `progress="50" finishCode="0"`
			if read >= 2 {
				status = `progress="100" finishCode="0"`
			}
			_, _ = fmt.Fprintf(response, `<tsResponse><job id="job-cross-site" type="RefreshExtract" %s/></tsResponse>`, status)
			if read == 1 {
				fixture.monitorReadStartedOnce.Do(func() { close(fixture.monitorReadStarted) })
			}
		case request.Method == http.MethodPost && strings.HasSuffix(request.URL.Path, "/workbooks"):
			fixture.mu.Lock()
			fixture.publicationPosts++
			fixture.mu.Unlock()
			http.Error(response, "publication was not expected", http.StatusConflict)
		default:
			t.Errorf("unexpected request %s %s", request.Method, request.URL.Path)
			http.Error(response, "unexpected request", http.StatusNotFound)
		}
	}))
	defer server.Close()

	t.Setenv("PROD_PAT_NAME", "cross-site-pat")
	t.Setenv("PROD_PAT_SECRET", "cross-site-secret")
	configA := writePhaseOneConfigWithSite(t, server.URL, "site-a")
	configB := writePhaseOneConfigWithSite(t, server.URL, "site-b")
	jobDirectory := t.TempDir()
	store := jobmonitor.Store{Directory: jobDirectory}
	if _, err := store.Register(t.Context(), jobmonitor.Receipt{
		Version:           1,
		Operation:         "job.wait",
		Environment:       "production",
		Server:            server.URL,
		Site:              "site-a",
		SiteID:            "site-a-luid",
		ConfigPath:        configA,
		CoordinationKey:   "cross-site-coordination",
		TrackingStartedAt: time.Now().UTC(),
		PoolAfter:         time.Now().UTC().Add(time.Hour),
		Observation:       value.JobStatus{ID: "job-cross-site", Type: "RefreshExtract", Status: "pending"},
	}); err != nil {
		t.Fatal(err)
	}

	jobContext, cancelJob := context.WithTimeout(t.Context(), 12*time.Second)
	defer cancelJob()
	type runResult struct {
		code   int
		output string
	}
	jobDone := make(chan runResult, 1)
	go func() {
		var output strings.Builder
		code := app.Run(jobContext, []string{"job", "wait", "--environment", "production", "--site", "site-a", "--id", "job-cross-site"}, &output, app.Options{ConfigPath: configA, HTTPClient: server.Client(), JobDirectory: jobDirectory})
		jobDone <- runResult{code: code, output: output.String()}
	}()

	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()
	select {
	case <-fixture.monitorReadStarted:
	case result := <-jobDone:
		t.Fatalf("job wait ended before the cross-site check: code=%d output=%s", result.code, result.output)
	case <-timer.C:
		cancelJob()
		result := <-jobDone
		t.Fatalf("job wait did not reach its first monitor read: code=%d output=%s", result.code, result.output)
	}

	var authOutput strings.Builder
	authContext, cancelAuth := context.WithTimeout(t.Context(), 5*time.Second)
	authCode := app.Run(authContext, []string{"auth", "check", "--environment", "production"}, &authOutput, app.Options{ConfigPath: configB, HTTPClient: server.Client()})
	cancelAuth()
	if authCode != 0 || !strings.Contains(authOutput.String(), "site-b-luid") {
		cancelJob()
		result := <-jobDone
		t.Fatalf("site-b auth check code=%d output=%s; job wait code=%d output=%s", authCode, authOutput.String(), result.code, result.output)
	}

	var result runResult
	select {
	case result = <-jobDone:
	case <-jobContext.Done():
		result = <-jobDone
		signIns, jobReads, publicationPosts := fixture.snapshot()
		t.Fatalf("job wait timed out: code=%d sign-ins=%d job-reads=%d publication-posts=%d output=%s", result.code, signIns, jobReads, publicationPosts, result.output)
	}
	if result.code != 0 || !strings.Contains(result.output, "status: succeeded") || !strings.Contains(result.output, "job-cross-site") {
		t.Fatalf("job wait code=%d output=%s", result.code, result.output)
	}

	signIns, jobReads, publicationPosts := fixture.snapshot()
	if signIns != 3 {
		t.Fatalf("sign-ins=%d, want site-a, site-b, and resumed site-a authentication", signIns)
	}
	if jobReads < 2 {
		t.Fatalf("job reads=%d, want the first and resumed monitor reads", jobReads)
	}
	if publicationPosts != 0 {
		t.Fatalf("publication POSTs=%d, want no resubmission", publicationPosts)
	}

	receipt, _, err := (jobmonitor.Store{Directory: jobDirectory}).FindByJobID(t.Context(), "job-cross-site", "production", "site-a")
	if err != nil {
		t.Fatal(err)
	}
	if receipt.Observation.ID != "job-cross-site" || receipt.Observation.Status != "succeeded" || receipt.Site != "site-a" || receipt.SiteID != "site-a-luid" {
		t.Fatalf("retained receipt=%+v, want exact terminal site-a job identity", receipt)
	}

}
