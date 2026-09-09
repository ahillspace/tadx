package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/config"
)

func targetCatalogFixture(t *testing.T, configPath string, now func() time.Time) *catalog.Store {
	t.Helper()
	configuration, err := config.Load(configPath)
	if err != nil {
		t.Fatal(err)
	}
	environment, err := configuration.ResolveEnvironment("")
	if err != nil {
		t.Fatal(err)
	}
	return catalog.NewTargetStore(filepath.Dir(configPath), environment.URL, environment.SiteContentURL, now)
}

func TestCatalogCLIRejectsLegacyUnboundInventory(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; http.Error(w, "unexpected network", 500) }))
	defer server.Close()
	options := diagnosticOptions(t, server)
	legacy := catalog.NewStore(filepath.Dir(options.ConfigPath), nil)
	_, err := legacy.ReplaceResourceScope(context.Background(), catalog.ResourceScopeReplacement{Environment: "test", Kind: "workbook", Source: "fixture", GeneratedAt: time.Now(), Entries: []catalog.ResourceEntry{{LUID: "other-server-workbook", Name: "Other server workbook"}}})
	if err != nil {
		t.Fatal(err)
	}
	var out strings.Builder
	exit := app.Run(context.Background(), []string{"content", "workbook", "list", "--catalog", "--environment", "test"}, &out, options)
	if exit == 0 || strings.Contains(out.String(), "other-server-workbook") || !strings.Contains(out.String(), "catalog refresh") || calls != 0 {
		t.Fatalf("exit=%d calls=%d output=%s", exit, calls, out.String())
	}
}

func TestCatalogCLIInFlightRefreshRetainsOriginalServerAfterAliasRetarget(t *testing.T) {
	started, release := make(chan struct{}), make(chan struct{})
	handler := func(name string, block bool) http.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) {
			switch {
			case strings.HasSuffix(r.URL.Path, "/auth/signin"):
				_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"same-site"},"user":{"id":"user"}}}`)
			case strings.HasSuffix(r.URL.Path, "/projects"):
				_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><projects/></tsResponse>`)
			case strings.HasSuffix(r.URL.Path, "/workbooks"):
				if block {
					close(started)
					<-release
				}
				_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><workbooks><workbook id="same-workbook" name="%s"/></workbooks></tsResponse>`, name)
			default:
				http.Error(w, "unexpected request", 500)
			}
		}
	}
	first := httptest.NewTLSServer(handler("Original server", true))
	defer first.Close()
	second := httptest.NewTLSServer(handler("Retargeted server", false))
	defer second.Close()
	options := diagnosticOptions(t, first)
	args := []string{"catalog", "refresh", "--environment", "test", "--scope", "workbooks"}
	type result struct {
		exit   int
		output string
	}
	done := make(chan result, 1)
	go func() {
		var out strings.Builder
		exit := app.Run(context.Background(), args, &out, options)
		done <- result{exit, out.String()}
	}()
	select {
	case <-started:
	case <-time.After(10 * time.Second):
		close(release)
		t.Fatal("original refresh did not start")
	}
	configBytes, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		close(release)
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ConfigPath, []byte(strings.ReplaceAll(string(configBytes), first.URL, second.URL)), 0o600); err != nil {
		close(release)
		t.Fatal(err)
	}
	var missing strings.Builder
	if code := app.Run(context.Background(), []string{"content", "workbook", "list", "--catalog", "--environment", "test"}, &missing, options); code == 0 {
		close(release)
		t.Fatalf("retarget reused old cache: %s", missing.String())
	}
	var refreshed strings.Builder
	code := app.Run(context.Background(), args, &refreshed, options)
	close(release)
	old := <-done
	if code != 0 || old.exit != 0 {
		t.Fatalf("refreshes old=%d %s new=%d %s", old.exit, old.output, code, refreshed.String())
	}
	var out strings.Builder
	code = app.Run(context.Background(), []string{"content", "workbook", "list", "--catalog", "--environment", "test"}, &out, options)
	if code != 0 || !strings.Contains(out.String(), "Retargeted server") || strings.Contains(out.String(), "Original server") {
		t.Fatalf("late publication polluted target: exit=%d %s", code, out.String())
	}
}
