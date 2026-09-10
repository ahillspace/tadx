package app_test

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestGroupUpdateReceiptReportsConfirmedChangesThroughCLI(t *testing.T) {
	reads, writes := 0, 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		if r.Method == http.MethodGet {
			reads++
			if writes > 0 {
				t.Errorf("post-write read: %s", r.URL.Path)
			}
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/groups":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><groups><group id="group-1" name="Before"/></groups></tsResponse>`, r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="2"/><users><user id="keep-user" name="Existing Inventory"/><user id="old-user" name="Old User"/></users></tsResponse>`, r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1":
			writes++
			_, _ = io.WriteString(w, `<tsResponse><group id="group-1" name="Confirmed Name"/></tsResponse>`)
		case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users":
			writes++
			_, _ = io.WriteString(w, `<tsResponse><user id="new-user" name="New User"/></tsResponse>`)
		case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users/old-user":
			writes++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", 500)
		}
	}))
	defer server.Close()
	var output strings.Builder
	code := app.Run(context.Background(), []string{"admin", "group", "update", "--id", "group-1", "--new-name", "Requested Name", "--set-members", "--member-id", "keep-user", "--member-id", "new-user", "--environment", "test"}, &output, diagnosticOptions(t, server))
	if code != 0 || reads != 4 || writes != 3 {
		t.Fatalf("code=%d reads=%d writes=%d %s", code, reads, writes, output.String())
	}
	for _, want := range []string{"Confirmed Name", "added_user_luids[1]: new-user", "removed_user_luids[1]: old-user"} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("receipt missing %q: %s", want, output.String())
		}
	}
	for _, hidden := range []string{"Requested Name", "Existing Inventory", "keep-user"} {
		if strings.Contains(output.String(), hidden) {
			t.Errorf("compact receipt leaks plan/inventory %q: %s", hidden, output.String())
		}
	}
}
