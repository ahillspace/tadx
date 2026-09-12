package app_test

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestNativeAcquisitionPreviewResolvesWithoutDownloadingOrWriting(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow", "lineage"} {
		t.Run(kind, func(t *testing.T) {
			remoteKind := kind
			if kind == "lineage" {
				remoteKind = "flow"
			}
			var unexpected []string
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch r.URL.Path {
				case "/api/3.29/sites/site-1/projects":
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Shared"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
				case "/api/3.29/sites/site-1/" + remoteKind + "s/item-1":
					_, _ = fmt.Fprintf(w, `<tsResponse><%s id="item-1" name="Example" fileType="tfl"><project id="project-1" name="Shared"/></%s></tsResponse>`, remoteKind, remoteKind)
				default:
					unexpected = append(unexpected, r.Method+" "+r.URL.Path)
					http.Error(w, "unexpected acquisition request", http.StatusBadRequest)
				}
			}))
			defer server.Close()
			options := diagnosticOptions(t, server)
			root := filepath.Join(t.TempDir(), "workspace")
			runGroupOneCLI(t, options, "workspace", "create", "work", "--path", root)
			before := acquisitionSnapshot(t, root)
			args := []string{"content", kind, "pull", "--id", "item-1", "--workspace", "work", "--environment", "test", "--preview"}
			if kind == "lineage" {
				args = append(args, "--kind", "flow", "--direction", "upstream", "--depth", "3")
			}
			out := runGroupOneCLI(t, options, args...)
			if !strings.Contains(out, "status: preview") || !strings.Contains(out, "item-1") || !strings.Contains(out, "artifacts/"+kind+"/") {
				t.Fatalf("incomplete preview: %s", out)
			}
			if len(unexpected) != 0 {
				t.Fatalf("preview acquired payloads: %v", unexpected)
			}
			if after := acquisitionSnapshot(t, root); !reflect.DeepEqual(before, after) {
				t.Fatalf("preview changed workspace: before=%v after=%v", before, after)
			}
		})
	}
}

func TestPulseAcquisitionPreviewChecksCompleteBundleWithoutWriting(t *testing.T) {
	incomplete := false
	var unexpected []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/-/pulse/definitions/definition-1":
			_, _ = fmt.Fprint(w, `{"metadata":{"id":"definition-1","name":"Revenue"},"specification":{"datasource":{"id":"datasource-1"}}}`)
		case "/api/-/pulse/definitions/definition-1/metrics":
			if incomplete {
				_, _ = fmt.Fprint(w, `{"metrics":[]}`)
				return
			}
			_, _ = fmt.Fprint(w, `{"metrics":[{"id":"metric-1","definition_id":"definition-1","is_default":true}]}`)
		case "/api/-/pulse/metrics/metric-1":
			_, _ = fmt.Fprint(w, `{"id":"metric-1","definition_id":"definition-1","is_default":true,"specification":{"measurement_period":{"granularity":"GRANULARITY_BY_MONTH","range":"RANGE_CURRENT_PARTIAL"},"filters":[]}}`)
		default:
			unexpected = append(unexpected, r.Method+" "+r.URL.Path)
			http.Error(w, "unexpected acquisition request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	configuration, err := os.ReadFile(options.ConfigPath)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(options.ConfigPath, []byte(strings.ReplaceAll(string(configuration), "site_content_url: ''", "site_content_url: 'test'")), 0o600); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "workspace")
	runGroupOneCLI(t, options, "workspace", "create", "work", "--path", root)
	before := acquisitionSnapshot(t, root)
	args := []string{"pulse", "definition", "pull", "--id", "definition-1", "--workspace", "work", "--environment", "test", "--preview"}
	out := runGroupOneCLI(t, options, args...)
	if !strings.Contains(out, "status: preview") || !strings.Contains(out, "metric_count: 1") {
		t.Fatalf("incomplete preview: %s", out)
	}
	incomplete = true
	var failure strings.Builder
	if code := app.Run(context.Background(), args, &failure, options); code == 0 || !strings.Contains(failure.String(), "pulse.definition.pull.incomplete") {
		t.Fatalf("incomplete bundle accepted: %s", failure.String())
	}
	if len(unexpected) != 0 {
		t.Fatalf("unexpected calls: %v", unexpected)
	}
	if after := acquisitionSnapshot(t, root); !reflect.DeepEqual(before, after) {
		t.Fatalf("preview changed workspace: before=%v after=%v", before, after)
	}
}

func acquisitionSnapshot(t *testing.T, root string) map[string]string {
	t.Helper()
	result := map[string]string{}
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		if entry.IsDir() {
			result[rel] = "directory"
			return nil
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		result[rel] = string(data)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return result
}
