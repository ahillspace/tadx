package main

import (
	"bytes"
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/releasenotice"
	"github.com/ahillspace/tadx/internal/version"
)

func TestRunWithReleaseNoticePreservesOutputAndStatus(t *testing.T) {
	for _, test := range []struct {
		name   string
		status int
		notice bool
	}{
		{"success", 0, true},
		{"failure", 2, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			var stdout, stderr bytes.Buffer
			cache := t.TempDir()
			options := releasenotice.Options{
				Current:    "1.0.0",
				IsTerminal: func(io.Writer) bool { return true },
				Env:        func(string) string { return "" },
				CacheDir:   func() (string, error) { return cache, nil },
				Latest: func(context.Context) (version.Release, error) {
					return version.Release{Version: "1.1.0", URL: "https://github.com/ahillspace/tadx/releases/tag/v1.1.0"}, nil
				},
			}
			options.Args, options.Stdout, options.Stderr = []string{"capability", "list"}, &stdout, &stderr
			check := releasenotice.Start(t.Context(), options)
			<-check.Done()
			check.Finish(false)
			got := runWithReleaseNotice(t.Context(), []string{"capability", "list"}, &stdout, &stderr, options, func() int {
				_, _ = stdout.WriteString("command result\n")
				return test.status
			})
			if got != test.status || stdout.String() != "command result\n" ||
				strings.Contains(stderr.String(), "1.1.0") != test.notice {
				t.Fatalf("status=%d stdout=%q stderr=%q", got, stdout.String(), stderr.String())
			}
		})
	}
}

func TestRunWithReleaseNoticeDoesNotDelayFastCommand(t *testing.T) {
	var stdout, stderr bytes.Buffer
	options := releasenotice.Options{
		Current:    "1.0.0",
		IsTerminal: func(io.Writer) bool { return true },
		Env:        func(string) string { return "" },
		CacheDir:   func() (string, error) { return t.TempDir(), nil },
		Latest: func(ctx context.Context) (version.Release, error) {
			<-ctx.Done()
			return version.Release{}, ctx.Err()
		},
	}
	start := time.Now()
	got := runWithReleaseNotice(t.Context(), []string{"capability", "list"}, &stdout, &stderr, options, func() int { return 0 })
	if got != 0 || stderr.Len() != 0 || time.Since(start) > time.Second {
		t.Fatalf("status=%d stderr=%q elapsed=%s", got, stderr.String(), time.Since(start))
	}
}
