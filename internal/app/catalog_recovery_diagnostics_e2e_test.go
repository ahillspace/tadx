package app_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"regexp"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/catalog"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestCatalogReadDiagnosticRepairPopulatesRequiredScopeThroughCLI(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "project", "user", "group"} {
		t.Run(kind, func(t *testing.T) {
			server, requests := catalogRecoveryServer(t, kind)
			defer server.Close()
			options := diagnosticOptions(t, server)
			root := "content"
			if kind == "user" || kind == "group" {
				root = "admin"
			}
			args := []string{root, kind, "list", "--environment", "test", "--catalog"}
			failure := catalogDiagnosticFailure(t, options, args)
			if failure.ID != "catalog.uninitialized" || requests.Load() != 0 {
				t.Fatalf("uninitialized read: %#v; requests=%d", failure, requests.Load())
			}
			repair := catalogDiagnosticRepair(failure.CorrectiveAction)
			if len(repair) == 0 && strings.Contains(failure.CorrectiveAction, "without --catalog") {
				// Follow the old recommendation as an end user would: its live
				// answer succeeds, but the original local read still fails.
				repair = args[:len(args)-1]
			}
			if len(repair) == 0 {
				t.Fatalf("no executable recovery in %#v", failure)
			}
			runGroupOneCLI(t, options, repair...)
			before := requests.Load()
			if before == 0 {
				t.Fatal("repair did not collect the requested inventory")
			}
			result := runGroupOneCLI(t, options, args...)
			if requests.Load() != before || !strings.Contains(result, kind+"-1") {
				t.Fatalf("repaired local read: requests=%d/%d output=%s", before, requests.Load(), result)
			}
			if strings.Join(repair, " ") != "catalog refresh --environment test --scope "+kind+"s" {
				t.Fatalf("repair expands beyond the required scope: %v", repair)
			}
		})
	}
}

func TestOldCatalogReadDiagnosticOffersExecutableScopedRebuildThroughCLI(t *testing.T) {
	server, requests := catalogRecoveryServer(t, "project")
	defer server.Close()
	options := diagnosticOptions(t, server)
	runGroupOneCLI(t, options, "catalog", "refresh", "--environment", "test", "--scope", "projects")
	db, err := sql.Open("sqlite", filepath.Join(filepath.Dir(options.ConfigPath), catalog.DatabasePath()))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(`UPDATE catalog_schema SET version=6,signature='tadx-catalog-v6'; PRAGMA user_version=6`); err != nil {
		db.Close()
		t.Fatal(err)
	}
	if err := db.Close(); err != nil {
		t.Fatal(err)
	}
	args := []string{"content", "project", "list", "--environment", "test", "--catalog"}
	before := requests.Load()
	failure := catalogDiagnosticFailure(t, options, args)
	if failure.ID != "catalog.schema_refresh_required" || requests.Load() != before {
		t.Fatalf("old-schema diagnostic: %#v; requests=%d/%d", failure, before, requests.Load())
	}
	repair := catalogDiagnosticRepair(failure.CorrectiveAction)
	if strings.Join(repair, " ") != "catalog refresh --environment test --scope projects" {
		t.Fatalf("schema rebuild is not actionable for this scope: %q", failure.CorrectiveAction)
	}
	runGroupOneCLI(t, options, repair...)
	before = requests.Load()
	runGroupOneCLI(t, options, args...)
	if requests.Load() != before {
		t.Fatal("repaired cached read contacted Tableau")
	}
}

func TestProjectionCacheMissDoesNotSuggestUnrelatedInventoryRefreshThroughCLI(t *testing.T) {
	for _, args := range [][]string{
		{"content", "datasource", "schema", "--id", "datasource-1"},
		{"pulse", "definition", "list"},
		{"pulse", "metric", "list", "--definition-id", "definition-1"},
	} {
		t.Run(strings.Join(args[:3], "-"), func(t *testing.T) {
			server, requests := catalogRecoveryServer(t, "project")
			defer server.Close()
			options := diagnosticOptions(t, server)
			args = append(args, "--environment", "test", "--catalog")
			failure := catalogDiagnosticFailure(t, options, args)
			if strings.Contains(failure.CorrectiveAction, "tadx catalog refresh") || !strings.Contains(failure.CorrectiveAction, "without --catalog") || requests.Load() != 0 {
				t.Fatalf("projection recovery suggests an unrelated inventory repair: %#v", failure)
			}
		})
	}
}

func catalogDiagnosticFailure(t *testing.T, options app.Options, args []string) errs.Payload {
	t.Helper()
	var output strings.Builder
	if exit := app.Run(context.Background(), args, &output, options); exit == 0 {
		t.Fatalf("expected cached read failure: %s", output.String())
	}
	decoded, err := toon.Decode([]byte(output.String()))
	if err != nil {
		t.Fatal(err)
	}
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var envelope errs.Envelope
	if err := json.Unmarshal(encoded, &envelope); err != nil {
		t.Fatal(err)
	}
	return envelope.Error
}

func catalogDiagnosticRepair(advice string) []string {
	command := regexp.MustCompile(`tadx (catalog refresh --environment [a-zA-Z0-9_-]+ --scope [a-z]+(?:,[a-z]+)*)`).FindStringSubmatch(advice)
	if len(command) != 2 {
		return nil
	}
	return strings.Fields(command[1])
}

func catalogRecoveryServer(t *testing.T, kind string) (*httptest.Server, *atomic.Int32) {
	t.Helper()
	requests := &atomic.Int32{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		if diagnosticSignIn(w, r) {
			return
		}
		resource := kind
		if strings.HasSuffix(r.URL.Path, "/projects") {
			resource = "project"
		} else if !strings.HasSuffix(r.URL.Path, "/"+kind+"s") {
			t.Errorf("repair requested unrelated resource %s", r.URL.Path)
			http.Error(w, "unrelated inventory scope", http.StatusNotFound)
			return
		}
		_, _ = io.WriteString(w, `<tsResponse>`)
		_, _ = fmt.Fprintf(w, `<pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><%ss><%s id="%s-1" name="Recovery" siteRole="Viewer" type="hyper" fileType="tflx"><project id="project-1" name="Recovery"/><owner id="user-1"/></%s></%ss></tsResponse>`, r.URL.Query().Get("pageSize"), resource, resource, resource, resource, resource)
	}))
	return server, requests
}
