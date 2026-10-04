package app_test

import (
	"bytes"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestAdminDetailCachePayloadsPreserveInspectionContract(t *testing.T) {
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		w.Header().Set("X-Tableau-Request-Id", "read-request")
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/users/user-1":
			io.WriteString(w, `<tsResponse><user id="user-1" name="analyst" fullName="Analyst" email="analyst@example.test" siteRole="Viewer"/></tsResponse>`)
		case "/api/3.29/sites/site-1/groups":
			io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><groups><group id="group-1" name="Authors"/></groups></tsResponse>`)
		case "/api/3.29/sites/site-1/groups/group-1/users":
			io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><users/></tsResponse>`)
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	store := cache.NewTargetStore(filepath.Dir(options.ConfigPath), server.URL, "", time.Now)
	for _, tc := range []struct{ kind, id, want string }{
		{"user", "user-1", `{"luid":"user-1","name":"analyst","full_name":"Analyst","email":"analyst@example.test","site_role":"Viewer"}`},
		{"group", "group-1", `{"luid":"group-1","name":"Authors","members":[],"members_fetched":true}`},
	} {
		var out bytes.Buffer
		args := []string{"admin", tc.kind, "inspect", "--environment", "test", "--id", tc.id, "--json", "--full"}
		if tc.kind == "group" {
			args = append(args, "--members")
		}
		if code := app.Run(t.Context(), args, &out, options); code != 0 {
			t.Fatalf("%s inspect: code=%d output=%s", tc.kind, code, &out)
		}
		result, err := store.ReadResources(t.Context(), cache.ResourceQuery{Environment: "test", Site: "", Kind: tc.kind, LUID: tc.id, Limit: 1, ExactlyOne: true})
		if err != nil || len(result.Entries) != 1 {
			t.Fatalf("%s cache: result=%+v error=%v", tc.kind, result, err)
		}
		if got := string(result.Entries[0].Payload); got != tc.want {
			t.Errorf("%s cache payload=%s, want %s", tc.kind, got, tc.want)
		}
	}
}

func TestCachedGroupMembersRequireObservedCoverageThroughCLI(t *testing.T) {
	var memberReads int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/groups":
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><groups><group id="group-1" name="Authors"/></groups></tsResponse>`)
		case "/api/3.29/sites/site-1/groups/group-1/users":
			memberReads++
			_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><users/></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	run := func(args ...string) (int, string) {
		t.Helper()
		var out bytes.Buffer
		code := app.Run(t.Context(), append([]string{"admin", "group", "inspect", "--environment", "test", "--id", "group-1"}, args...), &out, options)
		return code, out.String()
	}
	if code, out := run(); code != 0 {
		t.Fatalf("live inspect: code=%d output=%s", code, out)
	}
	store := cache.NewTargetStore(filepath.Dir(options.ConfigPath), server.URL, "", time.Now)
	seedGroup := func(payload string) {
		t.Helper()
		entry := cache.ResourceEntry{Environment: "test", Site: "", Kind: "group", LUID: "group-1", Name: "Authors", Coverage: "detail", Payload: []byte(payload), ObservedAt: time.Now()}
		if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{entry}); err != nil {
			t.Fatal(err)
		}
	}
	// The cache write-through is best-effort; this test owns its coverage fixture.
	seedGroup(`{"luid":"group-1","name":"Authors","members":[],"members_fetched":false}`)
	for _, args := range [][]string{{"--cache", "--members"}, {"--cache", "--members", "--json"}, {"--cache", "--members", "--json", "--full"}} {
		code, out := run(args...)
		if code == 0 || !strings.Contains(out, "cache.detail_not_indexed") || memberReads != 0 {
			t.Errorf("cached unobserved members: args=%v code=%d reads=%d output=%s", args, code, memberReads, out)
		}
	}
	if code, out := run("--members"); code != 0 || memberReads != 1 {
		t.Fatalf("live members: code=%d reads=%d output=%s", code, memberReads, out)
	}
	seedGroup(`{"luid":"group-1","name":"Authors","members":[],"members_fetched":true}`)
	for _, args := range [][]string{{"--cache", "--members"}, {"--cache", "--members", "--json"}, {"--cache", "--members", "--json", "--full"}} {
		code, out := run(args...)
		if code != 0 || !strings.Contains(out, "members") || !strings.Contains(out, "[]") || memberReads != 1 {
			t.Errorf("cached observed empty members: args=%v code=%d reads=%d output=%s", args, code, memberReads, out)
		}
	}
	legacy := cache.ResourceEntry{Environment: "test", Site: "", Kind: "group", LUID: "group-1", Name: "Authors", Coverage: "detail", Payload: []byte(`{"luid":"group-1","name":"Authors","members":[]}`), ObservedAt: time.Now().Add(time.Minute)}
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{legacy}); err != nil {
		t.Fatal(err)
	}
	if code, out := run("--cache", "--members", "--json"); code == 0 || !strings.Contains(out, "cache.detail_not_indexed") || memberReads != 1 {
		t.Fatalf("legacy detail implied members: code=%d reads=%d output=%s", code, memberReads, out)
	}
}
