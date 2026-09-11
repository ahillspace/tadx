package app_test

import (
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

func TestEnvironmentConcurrencyRoundTripThroughCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	run := func(wantSuccess bool, args ...string) string {
		t.Helper()
		var out strings.Builder
		code := app.Run(context.Background(), args, &out, app.Options{ConfigPath: path})
		if (code == 0) != wantSuccess {
			t.Fatalf("args=%v exit=%d output=%s", args, code, out.String())
		}
		return out.String()
	}
	check := func(want int) {
		t.Helper()
		cfg, err := config.Load(path)
		if err != nil || cfg.Environments["test"].CacheMaxConcurrency != want {
			t.Fatalf("config=%+v err=%v want concurrency=%d", cfg.Environments, err, want)
		}
	}
	run(true, "env", "add", "test", "--url", "https://tableau.example.test", "--site", "test-site", "--cache-max-concurrency", "8")
	check(8)
	if got := run(true, "env", "get", "test", "--full"); !strings.Contains(got, "cache_max_concurrency: 8") {
		t.Fatalf("missing configured concurrency: %s", got)
	}
	run(true, "env", "update", "test", "--cache-max-concurrency", "3")
	check(3)
	run(false, "env", "update", "test", "--cache-max-concurrency", "257")
	check(3)
	run(true, "env", "update", "test", "--clear-cache-max-concurrency")
	check(0)
}
