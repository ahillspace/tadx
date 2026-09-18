package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	userlist "github.com/ahillspace/tadx/actions/admin/user/list"
	"github.com/ahillspace/tadx/internal/managedpolicy"
)

type indirectTestPolicy struct{ denied map[string]bool }

func (p indirectTestPolicy) Status() managedpolicy.Status {
	return managedpolicy.Status{State: managedpolicy.StateActive, Protected: true, CandidateValid: true, RemoteMutations: true}
}
func (p indirectTestPolicy) CheckCapability(id string) error {
	if p.denied[id] {
		return managedpolicy.ErrCapabilityDenied
	}
	return nil
}
func (indirectTestPolicy) CheckRemoteMutation() error { return nil }

func TestManagedDeniedConfirmationPreservesAcknowledgedCategoryCreate(t *testing.T) {
	var reads, writes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/labelCategories") && r.Method == http.MethodGet:
			reads.Add(1)
			io.WriteString(w, `<tsResponse><labelCategoryList/></tsResponse>`)
		case strings.HasSuffix(r.URL.Path, "/labelCategories") && r.Method == http.MethodPost:
			writes.Add(1)
			io.WriteString(w, `<tsResponse><labelCategory name="Custom" description="Reviewed"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected request", 500)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	options := withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), managedPolicy: indirectTestPolicy{map[string]bool{"admin.label.category.inspect": true}}}, true)
	var out bytes.Buffer
	code := Run(t.Context(), []string{"admin", "label-category", "create", "--env", "production", "--name", "Custom", "--description", "Reviewed", "--json"}, &out, options)
	var document struct {
		Error struct {
			ID       string `json:"id"`
			Phase    string `json:"phase"`
			Outcome  string `json:"outcome"`
			Resource string `json:"resource"`
			Cause    string `json:"upstream_cause"`
		} `json:"error"`
		Output struct {
			Result struct {
				Status string `json:"status"`
				Item   struct {
					Name string `json:"name"`
				} `json:"item"`
				Completed []string `json:"completed"`
			} `json:"result"`
		} `json:"output"`
	}
	if err := json.Unmarshal(out.Bytes(), &document); err != nil {
		t.Fatal(err)
	}
	if code != 1 || reads.Load() != 2 || writes.Load() != 1 {
		t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads.Load(), writes.Load(), &out)
	}
	if document.Error.ID != "admin.label.category.create.verification" || document.Error.Phase != "verification" || document.Error.Outcome != "confirmed" || document.Error.Resource != "Custom" || !strings.Contains(document.Error.Cause, "administrator-managed policy") {
		t.Fatalf("acknowledged mutation downgraded to setup: %s", &out)
	}
	result := document.Output.Result
	if result.Status != "verification_pending" || result.Item.Name != "Custom" || len(result.Completed) != 1 || result.Completed[0] != "definition_write" {
		t.Fatalf("acknowledged identity or completion lost: %s", &out)
	}
}

func TestManagedIndirectCLIStopsBeforeForbiddenRequests(t *testing.T) {
	for _, tc := range []struct {
		name   string
		args   []string
		denied string
	}{
		{"cache-default", []string{"cache", "refresh"}, "admin.user.list"},
		{"cache-users", []string{"cache", "refresh", "--scope", "users"}, "admin.user.list"},
		{"cache-groups", []string{"cache", "refresh", "--scope", "groups"}, "admin.group.list"},
		{"cache-permissions", []string{"cache", "refresh", "--scope", "permissions"}, "admin.permission.inspect"},
		{"search-live", []string{"search", "--type", "user"}, "admin.user.list"},
		{"search-cache", []string{"search", "--type", "group", "--cache"}, "admin.group.list"},
		{"content-label-preview", []string{"catalog", "label", "update", "--type", "table", "--target-id", "table-1", "--value", "Warning", "--message", "After", "--preview"}, "admin.label.value.inspect"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			runtime, _ := datasourceLifecycleRuntime(t, server)
			options := Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), UserHomeDir: func() (string, error) { return t.TempDir(), nil }, managedPolicy: indirectTestPolicy{map[string]bool{tc.denied: true}}}
			var output bytes.Buffer
			args := append(tc.args, "--env", "production", "--json")
			code := Run(t.Context(), args, &output, options)
			if code == 0 || !strings.Contains(output.String(), "policy.denied") || calls.Load() != 0 {
				t.Fatalf("exit=%d calls=%d output=%s", code, calls.Load(), output.String())
			}
		})
	}
}

func TestManagedCacheNonAdminScopeRemainsUsable(t *testing.T) {
	var forbidden atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/3.29/auth/signin":
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/workbooks":
			_, _ = io.WriteString(w, cacheListXML("workbooks", "workbook", ""))
		case "/api/3.29/sites/site-1/projects":
			_, _ = io.WriteString(w, cacheListXML("projects", "project", ""))
		default:
			forbidden.Add(1)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	options := Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), UserHomeDir: func() (string, error) { return t.TempDir(), nil }, managedPolicy: indirectTestPolicy{map[string]bool{"admin.user.list": true, "admin.group.list": true, "admin.permission.inspect": true}}}
	var output bytes.Buffer
	code := Run(t.Context(), []string{"cache", "refresh", "--scope", "workbooks", "--env", "production", "--json"}, &output, options)
	if code != 0 || forbidden.Load() != 0 {
		t.Fatalf("exit=%d forbidden=%d output=%s", code, forbidden.Load(), output.String())
	}
}

func TestManagedCachedAdminReadersCheckBeforeStoreAccess(t *testing.T) {
	denied := errors.New("denied before cache access")
	reader := &cacheUserListReader{checkCapability: func(string) error { return denied }}
	_, err := reader.ListUsers(t.Context(), userlist.PageRequest{PageNumber: 1, PageSize: 100})
	if !errors.Is(err, denied) {
		t.Fatalf("error=%v", err)
	}
}

func TestManagedPublishSkipsDeniedOptionalRoleLookup(t *testing.T) {
	var userReads, publishes atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.Contains(r.URL.Path, "/users/"):
			userReads.Add(1)
			w.WriteHeader(http.StatusForbidden)
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Analytics" topLevelProject="true"/></projects></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/workbooks"):
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="0"/><workbooks/></tsResponse>`, r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/validateWorkbook"):
			_, _ = io.WriteString(w, `{"errors":[],"warnings":[]}`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/workbooks"):
			publishes.Add(1)
			if r.URL.Query().Get("asJob") == "true" {
				t.Error("denied role lookup enabled asynchronous publication")
			}
			w.WriteHeader(http.StatusCreated)
			_, _ = io.WriteString(w, `<tsResponse><workbook id="created-1" name="Native"><project id="project-1"/></workbook></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	file := filepath.Join(t.TempDir(), "Native.twb")
	if err := os.WriteFile(file, []byte("<workbook/>"), 0600); err != nil {
		t.Fatal(err)
	}
	options := withSiteMutationConsent(t, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), JobDirectory: t.TempDir(), managedPolicy: indirectTestPolicy{map[string]bool{"admin.user.inspect": true}}}, true)
	var output bytes.Buffer
	code := Run(t.Context(), []string{"content", "workbook", "publish", "--file", file, "--environment", "production", "--project-id", "project-1", "--json"}, &output, options)
	if code != 0 || userReads.Load() != 0 || publishes.Load() != 1 {
		t.Fatalf("exit=%d userReads=%d publishes=%d output=%s", code, userReads.Load(), publishes.Load(), output.String())
	}
}

func TestManagedAdminUsernamePreflightDeniesBeforeInventory(t *testing.T) {
	var inventory atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/auth/signin") {
			_, _ = io.WriteString(w, `{"credentials":{"token":"test-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			return
		}
		inventory.Add(1)
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	runtime, _ := datasourceLifecycleRuntime(t, server)
	var output bytes.Buffer
	code := Run(t.Context(), []string{"admin", "user", "update", "--username", "alice", "--site-role", "Viewer", "--preview", "--env", "production", "--json"}, &output, Options{ConfigPath: runtime.configPath, HTTPClient: server.Client(), managedPolicy: indirectTestPolicy{map[string]bool{"admin.user.inspect": true}}})
	if code == 0 || inventory.Load() != 0 || !strings.Contains(output.String(), "policy.denied") {
		t.Fatalf("exit=%d inventory=%d output=%s", code, inventory.Load(), output.String())
	}
}
