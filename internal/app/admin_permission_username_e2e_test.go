package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"
)

func TestPermissionUsernamePreviewUsesExactFilteredIdentityThroughCLI(t *testing.T) {
	var listGETs, filteredGETs int
	var filters []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users":
			listGETs++
			filter := r.URL.Query().Get("filter")
			filters = append(filters, filter)
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
		args := []string{"admin", "permission", "create", "--environment", "test", "--kind", "workbook", "--id", "workbook-1", "--principal-type", "user", "--principal-username", fmt.Sprintf("user-%04d", index), "--capability", "Read", "--mode", "Allow", "--preview", "--json"}
		if code := app.Run(t.Context(), args, &out, options); code != 0 {
			t.Fatalf("preview %d: code=%d output=%s", index, code, out.String())
		}
		var result struct {
			Plan struct {
				Target struct {
					PrincipalLUID string `json:"principal_luid"`
				} `json:"target"`
			} `json:"plan"`
		}
		if err := json.Unmarshal(out.Bytes(), &result); err != nil {
			t.Fatalf("preview %d JSON: %v, output=%s", index, err, out.String())
		}
		if want := fmt.Sprintf("u-%04d", index); result.Plan.Target.PrincipalLUID != want {
			t.Errorf("preview %d principal=%q, want %q", index, result.Plan.Target.PrincipalLUID, want)
		}
	}
	if listGETs != 10 || filteredGETs != 10 {
		t.Fatalf("list GETs=%d filtered GETs=%d, want 10 each", listGETs, filteredGETs)
	}
	for index, filter := range filters {
		if want := fmt.Sprintf("name:eq:user-%04d", index); filter != want {
			t.Errorf("preview %d filter=%q, want %q", index, filter, want)
		}
	}
}
