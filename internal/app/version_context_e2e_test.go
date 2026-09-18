package app

import (
	"errors"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/version"
)

type failedReleaseTransport struct{}

func (failedReleaseTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return nil, errors.New("release fixture unavailable")
}

func TestVersionCheckFailureRetainsInstalledVersionThroughCLI(t *testing.T) {
	var out strings.Builder
	exit := Run(t.Context(), []string{"version", "--check", "--json"}, &out, Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), HTTPClient: &http.Client{Transport: failedReleaseTransport{}}})
	if exit == 0 || !strings.Contains(out.String(), `"version":"`+version.Current()+`"`) || !strings.Contains(out.String(), "check_unavailable") || strings.Contains(out.String(), "offline version information") {
		t.Fatalf("exit=%d output=%s", exit, out.String())
	}
}
