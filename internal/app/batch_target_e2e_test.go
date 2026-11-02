package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestMutationBatchesRejectResolvedSelectorAliasesBeforeAnyWrite(t *testing.T) {
	for _, test := range []struct {
		name, body string
		command    []string
	}{
		{
			name: "workbook ID and scoped name", command: []string{"content", "workbook", "update"},
			body: `{"items":[{"id":"w1","description":"A"},{"name":"Workbook","project":"Project","description":"B"}]}`,
		},
		{
			name: "project ID and path", command: []string{"content", "project", "update"},
			body: `{"items":[{"id":"p1","description":"A"},{"project":"Project","description":"B"}]}`,
		},
		{
			name: "project parent ID and path", command: []string{"content", "project", "create"},
			body: `{"items":[{"name":"Child","parent-id":"p1","description":"A"},{"name":"Child","parent":"Project","description":"B"}]}`,
		},
		{
			name: "user ID and exact username", command: []string{"admin", "user", "update"},
			body: `{"items":[{"id":"u1","site-role":"Viewer"},{"username":"alice","site-role":"Creator"}]}`,
		},
		{
			name: "membership user ID and username", command: []string{"admin", "group-member", "add"},
			body: `{"items":[{"group-id":"g1","user-id":"u1"},{"group-id":"g1","username":"alice"}]}`,
		},
		{
			name: "permission principal ID and username", command: []string{"admin", "permission", "create"},
			body: `{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-id":"u1","capability":"Read","mode":"Allow"},{"kind":"workbook","id":"w1","principal-type":"user","principal-username":"alice","capability":"Read","mode":"Deny"}]}`,
		},
	} {
		t.Run(test.name, func(t *testing.T) {
			writes, reads := 0, 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.Method != http.MethodGet {
					writes++
					http.Error(w, "unexpected mutation", http.StatusInternalServerError)
					return
				}
				reads++
				page, size := r.URL.Query().Get("pageNumber"), r.URL.Query().Get("pageSize")
				switch r.URL.Path {
				case "/api/3.29/sites/site-1/projects":
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><projects><project id="p1" name="Project"/></projects></tsResponse>`, page, size)
				case "/api/3.29/sites/site-1/workbooks":
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><workbooks><workbook id="w1" name="Workbook"><project id="p1" name="Project"/><owner id="owner"/></workbook></workbooks></tsResponse>`, page, size)
				case "/api/3.29/sites/site-1/users":
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%s" pageSize="%s" totalAvailable="1"/><users><user id="u1" name="alice" siteRole="Viewer"/></users></tsResponse>`, page, size)
				default:
					t.Errorf("unexpected read %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer server.Close()
			batch := filepath.Join(t.TempDir(), "items.json")
			if err := os.WriteFile(batch, []byte(test.body), 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			args := append(append([]string(nil), test.command...), "--environment", "test", "--batch-file", batch, "--json")
			code := app.Run(t.Context(), args, &output, diagnosticOptions(t, server))
			if code == 0 || writes != 0 || reads == 0 || !strings.Contains(output.String(), "batch item 2 duplicates batch item 1") {
				t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads, writes, output.String())
			}
		})
	}
}

func TestBatchAliasReadFailureRetainsOperationContract(t *testing.T) {
	reads, writes := 0, 0
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		if r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users" {
			reads++
			http.Error(w, "unavailable", http.StatusServiceUnavailable)
			return
		}
		writes++
		http.Error(w, "unexpected mutation", http.StatusInternalServerError)
	}))
	defer server.Close()
	batch := filepath.Join(t.TempDir(), "items.json")
	if err := os.WriteFile(batch, []byte(`{"items":[{"username":"alice"}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	code := app.Run(t.Context(), []string{"admin", "user", "delete", "--environment", "test", "--batch-file", batch, "--json"}, &output, diagnosticOptions(t, server))
	var envelope struct {
		Error struct{ ID, Kind, Phase, Outcome string }
	}
	if err := json.Unmarshal(output.Bytes(), &envelope); err != nil {
		t.Fatal(err)
	}
	if code != 1 || reads != 1 || writes != 0 || envelope.Error.ID != "admin.user.delete.batch.prepare" || envelope.Error.Kind != "operation" || envelope.Error.Phase != "validation" || envelope.Error.Outcome != "not_attempted" {
		t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads, writes, output.String())
	}
}
