package app_test

import (
	"context"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
	"strings"
	"testing"
	"time"
)

func TestCachedProjectPathIsOpaqueThroughCLI(t *testing.T) {
	server, _ := slashInventoryServer(t)
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	runProjectFlowCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "projects")
	for _, selector := range [][]string{{"--project", "Ops/Reports"}, {"--project-id", "slash"}, {"--project-id", "nested"}, {"--project", "does/not/exist"}} {
		var out strings.Builder
		args := append([]string{"content", "project", "inspect", "--environment", "production", "--cache"}, selector...)
		exit := app.Run(context.Background(), args, &out, options)
		if selector[0] == "--project-id" {
			if exit != 0 {
				t.Fatalf("%v: %s", selector, out.String())
			}
		} else if exit == 0 {
			t.Fatalf("ambiguous or missing path succeeded: %s", out.String())
		}
	}
	store := targetCacheFixture(t, options.ConfigPath, nil)
	_, err := store.ReplaceResourceScope(context.Background(), cache.ResourceScopeReplacement{Environment: "production", Kind: "project", Source: "test", GeneratedAt: time.Now(), Entries: []cache.ResourceEntry{{LUID: "literal-only", Name: "Ops/Reports", ProjectPath: "Ops/Reports"}}})
	if err != nil {
		t.Fatal(err)
	}
	output := runProjectFlowCLI(t, options, "content", "project", "inspect", "--environment", "production", "--cache", "--project", "Ops/Reports")
	if !strings.Contains(output, "literal-only") {
		t.Fatalf("literal-only opaque path did not resolve: %s", output)
	}
}
