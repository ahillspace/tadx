package app_test

import (
	"context"
	"github.com/ahillspace/tadx/internal/app"
	corecache "github.com/ahillspace/tadx/internal/cache"
	"strings"
	"testing"
	"time"
)

func TestCachedSearchReportsGenerationAndWorkbookThroughCLI(t *testing.T) {
	configPath := writeCLIConfigWithSite(t, "https://tableau.example.com", "marketing")
	now := time.Date(2026, 8, 30, 12, 0, 0, 0, time.UTC)
	store := targetCacheFixture(t, configPath, func() time.Time { return now })
	if _, err := store.Replace(context.Background(), corecache.Generation{
		ID: "generation-1", Environment: "production", Site: "marketing", GeneratedAt: now, Complete: true, Source: "test-fixture", Scopes: []string{"workbooks"},
		Records: []corecache.Record{{LUID: "wb-1", Kind: "workbook", Name: "Finance", ProjectPath: "Ops", Owner: "user-1"}},
	}); err != nil {
		t.Fatal(err)
	}
	options := app.Options{ConfigPath: configPath, Now: func() time.Time { return now }}

	var stdout strings.Builder
	if exit := app.Run(context.Background(), []string{"search", "--cache", "--type", "workbook", "--environment", "production"}, &stdout, options); exit != 0 {
		t.Fatalf("cache search exit = %d, output = %s", exit, stdout.String())
	}
	output := stdout.String()
	for _, want := range []string{"source: cache", "returned: 1", "id: generation-1", "wb-1", "help[1]:"} {
		if !strings.Contains(output, want) {
			t.Fatalf("cache search output missing %q: %s", want, output)
		}
	}
}
