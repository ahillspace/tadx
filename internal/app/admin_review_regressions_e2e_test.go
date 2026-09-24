package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/cache"
)

func TestAdminCreateSubmittedHTTP502IsUnknownThroughCLI(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		t.Run(kind, func(t *testing.T) {
			var posts int
			var applied bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s" {
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><%ss/></tsResponse>`, kind)
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s" {
					posts++
					applied = true
					w.Header().Set("X-Tableau-Request-Id", "submitted-create-1")
					w.WriteHeader(http.StatusBadGateway)
					_, _ = io.WriteString(w, `<tsResponse><error code="502000"><summary>Gateway failed after apply</summary></error></tsResponse>`)
					return
				}
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}))
			defer server.Close()
			args := []string{"admin", kind, "create", "--environment", "test", "--name", "alice"}
			if kind == "user" {
				args = append(args, "--site-role", "Viewer", "--auth-setting", "ServerDefault")
			}
			var out bytes.Buffer
			code := app.Run(t.Context(), append(args, "--json"), &out, diagnosticOptions(t, server))
			var result struct {
				Error struct {
					ID               string `json:"id"`
					Phase            string `json:"phase"`
					Outcome          string `json:"outcome"`
					Resource         string `json:"resource"`
					TableauRequestID string `json:"tableau_request_id"`
					CorrectiveAction string `json:"corrective_action"`
					Retryable        *bool  `json:"retryable"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			e := result.Error
			if e.ID != "admin."+kind+".create.outcome_unknown" || e.Phase != "submission" || e.Outcome != "unknown" || e.Resource != "alice" || e.TableauRequestID != "submitted-create-1" || e.Retryable == nil || *e.Retryable || !strings.Contains(e.CorrectiveAction, "--name alice") {
				t.Errorf("unexpected error payload: %s", out.String())
			}
			if code == 0 || posts != 1 || !applied {
				t.Errorf("code=%d posts=%d applied=%t output=%s", code, posts, applied, out.String())
			}
		})
	}
}

func TestPermissionUsernamePreviewFiltersFiveThousandUsersThroughCLI(t *testing.T) {
	var listGETs, filteredGETs int
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users":
			listGETs++
			filter := r.URL.Query().Get("filter")
			page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
			var users strings.Builder
			total := 5000
			if strings.HasPrefix(filter, "name:eq:user-") {
				filteredGETs++
				id := strings.TrimPrefix(filter, "name:eq:user-")
				fmt.Fprintf(&users, `<user id="u-%s" name="user-%s"/>`, id, id)
				total = 1
			} else {
				for index := (page - 1) * 1000; index < page*1000; index++ {
					fmt.Fprintf(&users, `<user id="u-%04d" name="user-%04d"/>`, index, index)
				}
			}
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="1000" totalAvailable="%d"/><users>%s</users></tsResponse>`, page, total, users.String())
		case r.Method == http.MethodGet && strings.HasPrefix(r.URL.Path, "/api/3.29/sites/site-1/users/u-"):
			id := strings.TrimPrefix(r.URL.Path, "/api/3.29/sites/site-1/users/")
			fmt.Fprintf(w, `<tsResponse><user id="%s" name="user-%s"/></tsResponse>`, id, strings.TrimPrefix(id, "u-"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/workbooks/workbook-1/permissions":
			_, _ = io.WriteString(w, `<tsResponse><permissions><workbook id="workbook-1"/></permissions></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
		}
	}))
	defer server.Close()
	options := diagnosticOptions(t, server)
	for index := range 10 {
		var out bytes.Buffer
		args := []string{"admin", "permission", "create", "--environment", "test", "--kind", "workbook", "--id", "workbook-1", "--principal-type", "user", "--principal-username", fmt.Sprintf("user-%04d", index), "--capability", "Read", "--mode", "Allow", "--preview"}
		if code := app.Run(t.Context(), args, &out, options); code != 0 {
			t.Fatalf("preview %d: code=%d output=%s", index, code, out.String())
		}
	}
	t.Logf("5000 fixture users, 10 permission previews: list GETs=%d, filtered GETs=%d", listGETs, filteredGETs)
	if listGETs != 10 || filteredGETs != 10 {
		t.Fatalf("list GETs=%d filtered GETs=%d, want 10 each", listGETs, filteredGETs)
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
	for _, args := range [][]string{{"--cache", "--members"}, {"--cache", "--members", "--json"}, {"--cache", "--members", "--json", "--full"}} {
		code, out := run(args...)
		if code == 0 || !strings.Contains(out, "cache.detail_not_indexed") || memberReads != 0 {
			t.Errorf("cached unobserved members: args=%v code=%d reads=%d output=%s", args, code, memberReads, out)
		}
	}
	if code, out := run("--members"); code != 0 || memberReads != 1 {
		t.Fatalf("live members: code=%d reads=%d output=%s", code, memberReads, out)
	}
	for _, args := range [][]string{{"--cache", "--members"}, {"--cache", "--members", "--json"}, {"--cache", "--members", "--json", "--full"}} {
		code, out := run(args...)
		if code != 0 || !strings.Contains(out, "members") || !strings.Contains(out, "[]") || memberReads != 1 {
			t.Errorf("cached observed empty members: args=%v code=%d reads=%d output=%s", args, code, memberReads, out)
		}
	}
	store := cache.NewTargetStore(filepath.Dir(options.ConfigPath), server.URL, "", time.Now)
	legacy := cache.ResourceEntry{Environment: "test", Site: "", Kind: "group", LUID: "group-1", Name: "Authors", Coverage: "detail", Payload: []byte(`{"luid":"group-1","name":"Authors","members":[]}`), ObservedAt: time.Now().Add(time.Minute)}
	if err := store.UpsertResources(t.Context(), []cache.ResourceEntry{legacy}); err != nil {
		t.Fatal(err)
	}
	if code, out := run("--cache", "--members", "--json"); code == 0 || !strings.Contains(out, "cache.detail_not_indexed") || memberReads != 1 {
		t.Fatalf("legacy detail implied members: code=%d reads=%d output=%s", code, memberReads, out)
	}
}
