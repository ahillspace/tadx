package app

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/operationrun"
	"github.com/ahillspace/tadx/internal/value"
)

func TestPublicationPreparationRejectsUnsupportedOrMismatchedSnapshots(t *testing.T) {
	valid := publicationPreparationSnapshot{Version: 1, Operation: "workbook.publish", Destinations: []value.PublishDestination{{Environment: "production", Site: "site", ProjectLUID: "project", Name: "Destination"}}, Scopes: []publicationTargetScope{{Environment: "production", Site: "site", Server: "https://tableau.example.com", SiteLUID: "site-1"}}}
	data, err := json.Marshal(valid)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := decodePublicationPreparation(data, valid.Operation); err != nil {
		t.Fatal(err)
	}
	if targets, err := decodePublicationPreparation(nil, valid.Operation); err != nil || targets.Destinations != nil || targets.Scopes != nil {
		t.Fatalf("legacy preparation=%v error=%v", targets, err)
	}
	for _, invalid := range []string{
		strings.Replace(string(data), `"version":1`, `"version":2`, 1),
		strings.Replace(string(data), `"workbook.publish"`, `"flow.publish"`, 1),
		strings.Replace(string(data), `"project_luid":"project"`, `"project_luid":""`, 1),
		strings.Replace(string(data), `"version":1`, `"version":1,"unknown":true`, 1),
		strings.Replace(string(data), `"site_luid":"site-1"`, `"site_luid":""`, 1),
		strings.Replace(string(data), `"server":"https://tableau.example.com"`, `"server":"not-a-server"`, 1),
		`{"version":1,"operation":"workbook.publish","destinations":[]}`,
		string(data) + `{}`,
	} {
		if _, err := decodePublicationPreparation([]byte(invalid), valid.Operation); err == nil {
			t.Fatalf("invalid preparation accepted: %s", invalid)
		}
	}
}

func TestPublicationScopeUsesCanonicalServerAndExactAuthenticatedSite(t *testing.T) {
	expected := publicationTargetScope{Environment: "production", Site: "team", Server: "https://tableau.example.com/Prefix", SiteLUID: "site-1"}
	for _, scenario := range []struct {
		name, server, siteURL, siteLUID string
		allowed                         bool
	}{
		{"equivalent spelling", "https://TABLEAU.example.com:443/Prefix/", "team", "site-1", true},
		{"changed server", "https://other.example.com/Prefix", "team", "site-1", false},
		{"changed base path", "https://tableau.example.com/prefix", "team", "site-1", false},
		{"changed site URL", "https://tableau.example.com/Prefix", "other", "site-1", false},
		{"changed site LUID", "https://tableau.example.com/Prefix", "team", "site-2", false},
	} {
		t.Run(scenario.name, func(t *testing.T) {
			ctx := withPublicationScopes(t.Context(), []publicationTargetScope{expected})
			environment := config.Environment{Alias: "production", URL: scenario.server, SiteContentURL: scenario.siteURL}
			err := bindPublicationScope(ctx, "workbook.publish", environment, scenario.siteLUID)
			if (err == nil) != scenario.allowed {
				t.Fatalf("allowed=%t error=%v", scenario.allowed, err)
			}
		})
	}
}

func TestPublicationWorkerRetainsForegroundDestinationGeneration(t *testing.T) {
	for _, scenario := range []struct {
		name, original string
		changedSite    bool
	}{{"expected-absence", "", false}, {"existing-generation", "original", false}, {"authenticated-site", "original", true}} {
		t.Run(scenario.name, func(t *testing.T) {
			original := scenario.original
			var changed atomic.Bool
			var writes atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					site := "site-1"
					if scenario.changedSite && changed.Load() {
						site = "site-2"
					}
					_, _ = fmt.Fprintf(w, `{"credentials":{"token":"session-token","site":{"id":%q},"user":{"id":"user-1"}}}`, site)
				case strings.HasSuffix(r.URL.Path, "/projects"):
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
					id := original
					if changed.Load() && !scenario.changedSite {
						id = "replacement"
					}
					if id == "" {
						_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><workbooks/></tsResponse>`)
					} else {
						_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><workbooks><workbook id="`+id+`" name="Destination"><project id="project-1"/></workbook></workbooks></tsResponse>`)
					}
				case r.Method == http.MethodPost:
					writes.Add(1)
					http.Error(w, "publication must not reach a write", http.StatusBadRequest)
				default:
					t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			runtime, _ := datasourceLifecycleRuntime(t, server)
			file := filepath.Join(t.TempDir(), "Source.twb")
			if err := os.WriteFile(file, []byte("<workbook/>"), 0600); err != nil {
				t.Fatal(err)
			}
			batch, err := json.Marshal(map[string]any{"items": []map[string]string{{"file": file, "name": "Destination"}}})
			if err != nil {
				t.Fatal(err)
			}
			batchPath := filepath.Join(t.TempDir(), "batch.json")
			if err := os.WriteFile(batchPath, batch, 0600); err != nil {
				t.Fatal(err)
			}
			options := publicationWorkerTestOptions(runtime.configPath, server, t)
			done := make(chan int, 1)
			var acceptedID string
			options.WorkerLauncher = func(ctx context.Context, directory, id string) error {
				acceptedID = id
				record, err := (operationrun.Store{Directory: directory}).Read(id)
				if err != nil {
					return err
				}
				preparation, err := decodePublicationPreparation(record.Request.PreparationData, record.Operation)
				targets := preparation.Destinations
				if err != nil || len(targets) != 1 || targets[0].ExistingLUID != original {
					t.Errorf("accepted targets=%v error=%v", targets, err)
				}
				changed.Store(true)
				return launchInProcessPublicationWorkerTest(ctx, directory, id, options, done)
			}
			var output strings.Builder
			Run(t.Context(), []string{"content", "workbook", "publish", "--environment", "production", "--project-id", "project-1", "--overwrite", "--batch-file", batchPath, "--no-wait", "--json"}, &output, options)
			select {
			case exit := <-done:
				if exit == 0 {
					t.Fatal("worker replaced a different destination generation")
				}
			case <-time.After(5 * time.Second):
				t.Fatalf("worker did not finish: %s", output.String())
			}
			record, err := (operationrun.Store{Directory: options.OperationDirectory}).Read(acceptedID)
			failureText := "destination changed after batch preparation"
			if scenario.changedSite {
				failureText = "authenticated publication target changed"
			}
			if err != nil || writes.Load() != 0 || record.Phase != operationrun.PhaseFailed || !strings.Contains(string(record.FullResult), failureText) {
				t.Fatalf("writes=%d record=%+v error=%v", writes.Load(), record, err)
			}
		})
	}
}
