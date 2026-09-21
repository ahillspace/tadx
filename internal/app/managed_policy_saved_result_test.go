package app

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/config"
	"github.com/ahillspace/tadx/internal/lastcommand"
	"github.com/ahillspace/tadx/internal/managedpolicy"
	"github.com/ahillspace/tadx/internal/value"
)

func TestManagedPolicySavedSearchRechecksSemanticPrerequisites(t *testing.T) {
	var requests atomic.Int32
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests.Add(1)
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/auth/signin"):
			io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"current-user"}}}`)
		case strings.HasSuffix(r.URL.Path, "/auth/signout"):
			w.WriteHeader(http.StatusNoContent)
		case strings.HasSuffix(r.URL.Path, "/users"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><users><user id="user-1" name="private-search-user" siteRole="Viewer"/></users></tsResponse>`, r.URL.Query().Get("pageSize"))
		case strings.HasSuffix(r.URL.Path, "/groups"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><groups><group id="group-1" name="private-search-group"/></groups></tsResponse>`, r.URL.Query().Get("pageSize"))
		default:
			t.Errorf("unexpected request %s", r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	root := t.TempDir()
	options := overviewOptions(t, root)
	options.HTTPClient = server.Client()
	if err := config.Save(options.ConfigPath, config.Config{Version: config.CurrentVersion, Environments: map[string]config.Environment{"dev": {URL: server.URL, APIVersion: "3.29", Auth: config.Auth{Type: config.AuthTypePAT}}}}); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TADX_DEV_PAT_NAME", "fixture-name")
	t.Setenv("TADX_DEV_PAT_SECRET", "fixture-secret")
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"search.run": true, "admin.user.list": true, "admin.group.list": true, "last": true}}
	var out bytes.Buffer
	if code := Run(t.Context(), []string{"search", "--type", "admin", "--env", "dev", "--json"}, &out, options); code != 0 || !strings.Contains(out.String(), "private-search-user") || !strings.Contains(out.String(), "private-search-group") {
		t.Fatalf("search code=%d %s", code, &out)
	}
	store := lastcommand.Store{Path: filepath.Join(root, "last-result.json")}
	record, err := store.Read(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"search.run", "admin.user.list", "admin.group.list"} {
		if !slices.Contains(record.RequiredCapabilities, id) {
			t.Errorf("saved prerequisites omit %s: %v", id, record.RequiredCapabilities)
		}
	}
	before := requests.Load()
	options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"search.run": true, "last": true}}
	out.Reset()
	if code := Run(t.Context(), []string{"last", "--json"}, &out, options); code == 0 || strings.Contains(out.String(), "private-search-") {
		t.Fatalf("saved search leaked code=%d %s", code, &out)
	}
	if requests.Load() != before {
		t.Fatal("last replay made remote requests")
	}
}

func TestManagedPolicyLegacySearchChecksOnlyTypedAdministrativeRows(t *testing.T) {
	for _, test := range []struct {
		name    string
		payload string
		deny    bool
	}{
		{"user", `{"items":[{"type":"user","name":"private-result"}]}`, true},
		{"partial-group", `{"output":{"items":[{"type":"group","name":"private-result"}]},"error":{"summary":"partial"}}`, true},
		{"content-owner", `{"items":[{"type":"workbook","name":"public-result","owner":{"type":"user","name":"ordinary-owner"}}]}`, false},
	} {
		t.Run(test.name, func(t *testing.T) {
			root := t.TempDir()
			options := overviewOptions(t, root)
			options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{"search.run": true, "last": true}}
			store := lastcommand.Store{Path: filepath.Join(root, "last-result.json")}
			if err := store.Save(t.Context(), value.SavedExecution{RecordedAt: time.Now(), Operation: "search.run", Result: json.RawMessage(test.payload)}); err != nil {
				t.Fatal(err)
			}
			var out bytes.Buffer
			code := Run(t.Context(), []string{"last", "--json"}, &out, options)
			if (code != 0) != test.deny || (test.deny && strings.Contains(out.String(), "private-result")) {
				t.Fatalf("deny=%v code=%d %s", test.deny, code, &out)
			}
		})
	}
}
