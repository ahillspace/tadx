package app_test

import (
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/cache"
)

func TestCacheStatusRetainsIndependentEvidenceWithoutGeneration(t *testing.T) {
	server, requests := cacheRecoveryServer(t, "project")
	defer server.Close()
	options := diagnosticOptions(t, server)
	store := targetCacheFixture(t, options.ConfigPath, nil)
	old := time.Date(2025, 1, 2, 3, 4, 5, 0, time.UTC)
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{{Environment: "test", Site: "", Kind: "datasource_schema", LUID: "ds-1", Name: "Sales", Coverage: "detail", ObservedAt: old}}); err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		args := []string{"cache", "status", "--environment", "test", "--json"}
		if full {
			args = append(args, "--full")
		}
		out := runGroupOneCLI(t, options, args...)
		for _, fact := range []string{`"status": "uninitialized"`, `"kind": "datasource_schema"`, `"records": 1`, `"complete": false`, `"stale": true`} {
			if !strings.Contains(out, strings.ReplaceAll(fact, ": ", ":")) {
				t.Fatalf("missing %s: %s", fact, out)
			}
		}
		if full != strings.Contains(out, "2025-01-02T03:04:05Z") {
			t.Fatalf("timestamp projection: %s", out)
		}
	}
	if requests.Load() != 0 {
		t.Fatal("local status contacted Tableau")
	}
	runGroupOneCLI(t, options, "cache", "refresh", "--environment", "test", "--scope", "projects")
	out := runGroupOneCLI(t, options, "cache", "status", "--environment", "test", "--json")
	for _, fact := range []string{`"scope": "projects"`, `"requested": true`, `"kind": "datasource_schema"`, `"stale": true`} {
		if !strings.Contains(out, strings.ReplaceAll(fact, ": ", ":")) {
			t.Fatalf("missing %s: %s", fact, out)
		}
	}
}
