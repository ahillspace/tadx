package releasenotice

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/version"
)

func TestStableVersionComparison(t *testing.T) {
	for _, test := range []struct {
		current, latest string
		newer           bool
	}{
		{"1.2.3", "1.2.4", true},
		{"1.2.3", "1.2.3", false},
		{"1.2.4", "1.2.3", false},
		{"1.9.9", "1.10.0", true},
		{"dev", "1.0.0", false},
		{"1.2.3-dirty", "1.2.4", false},
		{"1.2.3+dirty", "1.2.4", false},
		{"1.2.3", "1.2.4-rc.1", false},
		{"1.2.3", "malformed", false},
	} {
		if got := newerStable(test.current, test.latest); got != test.newer {
			t.Errorf("newerStable(%q, %q) = %t, want %t", test.current, test.latest, got, test.newer)
		}
	}
}

func TestCompletedCheckCachesReleaseAndNotice(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	directory := t.TempDir()
	calls := 0
	options := testOptions(directory, now)
	options.Latest = func(context.Context) (version.Release, error) {
		calls++
		return version.Release{Version: "1.3.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v1.3.0"}, nil
	}
	check := Start(t.Context(), options)
	if check == nil {
		t.Fatal("eligible check did not start")
	}
	<-check.Done()
	if notice := check.Finish(true); !strings.Contains(notice, "1.3.0") || !strings.Contains(notice, "tadx update") {
		t.Fatalf("notice = %q", notice)
	}
	if notice := Start(t.Context(), options).Finish(true); notice != "" {
		t.Fatalf("repeated notice = %q", notice)
	}
	if calls != 1 {
		t.Fatalf("network calls = %d, want 1", calls)
	}
	options.Now = func() time.Time { return now.Add(25 * time.Hour) }
	check = Start(t.Context(), options)
	<-check.Done()
	if notice := check.Finish(true); !strings.Contains(notice, "1.3.0") {
		t.Fatalf("next-day notice = %q", notice)
	}
	if calls != 2 {
		t.Fatalf("network calls = %d, want 2", calls)
	}
}

func TestFailedCommandCanNotifyFromCache(t *testing.T) {
	options := testOptions(t.TempDir(), time.Now())
	options.Latest = func(context.Context) (version.Release, error) {
		return version.Release{Version: "2.0.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v2.0.0"}, nil
	}
	check := Start(t.Context(), options)
	<-check.Done()
	if notice := check.Finish(false); notice != "" {
		t.Fatalf("failure notice = %q", notice)
	}
	if notice := Start(t.Context(), options).Finish(true); !strings.Contains(notice, "2.0.0") {
		t.Fatalf("cached notice = %q", notice)
	}
}

func TestRefreshPreservesDelayedNoticeCadence(t *testing.T) {
	now := time.Date(2026, 9, 23, 12, 0, 0, 0, time.UTC)
	options := testOptions(t.TempDir(), now)
	options.Latest = func(context.Context) (version.Release, error) {
		return version.Release{Version: "2.0.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v2.0.0"}, nil
	}
	check := Start(t.Context(), options)
	<-check.Done()
	if notice := check.Finish(false); notice != "" {
		t.Fatalf("failed command notice = %q", notice)
	}
	options.Now = func() time.Time { return now.Add(23 * time.Hour) }
	if notice := Start(t.Context(), options).Finish(true); notice == "" {
		t.Fatal("expected first notice from cached release")
	}
	options.Now = func() time.Time { return now.Add(25 * time.Hour) }
	check = Start(t.Context(), options)
	<-check.Done()
	if notice := check.Finish(true); notice != "" {
		t.Fatalf("refresh repeated notice after only two hours: %q", notice)
	}
	options.Now = func() time.Time { return now.Add(47 * time.Hour) }
	if notice := Start(t.Context(), options).Finish(true); notice == "" {
		t.Fatal("expected notice after its independent 24-hour interval")
	}
}

func TestCompletedFailureIsThrottled(t *testing.T) {
	options := testOptions(t.TempDir(), time.Now())
	calls := 0
	options.Latest = func(context.Context) (version.Release, error) {
		calls++
		return version.Release{}, errors.New("offline")
	}
	check := Start(t.Context(), options)
	<-check.Done()
	if notice := check.Finish(true); notice != "" {
		t.Fatalf("failure notice = %q", notice)
	}
	if notice := Start(t.Context(), options).Finish(true); notice != "" || calls != 1 {
		t.Fatalf("cached failure notice = %q, calls = %d", notice, calls)
	}
}

func TestCanceledCheckDoesNotWaitOrThrottle(t *testing.T) {
	options := testOptions(t.TempDir(), time.Now())
	entered := make(chan struct{})
	options.Latest = func(ctx context.Context) (version.Release, error) {
		close(entered)
		<-ctx.Done()
		return version.Release{}, ctx.Err()
	}
	check := Start(t.Context(), options)
	<-entered
	started := time.Now()
	if notice := check.Finish(true); notice != "" {
		t.Fatalf("canceled notice = %q", notice)
	}
	if elapsed := time.Since(started); elapsed > 100*time.Millisecond {
		t.Fatalf("finish waited %s", elapsed)
	}
	options.Latest = func(context.Context) (version.Release, error) {
		return version.Release{Version: "1.1.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v1.1.0"}, nil
	}
	check = Start(t.Context(), options)
	<-check.Done()
	if notice := check.Finish(true); notice == "" {
		t.Fatal("canceled attempt throttled next command")
	}
}

func TestEligibilitySkipsWithoutCacheOrNetwork(t *testing.T) {
	for _, test := range []struct {
		name string
		edit func(*Options)
	}{
		{"bare", func(o *Options) { o.Args = nil }},
		{"help", func(o *Options) { o.Args = []string{"capability", "list", "--help"} }},
		{"help explicit true", func(o *Options) { o.Args = []string{"capability", "list", "--help=true"} }},
		{"help shorthand cluster", func(o *Options) { o.Args = []string{"capability", "list", "-fh"} }},
		{"preview shorthand cluster", func(o *Options) { o.Args = []string{"admin", "group", "create", "--name", "Example", "-fp"} }},
		{"preview shorthand cluster explicit true", func(o *Options) { o.Args = []string{"admin", "group", "create", "--name", "Example", "-fp=true"} }},
		{"help command", func(o *Options) { o.Args = []string{"help", "capability"} }},
		{"completion", func(o *Options) { o.Args = []string{"completion", "bash"} }},
		{"preview", func(o *Options) { o.Args = []string{"capability", "list", "--preview"} }},
		{"preview alias", func(o *Options) { o.Args = []string{"content", "workbook", "delete", "--pv"} }},
		{"preview shorthand", func(o *Options) { o.Args = []string{"content", "workbook", "delete", "-p"} }},
		{"update", func(o *Options) { o.Args = []string{"update", "--check"} }},
		{"update alias", func(o *Options) { o.Args = []string{"upd", "--chk"} }},
		{"cache category", func(o *Options) { o.Args = []string{"cache", "status"} }},
		{"cache flag", func(o *Options) { o.Args = []string{"content", "workbook", "list", "--cache"} }},
		{"cache alias", func(o *Options) { o.Args = []string{"content", "workbook", "list", "--cch=true"} }},
		{"json", func(o *Options) { o.Args = []string{"capability", "list", "--json"} }},
		{"json alias", func(o *Options) { o.Args = []string{"capability", "list", "--jsn=true"} }},
		{"no stdout tty", func(o *Options) { o.Stdout = nil }},
		{"no stderr tty", func(o *Options) { o.Stderr = nil }},
		{"ci", func(o *Options) {
			o.Env = func(name string) string {
				if name == "CI" {
					return "true"
				}
				return ""
			}
		}},
		{"opt out", func(o *Options) {
			o.Env = func(name string) string {
				if name == "TADX_NO_UPDATE_NOTIFIER" {
					return "1"
				}
				return ""
			}
		}},
		{"dev", func(o *Options) { o.Current = "dev" }},
		{"dirty build", func(o *Options) { o.Modified = true }},
	} {
		t.Run(test.name, func(t *testing.T) {
			directory := t.TempDir()
			options := testOptions(directory, time.Now())
			var calls atomic.Int32
			options.IsTerminal = func(writer io.Writer) bool { return writer != nil }
			options.Latest = func(context.Context) (version.Release, error) {
				calls.Add(1)
				return version.Release{}, nil
			}
			test.edit(&options)
			if check := Start(t.Context(), options); check != nil {
				check.Finish(false)
				if check.Done() != nil {
					<-check.Done()
				}
				t.Fatal("ineligible check started")
			}
			if calls.Load() != 0 {
				t.Fatal("network called")
			}
			if _, err := os.Stat(filepath.Join(directory, "tadx")); !os.IsNotExist(err) {
				t.Fatalf("cache created: %v", err)
			}
		})
	}
}

func TestPublicRequestHasNoAuthorizationAndIsBounded(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		if request.Header.Get("Authorization") != "" || request.Header.Get("Cookie") != "" {
			t.Error("request carried credentials")
		}
		_, _ = writer.Write([]byte(`{"tag_name":"v1.1.0","html_url":"https://github.com/ahillspace/tadx/releases/tag/v1.1.0"}`))
	}))
	defer server.Close()
	options := testOptions(t.TempDir(), time.Now())
	options.Latest = func(ctx context.Context) (version.Release, error) {
		return (version.Checker{Client: publicClient(), URL: server.URL}).Latest(ctx)
	}
	check := Start(t.Context(), options)
	<-check.Done()
	if notice := check.Finish(true); notice == "" {
		t.Fatal("public release produced no notice")
	}
}

func testOptions(directory string, now time.Time) Options {
	return Options{
		Args: []string{"capability", "list"}, Current: "1.0.0",
		Stdout: io.Discard, Stderr: io.Discard,
		IsTerminal: func(io.Writer) bool { return true },
		Env:        func(string) string { return "" },
		CacheDir:   func() (string, error) { return directory, nil },
		Now:        func() time.Time { return now },
	}
}
