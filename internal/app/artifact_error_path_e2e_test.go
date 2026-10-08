package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestArtifactWriteFailureDoesNotRenderAbsoluteWorkspacePaths(t *testing.T) {
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("requires POSIX directory permissions enforced for a non-root user")
	}
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.URL.Path == "/api/3.29/sites/site-1/projects":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Shared"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
		case r.URL.Path == "/api/3.29/sites/site-1/workbooks/item-1/content":
			w.Header().Set("Content-Disposition", `attachment; filename="Example.twb"`)
			_, _ = io.WriteString(w, "<workbook/>")
		case r.URL.Path == "/api/3.29/sites/site-1/workbooks/item-1":
			_, _ = io.WriteString(w, `<tsResponse><workbook id="item-1" name="Example"><project id="project-1" name="Shared"/></workbook></tsResponse>`)
		default:
			http.Error(w, "metadata unavailable", http.StatusForbidden)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	workspaceRoot := filepath.Join(t.TempDir(), "workspace")
	runGroupOneCLI(t, options, "workspace", "create", "work", "--path", workspaceRoot)
	locked := filepath.Join(workspaceRoot, "artifacts", "workbook")
	if err := os.MkdirAll(locked, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(locked, 0o500); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(locked, 0o700) })
	for _, format := range [][]string{nil, {"--json"}} {
		var out strings.Builder
		args := append([]string{"content", "workbook", "pull", "--id", "item-1", "--workspace", "work", "--environment", "test"}, format...)
		code := app.Run(context.Background(), args, &out, options)
		if code == 0 {
			t.Fatalf("%v succeeded against a read-only artifact directory:\n%s", format, out.String())
		}
		if !strings.Contains(out.String(), "workbook.pull.write") || !strings.Contains(out.String(), "mkdir artifacts/workbook/.tadx-workbook-stage-") {
			t.Fatalf("%v lost the workspace-relative cause:\n%s", format, out.String())
		}
		for _, private := range []string{workspaceRoot, filepath.ToSlash(workspaceRoot), os.TempDir()} {
			if private != "" && strings.Contains(out.String(), private) {
				t.Fatalf("%v rendered an absolute path %q:\n%s", format, private, out.String())
			}
		}
	}
}
