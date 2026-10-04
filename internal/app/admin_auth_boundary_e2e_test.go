package app_test

import (
	"bytes"
	"fmt"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestAdminDefaultSiteAndFailedAuthenticationBoundary(t *testing.T) {
	for _, reject := range []bool{false, true} {
		t.Run(fmt.Sprint(reject), func(t *testing.T) {
			auth, reads, writes := 0, 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/api/3.29/auth/signin" {
					auth++
					if reject {
						w.WriteHeader(http.StatusUnauthorized)
						return
					}
					diagnosticSignIn(w, r)
					return
				}
				if r.Method != http.MethodGet {
					writes++
				}
				reads++
				if r.Method != http.MethodGet || r.URL.Path != "/api/3.29/sites/site-1/groups" {
					t.Errorf("unexpected post-authentication route: %s %s", r.Method, r.URL.Path)
				}
				io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><groups/></tsResponse>`)
			}))
			defer server.Close()
			options := diagnosticOptions(t, server)
			var out bytes.Buffer
			code := app.Run(t.Context(), []string{"admin", "group", "create", "--environment", "test", "--name", "Fixture", "--preview", "--json"}, &out, options)
			if auth != 1 || writes != 0 || reject && (code == 0 || reads != 0) || !reject && (code != 0 || reads != 1) {
				t.Fatalf("code=%d auth=%d reads=%d writes=%d output=%s", code, auth, reads, writes, &out)
			}
		})
	}
}
