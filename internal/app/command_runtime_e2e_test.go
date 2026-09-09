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
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkbookBatchReusesCommandSignInAndPreservesFailures(t *testing.T) {
	var signins atomic.Int32
	var requested []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			signins.Add(1)
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		requested = append(requested, r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:])
		w.WriteHeader(http.StatusNotFound)
		_, _ = io.WriteString(w, `<tsResponse><error code="404006"><summary>Workbook not found</summary></error></tsResponse>`)
	}))
	defer server.Close()
	options := catalogResilienceOptions(t, server)
	createNamedWorkspace(t, options.ConfigPath, "batch")
	args := []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "batch"}
	for i := range 100 {
		args = append(args, "--id", fmt.Sprintf("workbook-%03d", i))
	}
	var output strings.Builder
	exit := app.Run(context.Background(), args, &output, options)
	if exit == 0 || !strings.Contains(output.String(), "failed: 100") {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	if signins.Load() != 1 {
		t.Errorf("signins=%d want1 per command", signins.Load())
	}
	if len(requested) != 100 {
		t.Fatalf("resource requests=%d", len(requested))
	}
	for i, id := range requested {
		if want := fmt.Sprintf("workbook-%03d", i); id != want {
			t.Fatalf("request%d=%q want%q", i, id, want)
		}
	}
	output.Reset()
	app.Run(context.Background(), args[:9], &output, options)
	if signins.Load() != 2 {
		t.Errorf("separate command reused previous session: signins=%d", signins.Load())
	}
}

func TestWorkbookBatchDownloadsArtifactsWithOneSetupSnapshot(t *testing.T) {
	var signins, projects, downloads atomic.Int32
	var configPath string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			signins.Add(1)
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/projects"):
			projects.Add(1)
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Ops"/></projects></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/content"):
			index := downloads.Add(1)
			// An in-flight command must keep its validated setup snapshot even if
			// another actor edits configuration before the next batch item.
			if index == 1 {
				if err := os.WriteFile(configPath, []byte("invalid: ["), 0600); err != nil {
					t.Error(err)
				}
			}
			w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="Book%d.twb"`, index))
			_, _ = io.WriteString(w, `<workbook/>`)
		case strings.Contains(r.URL.Path, "/workbooks/"):
			id := r.URL.Path[strings.LastIndex(r.URL.Path, "/")+1:]
			_, _ = fmt.Fprintf(w, `<tsResponse><workbook id="%s" name="%s"><project id="project-1" name="Ops"/></workbook></tsResponse>`, id, id)
		case r.URL.Path == "/api/metadata/graphql":
			_, _ = io.WriteString(w, `{"data":{"workbooksConnection":{"totalCount":1,"nodes":[{"embeddedDatasourcesConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false,"endCursor":""},"nodes":[]}}]}}}`)
		default:
			t.Errorf("unexpected resource request: %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	options := catalogResilienceOptions(t, server)
	configPath = options.ConfigPath
	root := createNamedWorkspace(t, configPath, "batch")
	var output strings.Builder
	exit := app.Run(context.Background(), []string{"content", "workbook", "pull", "--environment", "production", "--workspace", "batch", "--id", "one", "--id", "two", "--id", "three"}, &output, options)
	if exit != 0 {
		t.Fatalf("exit=%d output=%s", exit, output.String())
	}
	if signins.Load() != 1 || projects.Load() != 1 || downloads.Load() != 3 {
		t.Fatalf("signins=%d projects=%d downloads=%d", signins.Load(), projects.Load(), downloads.Load())
	}
	entries, err := os.ReadDir(filepath.Join(root, "artifacts", "workbook"))
	if err != nil || len(entries) != 3 {
		t.Fatalf("artifacts=%d error=%v", len(entries), err)
	}
	for _, entry := range entries {
		files, err := filepath.Glob(filepath.Join(root, "artifacts", "workbook", entry.Name(), "*.twb"))
		if err != nil || len(files) != 1 {
			t.Fatalf("native workbook files=%v error=%v", files, err)
		}
		data, err := os.ReadFile(files[0])
		if err != nil || string(data) != "<workbook/>" {
			t.Fatalf("native workbook content=%q error=%v", data, err)
		}
	}
}

func TestProjectMoveUsesOneHierarchyPerValidationPhase(t *testing.T) {
	for _, drift := range []bool{false, true} {
		t.Run(fmt.Sprint(drift), func(t *testing.T) {
			var projects, writes atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
					read := projects.Add(1)
					name := "Source"
					if drift && read > 1 {
						name = "Changed"
					}
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="source" name="%s"/><project id="destination" name="Destination"/></projects></tsResponse>`, name)
				case r.Method == http.MethodPut:
					writes.Add(1)
					_, _ = io.WriteString(w, `<tsResponse><project id="source" name="Source" parentProjectId="destination"/></tsResponse>`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			options := catalogResilienceOptions(t, server)
			options.MutationsEnabled = true
			var output strings.Builder
			exit := app.Run(context.Background(), []string{"content", "project", "move", "--environment", "production", "--project-id", "source", "--parent-id", "destination"}, &output, options)
			if drift {
				if exit == 0 || writes.Load() != 0 || !strings.Contains(output.String(), "target_changed") {
					t.Fatalf("drift exit=%d writes=%d output=%s", exit, writes.Load(), output.String())
				}
			} else if exit != 0 || writes.Load() != 1 {
				t.Fatalf("exit=%d writes=%d output=%s", exit, writes.Load(), output.String())
			}
			if projects.Load() != 2 {
				t.Fatalf("hierarchy reads=%d want2 (plan and prewrite)", projects.Load())
			}
		})
	}
}

func TestContentMovesShareSourceAndDestinationHierarchyWithinPhase(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, preview := range []bool{true, false} {
			t.Run(fmt.Sprintf("%s/preview=%v", kind, preview), func(t *testing.T) {
				var projects, writes atomic.Int32
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					switch {
					case strings.HasSuffix(r.URL.Path, "/auth/signin"):
						_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
					case strings.HasSuffix(r.URL.Path, "/projects"):
						projects.Add(1)
						_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="2"/><projects><project id="source" name="Source"/><project id="destination" name="Destination"/></projects></tsResponse>`)
					case r.Method == http.MethodPut:
						writes.Add(1)
						_, _ = fmt.Fprintf(w, `<tsResponse><%s id="item" name="Item"><project id="destination" name="Destination"/><owner id="owner"/></%s></tsResponse>`, kind, kind)
					case strings.HasSuffix(r.URL.Path, "/item"):
						_, _ = fmt.Fprintf(w, `<tsResponse><%s id="item" name="Item"><project id="source" name="Source"/><owner id="owner"/></%s></tsResponse>`, kind, kind)
					case strings.HasSuffix(r.URL.Path, "/"+kind+"s"):
						_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><%ss/></tsResponse>`, kind)
					default:
						t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer server.Close()
				options := catalogResilienceOptions(t, server)
				options.MutationsEnabled = true
				args := []string{"content", kind, "move", "--environment", "production", "--id", "item", "--destination-project-id", "destination"}
				if preview {
					args = append(args, "--preview")
				}
				var output strings.Builder
				if exit := app.Run(context.Background(), args, &output, options); exit != 0 {
					t.Fatalf("exit=%d output=%s", exit, output.String())
				}
				wantReads, wantWrites := int32(2), int32(1)
				if preview {
					wantReads, wantWrites = 1, 0
				}
				if projects.Load() != wantReads || writes.Load() != wantWrites {
					t.Fatalf("hierarchy reads=%d want%d writes=%d want%d", projects.Load(), wantReads, writes.Load(), wantWrites)
				}
			})
		}
	}
}
