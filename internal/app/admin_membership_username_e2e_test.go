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

func TestMembershipExactUsernameResolutionAndReceiptThroughCLI(t *testing.T) {
	for _, operation := range []string{"add", "remove"} {
		for _, scenario := range []string{"exact", "display", "email", "missing", "other-site", "ambiguous", "conflicting"} {
			t.Run(operation+"/"+scenario, func(t *testing.T) {
				userReads, memberReads, writes, foreignReads := 0, 0, 0, 0
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if diagnosticSignIn(w, r) {
						return
					}
					if writes > 0 && r.Method == http.MethodGet {
						t.Errorf("receipt performed a post-write read: %s", r.URL.Path)
					}
					switch {
					case r.URL.Path == "/api/3.29/sites/site-1/users":
						userReads++
						users := `<user id="user-1" name="analyst@example.test" fullName="Friendly User" email="email-alias@example.test" siteRole="Viewer"/>`
						total := 1
						if scenario == "ambiguous" {
							users += `<user id="user-2" name="analyst@example.test" siteRole="Viewer"/>`
							total++
						}
						_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="%d"/><users>%s</users></tsResponse>`, r.URL.Query().Get("pageSize"), total, users)
					case r.URL.Path == "/api/3.29/sites/site-2/users":
						foreignReads++
						_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="1"/><users><user id="foreign" name="foreign@example.test"/></users></tsResponse>`)
					case r.URL.Path == "/api/3.29/sites/site-1/groups":
						_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><groups><group id="group-1" name="Reviewers"/></groups></tsResponse>`, r.URL.Query().Get("pageSize"))
					case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users":
						memberReads++
						members := `<user id="other" name="other@example.test"/>`
						total := 1
						if operation == "remove" {
							members += `<user id="user-1" name="analyst@example.test"/>`
							total++
						}
						_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="%d"/><users>%s</users></tsResponse>`, r.URL.Query().Get("pageSize"), total, members)
					case r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users":
						writes++
						body, _ := io.ReadAll(r.Body)
						if !strings.Contains(string(body), `id="user-1"`) {
							t.Errorf("unresolved user mutation: %s", body)
						}
						_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="analyst@example.test"/></tsResponse>`)
					case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users/user-1":
						writes++
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						http.Error(w, "unexpected", 500)
					}
				}))
				defer server.Close()
				username := "analyst@example.test"
				switch scenario {
				case "display":
					username = "Friendly User"
				case "email":
					username = "email-alias@example.test"
				case "missing":
					username = "missing@example.test"
				case "other-site":
					username = "foreign@example.test"
				}
				args := []string{"admin", "group-member", operation, "--group-id", "group-1", "--username", username, "--environment", "test"}
				if scenario == "conflicting" {
					args = append(args, "--user-id", "user-1")
				}
				var output strings.Builder
				code := app.Run(context.Background(), args, &output, diagnosticOptions(t, server))
				if scenario != "exact" {
					if code == 0 || writes != 0 || foreignReads != 0 {
						t.Fatalf("unsafe resolution: code=%d writes=%d foreign=%d %s", code, writes, foreignReads, output.String())
					}
					if scenario == "conflicting" && userReads != 0 {
						t.Fatal("conflicting selectors reached remote reads")
					}
					return
				}
				membership := "present"
				if operation == "remove" {
					membership = "absent"
				}
				if code != 0 || writes != 1 || userReads != 1 || memberReads != 2 || !strings.Contains(output.String(), "membership: "+membership) || !strings.Contains(output.String(), "evidence: mutation_response") {
					t.Fatalf("membership receipt: code=%d writes=%d users=%d members=%d %s", code, writes, userReads, memberReads, output.String())
				}
			})
		}
	}
}
