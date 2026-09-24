package releasenotice

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/version"
)

func TestUntrustedReleaseResponsesStaySilent(t *testing.T) {
	for _, body := range []string{
		`not JSON`,
		`{"tag_name":"v1.1.0","html_url":"https://example.invalid/release"}`,
		`{"tag_name":"v1.1.0-rc.1","html_url":"https://github.com/ahillspace/tadx/releases/tag/v1.1.0-rc.1"}`,
		strings.Repeat("x", 65*1024),
	} {
		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		options := testOptions(t.TempDir(), time.Now())
		options.Latest = func(ctx context.Context) (version.Release, error) {
			return (version.Checker{Client: publicClient(), URL: server.URL}).Latest(ctx)
		}
		check := Start(t.Context(), options)
		<-check.Done()
		if notice := check.Finish(true); notice != "" {
			t.Errorf("untrusted response produced notice %q", notice)
		}
		server.Close()
	}
}

func TestPublicClientDoesNotFollowRedirects(t *testing.T) {
	var redirected atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		redirected.Add(1)
		w.WriteHeader(http.StatusOK)
	}))
	defer destination.Close()
	source := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusFound)
	}))
	defer source.Close()
	_, err := (version.Checker{Client: publicClient(), URL: source.URL}).Latest(t.Context())
	if err == nil || redirected.Load() != 0 {
		t.Fatalf("redirect err=%v destination requests=%d", err, redirected.Load())
	}
}
