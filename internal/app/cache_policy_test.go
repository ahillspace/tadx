package app

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/inventory"
	"github.com/ahillspace/tadx/internal/lastcommand"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	tableaucache "github.com/ahillspace/tadx/internal/tableau/cache"
)

func TestCacheRefreshSavedPrerequisites(t *testing.T) {
	for _, test := range []struct {
		name, scopes, status string
		permissionStatus     int
		emptyWorkbooks       bool
		prerequisites        []string
		collections          []string
	}{
		{"default", "", "refreshed", 200, false, []string{"admin.group.list", "admin.permission.inspect", "admin.user.list"}, []string{"datasources", "flows", "groups", "permissions", "projects", "users", "views", "workbooks"}},
		{"permission dependency closure", "permissions", "refreshed", 200, false, []string{"admin.permission.inspect"}, []string{"permissions", "projects", "workbooks"}},
		{"users and groups", "users,groups", "refreshed", 200, false, []string{"admin.group.list", "admin.user.list"}, []string{"groups", "users"}},
		{"content dependency closure", "views", "refreshed", 200, false, nil, []string{"projects", "views", "workbooks"}},
		{"default empty workbooks", "", "refreshed", 200, true, []string{"admin.group.list", "admin.permission.inspect", "admin.user.list"}, []string{"datasources", "flows", "groups", "projects", "users", "views", "workbooks"}},
		{"explicit empty workbooks", "permissions", "refreshed", 200, true, []string{"admin.permission.inspect"}, []string{"projects", "workbooks"}},
		{"default partial permissions", "", "partial", 403, false, []string{"admin.group.list", "admin.permission.inspect", "admin.user.list"}, []string{"datasources", "flows", "groups", "permissions", "projects", "users", "views", "workbooks"}},
		{"explicit partial permissions", "permissions", "partial", 403, false, []string{"admin.permission.inspect"}, []string{"permissions", "projects", "workbooks"}},
		{"default fatal permissions", "", "", 401, false, []string{"admin.group.list", "admin.permission.inspect", "admin.user.list"}, []string{"datasources", "flows", "groups", "permissions", "projects", "users", "views", "workbooks"}},
		{"explicit fatal permissions", "permissions", "", 401, false, []string{"admin.permission.inspect"}, []string{"permissions", "projects", "workbooks"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			var mu sync.Mutex
			seen := map[string]bool{}
			requests := 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				mu.Lock()
				requests++
				mu.Unlock()
				switch {
				case strings.HasSuffix(r.URL.Path, "/auth/signin"):
					io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
				case strings.HasSuffix(r.URL.Path, "/auth/signout"):
					w.WriteHeader(http.StatusNoContent)
				default:
					collection := path.Base(r.URL.Path)
					mu.Lock()
					seen[collection] = true
					mu.Unlock()
					if collection == "permissions" {
						if test.permissionStatus != http.StatusOK {
							w.WriteHeader(test.permissionStatus)
							fmt.Fprintf(w, `<tsResponse><error code="%d004"><summary>Permission request failed</summary></error></tsResponse>`, test.permissionStatus)
							return
						}
						io.WriteString(w, `<tsResponse><permissions><workbook id="workbook-1"/><granteeCapabilities><user id="user-1"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`)
						return
					}
					rows := ""
					if collection == "workbooks" && !test.emptyWorkbooks {
						rows = `<workbook id="workbook-1" name="Fixture workbook"/>`
					}
					io.WriteString(w, cacheListXML(collection, "", rows))
				}
			}))
			defer server.Close()
			options := cachePolicyOptions(t, server.URL)
			options.HTTPClient = server.Client()
			options.managedPolicy = cacheFixturePolicy()
			t.Setenv("TADX_DEV_PAT_NAME", "fixture-name")
			t.Setenv("TADX_DEV_PAT_SECRET", "fixture-secret")
			args := []string{"cache", "refresh", "--environment", "dev", "--json"}
			if test.scopes != "" {
				args = append(args, "--scope", test.scopes)
			}
			var out bytes.Buffer
			code := Run(t.Context(), args, &out, options)
			if (code == 0) != (test.status != "") || (test.status != "" && !strings.Contains(out.String(), `"status":"`+test.status+`"`)) {
				t.Fatalf("refresh code=%d output=%s", code, &out)
			}
			mu.Lock()
			collections := slices.Sorted(maps.Keys(seen))
			before := requests
			mu.Unlock()
			if !slices.Equal(collections, test.collections) {
				t.Fatalf("collections=%v want=%v", collections, test.collections)
			}
			store := lastcommand.Store{Path: filepath.Join(filepath.Dir(options.ConfigPath), "last-result.json")}
			record, err := store.Read(t.Context())
			if err != nil {
				t.Fatal(err)
			}
			want := append(slices.Clone(test.prerequisites), "cache.refresh")
			if !slices.Equal(record.RequiredCapabilities, want) || record.ExitCode != code {
				t.Fatalf("saved prerequisites=%v exit=%d want=%v/%d", record.RequiredCapabilities, record.ExitCode, want, code)
			}
			for _, denied := range test.prerequisites {
				policy := cacheFixturePolicy()
				delete(policy.allowed, denied)
				options.managedPolicy = policy
				out.Reset()
				if code := Run(t.Context(), []string{"last", "--json"}, &out, options); code == 0 || !strings.Contains(out.String(), "last.unavailable") || !strings.Contains(out.String(), "administrator-managed policy blocks") || strings.Contains(out.String(), `"result"`) {
					t.Fatalf("last allowed %s: code=%d output=%s", denied, code, &out)
				}
			}
			mu.Lock()
			after := requests
			mu.Unlock()
			if after != before {
				t.Fatalf("last made remote requests: before=%d after=%d", before, after)
			}
		})
	}
}

func TestCacheRefreshPolicyErrorOrdering(t *testing.T) {
	for _, test := range []struct {
		name, environment, scopes, errorID string
		preview                            bool
		prerequisites                      []string
	}{
		{"default invalid environment", "missing", "", "cache.refresh.setup", false, []string{"admin.group.list", "admin.user.list", "cache.refresh"}},
		{"explicit invalid environment", "missing", "permissions", "policy.denied", false, []string{"cache.refresh"}},
		{"default valid environment", "dev", "", "cache.refresh.failed", false, []string{"admin.group.list", "admin.user.list", "cache.refresh"}},
		{"explicit valid environment", "dev", "permissions", "policy.denied", false, []string{"cache.refresh"}},
		{"invalid scopes before environment", "missing", "unknown", "cache.refresh.usage", false, nil},
		{"duplicate scopes before environment", "missing", "users,users", "cache.refresh.usage", false, nil},
		{"default preview", "dev", "", "", true, []string{"cache.refresh"}},
		{"explicit preview", "dev", "permissions", "", true, []string{"cache.refresh"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := cachePolicyOptions(t, "https://unused.example")
			policy := cacheFixturePolicy()
			delete(policy.allowed, "admin.permission.inspect")
			options.managedPolicy = policy
			args := []string{"cache", "refresh", "--environment", test.environment, "--json"}
			if test.scopes != "" {
				args = append(args, "--scope", test.scopes)
			}
			if test.preview {
				args = append(args, "--preview")
			}
			var out bytes.Buffer
			code := Run(t.Context(), args, &out, options)
			var result struct {
				Error struct {
					ID string `json:"id"`
				} `json:"error"`
				Status string `json:"status"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatalf("decode %s: %v", &out, err)
			}
			if result.Error.ID != test.errorID || (code == 0) != test.preview || (test.preview && result.Status != "preview") {
				t.Fatalf("code=%d output=%s want=%s", code, &out, test.errorID)
			}
			store := lastcommand.Store{Path: filepath.Join(filepath.Dir(options.ConfigPath), "last-result.json")}
			record, err := store.Read(t.Context())
			if err != nil || !slices.Equal(record.RequiredCapabilities, test.prerequisites) {
				t.Fatalf("saved prerequisites=%v want=%v error=%v", record.RequiredCapabilities, test.prerequisites, err)
			}
		})
	}
}

func cacheFixturePolicy() fixtureManagedPolicy {
	return fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{
		"cache.refresh": true, "last": true, "admin.user.list": true, "admin.group.list": true, "admin.permission.inspect": true,
	}}
}

func TestCacheExecutorRetainsAdministrativeAuthorization(t *testing.T) {
	for _, test := range []struct {
		scope tableaucache.Scope
		id    string
	}{
		{tableaucache.ScopeUsers, "admin.user.list"},
		{tableaucache.ScopeGroups, "admin.group.list"},
		{tableaucache.ScopePermissions, "admin.permission.inspect"},
	} {
		t.Run(string(test.scope), func(t *testing.T) {
			denied := errors.New("administrative inventory denied")
			executor := inventory.AuthorizedExecutor{CheckScope: func(id string) error {
				if id != test.id {
					t.Fatalf("capability=%q want=%q", id, test.id)
				}
				return denied
			}}
			if _, err := executor.Do(t.Context(), tableaucache.Request{Scope: test.scope}); !errors.Is(err, denied) {
				t.Fatalf("authorization must precede transport use: %v", err)
			}
		})
	}
}

type oneTimeCacheScopePolicy struct {
	fixtureManagedPolicy
	mu         sync.Mutex
	userChecks int
}

func (p *oneTimeCacheScopePolicy) CheckCapability(id string) error {
	if id == "admin.user.list" {
		p.mu.Lock()
		p.userChecks++
		checks := p.userChecks
		p.mu.Unlock()
		if checks > 1 {
			return managedpolicy.ErrCapabilityDenied
		}
	}
	return p.fixtureManagedPolicy.CheckCapability(id)
}

func TestCacheRefreshDoesNotRecheckScopePolicyDuringCollection(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			_, _ = io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case strings.HasSuffix(r.URL.Path, "/auth/signout"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/users"):
			_, _ = io.WriteString(w, cacheListXML("users", "", ""))
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer server.Close()
	options := cachePolicyOptions(t, server.URL)
	options.HTTPClient = server.Client()
	policy := &oneTimeCacheScopePolicy{fixtureManagedPolicy: cacheFixturePolicy()}
	options.managedPolicy = policy
	t.Setenv("TADX_DEV_PAT_NAME", "fixture-name")
	t.Setenv("TADX_DEV_PAT_SECRET", "fixture-secret")
	var out bytes.Buffer
	code := Run(t.Context(), []string{"cache", "refresh", "--environment", "dev", "--scope", "users", "--json"}, &out, options)
	policy.mu.Lock()
	checks := policy.userChecks
	policy.mu.Unlock()
	if code != 0 || !strings.Contains(out.String(), `"status":"refreshed"`) || checks != 1 {
		t.Fatalf("refresh code=%d policy checks=%d output=%s", code, checks, &out)
	}
}

func cachePolicyOptions(t *testing.T, serverURL string) Options {
	t.Helper()
	options := overviewOptions(t, t.TempDir())
	if err := config.Save(options.ConfigPath, config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{
		"dev": {URL: serverURL, APIVersion: "3.29", Auth: config.Auth{Type: config.AuthTypePAT}},
	}}); err != nil {
		t.Fatal(err)
	}
	return options
}
