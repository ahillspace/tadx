package app_test

import (
	"database/sql"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestCacheRefreshCannotAcknowledgeInconsistentSchemaMarkers(t *testing.T) {
	for _, damage := range []string{
		`UPDATE catalog_schema SET version=6,signature='tadx-catalog-v6'`,
		`PRAGMA user_version=6`,
		`UPDATE catalog_schema SET signature='foreign-signature'`,
	} {
		t.Run(damage, func(t *testing.T) {
			server, _ := cacheRecoveryServer(t, "project")
			defer server.Close()
			options := diagnosticOptions(t, server)
			args := []string{"cache", "refresh", "--environment", "test", "--scope", "projects"}
			runGroupOneCLI(t, options, args...)
			path := filepath.Join(filepath.Dir(options.ConfigPath), targetCacheFixture(t, options.ConfigPath, nil).RelativePath())
			db, err := sql.Open("sqlite", path)
			if err != nil {
				t.Fatal(err)
			}
			if _, err := db.Exec(damage); err != nil {
				t.Fatal(err)
			}
			if err := db.Close(); err != nil {
				t.Fatal(err)
			}
			for _, command := range [][]string{args, {"content", "project", "list", "--environment", "test", "--cache"}} {
				var out strings.Builder
				if exit := app.Run(t.Context(), command, &out, options); exit == 0 || !strings.Contains(out.String(), "schema markers") {
					t.Fatalf("command=%v exit=%d output=%s", command, exit, out.String())
				}
				if failure := decodeDiagnosticFailure(t, out.String()); strings.Contains(failure.CorrectiveAction, "cache refresh") {
					t.Fatalf("circular recovery advice: %s", failure.CorrectiveAction)
				}
			}
		})
	}
}
