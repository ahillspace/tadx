package app_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestPermissionCreateBatchFileResolvesUsernamesBeforeWrites(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(fmt.Sprintf("preview=%t", preview), func(t *testing.T) {
			var signins, writes int
			var bodies []string
			savedRules := make(map[string]struct{ capability, mode string })
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method == http.MethodPost && r.URL.Path == "/api/3.29/auth/signin" {
					signins++
					w.Header().Set("Content-Type", "application/json")
					_, _ = io.WriteString(w, `{"credentials":{"token":"token","site":{"id":"site-1"},"user":{"id":"admin"}}}`)
					return
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users":
					_, _ = io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="2"/><users><user id="u-a" name="alice"/><user id="u-b" name="bob"/></users></tsResponse>`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/u-a":
					_, _ = io.WriteString(w, `<tsResponse><user id="u-a" name="alice"/></tsResponse>`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/u-b":
					_, _ = io.WriteString(w, `<tsResponse><user id="u-b" name="bob"/></tsResponse>`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/workbooks/w1/permissions":
					var rules strings.Builder
					for _, user := range []struct {
						id string
					}{
						{id: "u-a"},
						{id: "u-b"},
					} {
						rule, ok := savedRules[user.id]
						if !ok {
							continue
						}
						fmt.Fprintf(&rules, `<granteeCapabilities><user id="%s"/><capabilities><capability name="%s" mode="%s"/></capabilities></granteeCapabilities>`, user.id, rule.capability, rule.mode)
					}
					fmt.Fprintf(w, `<tsResponse><permissions><workbook id="w1"/>%s</permissions></tsResponse>`, rules.String())
				case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/workbooks/w1/permissions":
					body, _ := io.ReadAll(r.Body)
					bodies = append(bodies, string(body))
					writes++
					if strings.Contains(string(body), `id="u-a"`) {
						savedRules["u-a"] = struct{ capability, mode string }{capability: "Read", mode: "Allow"}
						_, _ = io.WriteString(w, `<tsResponse><permissions><granteeCapabilities><user id="u-a"/><capabilities><capability name="Read" mode="Allow"/></capabilities></granteeCapabilities></permissions></tsResponse>`)
						return
					}
					savedRules["u-b"] = struct{ capability, mode string }{capability: "Write", mode: "Deny"}
					_, _ = io.WriteString(w, `<tsResponse><permissions><granteeCapabilities><user id="u-b"/><capabilities><capability name="Write" mode="Deny"/></capabilities></granteeCapabilities></permissions></tsResponse>`)
				default:
					t.Fatalf("unexpected %s %s", r.Method, r.URL.Path)
				}
			}))
			defer server.Close()
			config := filepath.Join(t.TempDir(), "tadx.yaml")
			if err := os.WriteFile(config, []byte(fmt.Sprintf("version: 1\nenvironments:\n  test:\n    url: %s\n    site_content_url: marketing\n    api_version: \"3.29\"\n    auth:\n      type: pat\n      pat_name_env: BATCH_PAT_NAME\n      pat_secret_env: BATCH_PAT_SECRET\n", server.URL)), 0600); err != nil {
				t.Fatal(err)
			}
			batch := filepath.Join(t.TempDir(), "rules.json")
			if err := os.WriteFile(batch, []byte(`{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-username":"alice","capability":"Read","mode":"Allow"},{"kind":"workbook","id":"w1","principal-type":"user","principal-username":"bob","capability":"Write","mode":"Deny"}]}`), 0600); err != nil {
				t.Fatal(err)
			}
			t.Setenv("BATCH_PAT_NAME", "name")
			t.Setenv("BATCH_PAT_SECRET", "secret")
			args := []string{"admin", "permission", "create", "--batch-file", batch, "--environment", "test"}
			if preview {
				args = append(args, "--preview")
			}
			var output bytes.Buffer
			opts := app.Options{ConfigPath: config, HTTPClient: server.Client(), MutationEnvironment: func() (string, bool) { return "1", true }}
			code := app.Run(context.Background(), args, &output, opts)
			if code != 0 || signins != 1 || (preview && (writes != 0 || len(savedRules) != 0)) || (!preview && (writes != 2 || len(savedRules) != 2 || !strings.Contains(bodies[0], `id="u-a"`) || !strings.Contains(bodies[1], `id="u-b"`))) {
				t.Fatalf("code=%d signins=%d writes=%d bodies=%v output=%s", code, signins, writes, bodies, output.String())
			}
		})
	}
}
