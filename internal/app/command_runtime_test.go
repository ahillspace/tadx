package app

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/identity"
)

func TestCommandReadPhaseReusesProjectIndexButWriteChecksAreFresh(t *testing.T) {
	var signins, projects atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			signins.Add(1)
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/workbooks/book":
			_, _ = io.WriteString(w, `<tsResponse><workbook id="book" name="Book"><project id="project"/></workbook></tsResponse>`)
		case "/api/3.29/sites/site-1/projects":
			version := projects.Add(1)
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project" name="Version%d"/></projects></tsResponse>`, version)
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	commands := newRemoteContentCommands(runtime)
	ctx := context.Background()
	for _, step := range []struct {
		write bool
		want  string
	}{{false, "Version1"}, {false, "Version1"}, {true, "Version2"}, {true, "Version3"}} {
		connection, err := commands.connect(ctx, "production", step.write)
		if err != nil {
			t.Fatal(err)
		}
		item, err := connection.workbooks.ResolveWorkbook(ctx, identity.Selector{LUID: "book"})
		if err != nil || item.ProjectPath != step.want {
			t.Fatalf("write=%v project=%q want=%q error=%v", step.write, item.ProjectPath, step.want, err)
		}
	}
	if signins.Load() != 1 || projects.Load() != 3 {
		t.Fatalf("signins=%d project reads=%d", signins.Load(), projects.Load())
	}
	first, err := runtime.tableauConnection(ctx, "production", false)
	if err != nil {
		t.Fatal(err)
	}
	second, err := runtime.tableauConnection(ctx, "production", true)
	if err != nil {
		t.Fatal(err)
	}
	if first.transport != second.transport || first.session != second.session || runtime.clients(first) != runtime.clients(second) {
		t.Fatal("command setup was rebuilt")
	}
	if _, err := runtime.tableauConnection(ctx, "", true); err == nil {
		t.Fatal("shared setup bypassed explicit mutation target requirement")
	}
}
