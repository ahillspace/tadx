package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
	tableaupulse "github.com/ahillspace/tadx/internal/tableau/pulse"
)

func TestPulseFollowerSnapshotReplacesAndSurvivesFailedReadThroughCLI(t *testing.T) {
	response := `{"subscriptions":[{"id":"old-subscription","follower":{"user_id":"old-user"}}]}`
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests++
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/metrics/metric-1":
			_, _ = io.WriteString(w, `{"id":"metric-1","definition_id":"definition-1","specification":{}}`)
		case "/api/-/pulse/subscriptions":
			_, _ = io.WriteString(w, response)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	run := func(cached bool) (int, string) {
		args := []string{"pulse", "metric", "followers", "--id", "metric-1"}
		if cached {
			args = append(args, "--cache")
		}
		var output bytes.Buffer
		code := app.Run(context.Background(), args, &output, options)
		return code, output.String()
	}
	for _, next := range []string{response, `{"subscriptions":[{"id":"new-subscription","follower":{"user_id":"new-user"}}]}`, `{"subscriptions":[]}`} {
		response = next
		code, live := run(false)
		if code != 0 {
			t.Fatalf("live code=%d output=%s", code, live)
		}
		before := requests
		code, cached := run(true)
		if code != 0 || requests != before {
			t.Fatalf("cached code=%d requests=%d output=%s", code, requests-before, cached)
		}
		if strings.Contains(next, "new-subscription") && (strings.Contains(cached, "old-subscription") || !strings.Contains(cached, "new-subscription")) {
			t.Fatalf("snapshot not replaced: %s", cached)
		}
		if next == `{"subscriptions":[]}` && !strings.Contains(cached, "count: 0") {
			t.Fatalf("empty snapshot lost: %s", cached)
		}
	}
	for _, invalid := range []string{`{"subscriptions":[],"next_page_token":"incomplete"}`, `{"subscriptions":null}`, `{"subscriptions":[{"id":"invalid"}]}`} {
		response = invalid
		if code, output := run(false); code == 0 {
			t.Fatalf("incomplete read succeeded: %s", output)
		}
		before := requests
		if code, output := run(true); code != 0 || requests != before || !strings.Contains(output, "count: 0") {
			t.Fatalf("failed read damaged snapshot: code=%d output=%s", code, output)
		}
	}
}

func TestPulseFollowerSnapshotIgnoresLegacyIndividualRowsThroughCLI(t *testing.T) {
	requests := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { requests++; w.WriteHeader(403) }))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	payload, _ := json.Marshal(tableaupulse.Subscription{LUID: "legacy-subscription", MetricLUID: "metric-1", FollowerType: "USER", FollowerLUID: "legacy-user"})
	store := targetCacheFixture(t, options.ConfigPath, nil)
	if err := store.UpsertResources(context.Background(), []cache.ResourceEntry{{Environment: "test", Site: "test", Kind: "pulse_subscription", LUID: "legacy-subscription", Name: "legacy", ProjectPath: "metric-1", Payload: payload, ObservedAt: time.Now().UTC(), Coverage: "detail"}}); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "metric", "followers", "--id", "metric-1", "--cache"}, &output, options)
	if code == 0 || requests != 0 || strings.Contains(output.String(), "legacy-user") {
		t.Fatalf("legacy cache accepted: code=%d requests=%d %s", code, requests, output.String())
	}
}

func TestPulseFollowerSnapshotCacheWriteFailureWarnsThroughCLI(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/-/pulse/metrics/metric-1":
			_, _ = io.WriteString(w, `{"id":"metric-1","definition_id":"definition-1","specification":{}}`)
		case "/api/-/pulse/subscriptions":
			_, _ = io.WriteString(w, `{"subscriptions":[]}`)
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
			w.WriteHeader(404)
		}
	}))
	defer server.Close()
	options := pulseEfficiencyOptions(t, server)
	store := targetCacheFixture(t, options.ConfigPath, nil)
	if err := os.MkdirAll(filepath.Join(filepath.Dir(options.ConfigPath), filepath.FromSlash(store.RelativePath())), 0700); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := app.Run(context.Background(), []string{"pulse", "metric", "followers", "--id", "metric-1"}, &output, options)
	if code != 0 || !strings.Contains(output.String(), "count: 0") || !strings.Contains(output.String(), "could not be cached") {
		t.Fatalf("cache failure code=%d %s", code, output.String())
	}
}
