package app_test

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/config"
)

// targetFixtureTransport preserves the public target identity while serving all
// requests through a local TLS fixture, without DNS or external network access.
type targetFixtureTransport struct {
	endpoint *url.URL
	base     http.RoundTripper
}

func (transport targetFixtureTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	local := request.Clone(request.Context())
	local.URL.Scheme, local.URL.Host = transport.endpoint.Scheme, transport.endpoint.Host
	return transport.base.RoundTrip(local)
}

func TestDefaultSearchUsesProductSupportedSources(t *testing.T) {
	for _, cloud := range []bool{false, true} {
		for _, limit := range []string{"1", "20"} {
			t.Run(fmt.Sprintf("cloud=%t/limit=%s", cloud, limit), func(t *testing.T) {
				pulseCalls, contentCalls := 0, 0
				server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if diagnosticSignIn(w, r) {
						return
					}
					switch r.URL.Path {
					case "/api/-/search":
						contentCalls++
						fmt.Fprintf(w, `{"items":[{"uri":"/workbooks/wb-1","content":{"type":"workbook","luid":"wb-1","title":"Sales Dashboard"}}],"limit":%s,"pageIndex":0,"startIndex":0,"total":1}`, r.URL.Query().Get("limit"))
					case "/api/-/pulse/definitions":
						pulseCalls++
						if !cloud {
							t.Error("Server search requested Pulse")
							w.WriteHeader(http.StatusNotFound)
							return
						}
						io.WriteString(w, `{"definitions":[],"next_page_token":""}`)
					case "/api/3.29/sites/site-1/groups", "/api/3.29/sites/site-1/users":
						kind := "groups"
						if strings.HasSuffix(r.URL.Path, "/users") {
							kind = "users"
						}
						fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="0"/><%s/></tsResponse>`, r.URL.Query().Get("pageSize"), kind)
					default:
						t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				}))
				defer server.Close()
				options := diagnosticOptions(t, server)
				if cloud {
					if _, err := config.Update(options.ConfigPath, false, func(cfg config.Config) (config.Config, error) {
						environment := cfg.Environments["test"]
						environment.URL = "https://prod-test.online.tableau.com"
						cfg.Environments["test"] = environment
						return cfg, nil
					}); err != nil {
						t.Fatal(err)
					}
					endpoint, err := url.Parse(server.URL)
					if err != nil {
						t.Fatal(err)
					}
					client := *server.Client()
					client.Transport = targetFixtureTransport{endpoint: endpoint, base: client.Transport}
					options.HTTPClient = &client
				}
				args := []string{"search", "Sales", "--environment", "test", "--limit", limit, "--json"}
				var output strings.Builder
				if code := app.Run(t.Context(), args, &output, options); code != 0 || !strings.Contains(output.String(), "Sales Dashboard") {
					t.Fatalf("code=%d output=%s", code, output.String())
				}
				if !cloud && !strings.Contains(output.String(), "Search scope: Server content, groups, and users") {
					t.Fatalf("missing searched scope: %s", output.String())
				}
				if contentCalls != 1 || (cloud && limit == "20" && pulseCalls == 0) || ((!cloud || limit == "1") && pulseCalls != 0) {
					t.Fatalf("content=%d pulse=%d", contentCalls, pulseCalls)
				}
				if !cloud {
					for _, kind := range []string{"pulse", "definition", "metric"} {
						output.Reset()
						if code := app.Run(t.Context(), append(args, "--type", kind), &output, options); code == 0 || !strings.Contains(output.String(), "search.unsupported_type") || pulseCalls != 0 {
							t.Fatalf("unsupported %s: code=%d output=%s", kind, code, output.String())
						}
					}
				}
			})
		}
	}
}
