package app_test

import (
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestGroupMemberRemovalRequiresCompleteInventory(t *testing.T) {
	for _, scenario := range []string{"repeated boundary", "changed total", "empty intermediate", "changed page size", "complete absent", "complete present"} {
		t.Run(scenario, func(t *testing.T) {
			deletes, reads := 0, 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case r.URL.Path == "/api/3.29/sites/site-1/groups":
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><groups><group id="group-1" name="Reviewers"/></groups></tsResponse>`, r.URL.Query().Get("pageSize"))
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users":
					reads++
					number, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
					size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
					total := 1001
					start, count := (number-1)*size, size
					if number == 2 {
						count = 1
						switch scenario {
						case "repeated boundary":
							start = 0
						case "changed total":
							total++
						case "empty intermediate":
							count = 0
						case "changed page size":
							size--
						}
					}
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><users>`, number, size, total)
					for i := start; i < start+count; i++ {
						fmt.Fprintf(w, `<user id="u-%04d" name="user%04d@example.test"/>`, i, i)
					}
					io.WriteString(w, `</users></tsResponse>`)
				case r.Method == http.MethodDelete && r.URL.Path == "/api/3.29/sites/site-1/groups/group-1/users/u-1000":
					deletes++
					w.WriteHeader(http.StatusNoContent)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				}
			}))
			defer server.Close()
			user := "u-1000"
			if scenario == "complete absent" {
				user = "u-absent"
			}
			var output strings.Builder
			code := app.Run(t.Context(), []string{"admin", "group-member", "remove", "--group-id", "group-1", "--user-id", user, "--environment", "test"}, &output, diagnosticOptions(t, server))
			if strings.HasPrefix(scenario, "complete") {
				wantDeletes := 0
				if scenario == "complete present" {
					wantDeletes = 1
				}
				if code != 0 || deletes != wantDeletes || reads != 4 || !strings.Contains(output.String(), "membership: absent") {
					t.Fatalf("code=%d deletes=%d reads=%d output=%s", code, deletes, reads, output.String())
				}
			} else if code == 0 || deletes != 0 || strings.Contains(output.String(), "membership: absent") {
				t.Fatalf("incomplete inventory claimed success: code=%d deletes=%d output=%s", code, deletes, output.String())
			}
		})
	}
}
