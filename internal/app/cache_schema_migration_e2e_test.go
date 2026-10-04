package app_test

import (
	"context"
	"database/sql"
	"github.com/ahillspace/tadx/internal/app"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOldCacheSchemaRequiresExplicitRefreshThroughCLI(t *testing.T) {
	server, _ := slashInventoryServer(t)
	defer server.Close()
	options := cacheResilienceOptions(t, server)
	runProjectFlowCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "projects")
	cachePath := filepath.Join(filepath.Dir(options.ConfigPath), targetCacheFixture(t, options.ConfigPath, nil).RelativePath())
	db, err := sql.Open("sqlite", cachePath)
	if err != nil {
		t.Fatal(err)
	}
	_, err = db.Exec(`UPDATE catalog_schema SET version=5,signature='tadx-catalog-v5'; PRAGMA user_version=5`)
	if err != nil {
		t.Fatal(err)
	}
	db.Close()
	var out strings.Builder
	exit := app.Run(context.Background(), []string{"content", "project", "list", "--cache", "--environment", "production"}, &out, options)
	if exit == 0 || !strings.Contains(out.String(), "refresh") {
		t.Fatalf("old schema silently read or unclear recovery: exit=%d %s", exit, out.String())
	}
	before, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	runProjectFlowCLI(t, options, "cache", "refresh", "--environment", "production", "--scope", "projects")
	after, err := os.ReadFile(options.ConfigPath)
	if err != nil || string(before) != string(after) {
		t.Fatal("refresh changed configuration")
	}
	runProjectFlowCLI(t, options, "content", "project", "list", "--cache", "--environment", "production")
}
