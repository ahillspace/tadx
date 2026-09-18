package app_test

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestExplicitGroupMembersStayCompleteInCompactCLI(t *testing.T) {
	for _, total := range []int{0, 201} {
		t.Run(strconv.Itoa(total), func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				size, _ := strconv.Atoi(r.URL.Query().Get("pageSize"))
				page, _ := strconv.Atoi(r.URL.Query().Get("pageNumber"))
				switch {
				case strings.HasSuffix(r.URL.Path, "/groups"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="1"/><groups><group id="group-1" name="Readers"/></groups></tsResponse>`, page, size)
				case strings.HasSuffix(r.URL.Path, "/groups/group-1/users"):
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="%d" pageSize="%d" totalAvailable="%d"/><users>`, page, size, total)
					for i := (page - 1) * size; i < min(page*size, total); i++ {
						fmt.Fprintf(w, `<user id="user-%d" name="reader-%d"/>`, i, i)
					}
					fmt.Fprint(w, `</users></tsResponse>`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL)
				}
			}))
			defer server.Close()
			out := runGroupOneCLI(t, diagnosticOptions(t, server), "admin", "group", "inspect", "--environment", "test", "--id", "group-1", "--members", "--json")
			var result struct {
				Group struct {
					Members []struct {
						LUID string `json:"luid"`
					} `json:"members"`
				} `json:"group"`
			}
			if err := json.Unmarshal([]byte(out), &result); err != nil {
				t.Fatal(err)
			}
			if result.Group.Members == nil || len(result.Group.Members) != total {
				t.Fatalf("requested members=%d returned=%d: %s", total, len(result.Group.Members), out)
			}
		})
	}
}
