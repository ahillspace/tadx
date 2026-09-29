package app

import (
	"errors"
	"io"
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

type latestReleaseTransport struct{}

func (latestReleaseTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	body := `{"tag_name":"v0.1.1","html_url":"https://github.com/ahillspace/tadx/releases/tag/v0.1.1","published_at":"2026-01-02T03:04:05Z"}`
	return &http.Response{StatusCode: http.StatusOK, Header: http.Header{"Content-Type": []string{"application/json"}}, Body: io.NopCloser(strings.NewReader(body)), Request: request}, nil
}

func TestVersionCheckFailureRetainsInstalledVersionThroughCLI(t *testing.T) {
	var out strings.Builder
	exit := Run(t.Context(), []string{"version", "--check", "--json"}, &out, Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), HTTPClient: &http.Client{Transport: failedReleaseTransport{}}})
	if exit == 0 || !strings.Contains(out.String(), `"version":"`+version.Current()+`"`) || !strings.Contains(out.String(), "check_unavailable") || strings.Contains(out.String(), "offline version information") {
		t.Fatalf("exit=%d output=%s", exit, out.String())
	}
}

func TestVersionCheckRendersOnlyContextualFollowUpsThroughCLI(t *testing.T) {
	encodings := map[string][]string{"compact": nil, "full": {"--full"}, "json": {"--json"}}
	for name, transport := range map[string]http.RoundTripper{"unavailable": failedReleaseTransport{}, "current": latestReleaseTransport{}} {
		for encoding, flags := range encodings {
			t.Run(name+"/"+encoding, func(t *testing.T) {
				var out strings.Builder
				Run(t.Context(), append([]string{"version", "--check"}, flags...), &out, Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), HTTPClient: &http.Client{Transport: transport}})
				if strings.Contains(out.String(), "help") {
					t.Fatalf("release check without a follow-up rendered help:\n%s", out.String())
				}
			})
		}
	}
	previous := version.BuildVersion
	version.BuildVersion = "0.1.0"
	t.Cleanup(func() { version.BuildVersion = previous })
	// Shared rendering binds the selected configuration into the follow-up command.
	for encoding, want := range map[string][2]string{"compact": {"help[1]: tadx ", " update\n"}, "full": {"help[1]: tadx ", " update\n"}, "json": {`"help":["tadx `, ` update"]`}} {
		t.Run("update-available/"+encoding, func(t *testing.T) {
			var out strings.Builder
			exit := Run(t.Context(), append([]string{"version", "--check"}, encodings[encoding]...), &out, Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), HTTPClient: &http.Client{Transport: latestReleaseTransport{}}})
			if exit != 0 || !strings.Contains(out.String(), "update-available") || !strings.Contains(out.String(), want[0]) || !strings.Contains(out.String(), want[1]) {
				t.Fatalf("exit=%d output=%s, want tadx update follow-up", exit, out.String())
			}
		})
	}
}
