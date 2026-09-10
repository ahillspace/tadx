package app_test

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestProjectPermissionMetadataRemainsVisibleLiveAndCatalog(t *testing.T) {
	var reads atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		if r.URL.Path != "/api/3.29/sites/site-1/projects" || r.Method != http.MethodGet {
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
			return
		}
		reads.Add(1)
		_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="3"/><projects><project id="root" name="Root" contentPermissions="LockedToProject"/><project id="child" name="Child" parentProjectId="root" contentPermissions="LockedToProject" controllingPermissionsProjectId="root"/><project id="unknown" name="Unknown" parentProjectId="root"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	for _, cached := range []bool{false, true} {
		if cached {
			runGroupOneCLI(t, options, "catalog", "refresh", "--scope", "projects", "--environment", "test")
		}
		for _, operation := range []string{"list", "inspect"} {
			args := []string{"content", "project", operation, "--environment", "test"}
			if operation == "inspect" {
				args = append(args, "--project-id", "child")
			}
			if cached {
				args = append(args, "--catalog")
			}
			before := reads.Load()
			output := runGroupOneCLI(t, options, args...)
			if !strings.Contains(output, "content_permissions") || !strings.Contains(output, "LockedToProject") || !strings.Contains(output, "controlling_permissions_project_luid") {
				t.Fatalf("permission metadata omitted (%v): %s", args, output)
			}
			if cached && reads.Load() != before {
				t.Fatal("catalog metadata performed remote reads")
			}
			if !cached && reads.Load()-before != 1 {
				t.Fatalf("project metadata needs one inventory read, got %d", reads.Load()-before)
			}
		}
	}
	output := runGroupOneCLI(t, options, "content", "project", "inspect", "--project-id", "unknown", "--environment", "test", "--catalog")
	if strings.Contains(output, "controlling_permissions_project_luid") || strings.Contains(output, "content_permissions") {
		t.Fatalf("invented inherited controller or mode: %s", output)
	}
}

func TestProjectUpdateReceiptShowsReturnedPermissionMode(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/projects":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="root" name="Root" contentPermissions="ManagedByOwner"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/projects/root":
			_, _ = io.WriteString(w, `<tsResponse><project id="root" name="Root" contentPermissions="LockedToProject" controllingPermissionsProjectId="root"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	output := runGroupOneCLI(t, diagnosticOptions(t, server), "content", "project", "update", "--project-id", "root", "--content-permissions", "LockedToProject", "--environment", "test")
	resultIndex := strings.Index(output, "result:")
	if resultIndex < 0 || !strings.Contains(output[resultIndex:], "content_permissions: LockedToProject") || !strings.Contains(output[resultIndex:], "controlling_permissions_project_luid: root") {
		t.Fatalf("compact receipt omitted returned change: %s", output)
	}
}
