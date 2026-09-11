package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestMetadataTerminalCoverageCLI(t *testing.T) {
	for _, kind := range []string{"database", "table", "column"} {
		for _, command := range []string{"list", "search", "audit"} {
			if command == "audit" && kind != "column" {
				continue
			}
			for _, limited := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/%s/limited=%v", kind, command, limited), func(t *testing.T) {
					calls := 0
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if catalogMetadataSignIn(w, r) {
							return
						}
						if r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/tables/table-1") {
							io.WriteString(w, `<tsResponse><table id="table-1" name="Orders" description="Meaning"/></tsResponse>`)
							return
						}
						if r.URL.Path != "/api/metadata/graphql" {
							t.Errorf("unexpected request %s", r.URL)
							w.WriteHeader(500)
							return
						}
						calls++
						page := `{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"next"},"nodes":[{"id":"meta-1","luid":"asset-1","name":"Sales","description":"Meaning"}]}`
						if calls == 2 {
							page = `{"totalCount":2,"pageInfo":{"hasNextPage":false},"nodes":[]}`
						}
						if kind == "column" {
							fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"luid":"table-1","items":%s}]}}}`, page)
						} else {
							fmt.Fprintf(w, `{"data":{"items":%s}}`, page)
						}
					}))
					defer server.Close()
					args := []string{"catalog", kind, "list", "--all"}
					if kind == "column" {
						args = append(args, "--table-id", "table-1")
					}
					if command == "search" {
						args = []string{"catalog", "search", "Sales", "--type", kind, "--all"}
						if kind == "column" {
							args = append(args, "--table-id", "table-1")
						}
					}
					if command == "audit" {
						args = []string{"catalog", "audit", "--type", "table", "--id", "table-1", "--check", "descriptions"}
					}
					if limited {
						for i, arg := range args {
							if arg == "--all" {
								args[i] = "--limit=1"
							}
						}
						if command == "audit" {
							args = append(args, "--limit", "2")
						}
					}
					args = append(args, "--env", "production", "--json", "--full")
					var out bytes.Buffer
					code := app.Run(context.Background(), args, &out, catalogMetadataOptions(t, server, false))
					wantCalls := 2
					if limited {
						wantCalls = 1
					}
					if (code == 0) != limited || calls != wantCalls || !strings.Contains(out.String(), "meta-1") || !strings.Contains(out.String(), `"complete":false`) {
						t.Fatalf("code=%d calls=%d %s", code, calls, &out)
					}
				})
			}
		}
	}
}

func TestMetadataAuditInheritedCoverageCLI(t *testing.T) {
	for _, tc := range []struct {
		name, inherited, state string
		directOnly             bool
	}{
		{"unobserved", "null", "unknown", false},
		{"null-value", `[{"value":null,"assetId":"column-1","attribute":"description"}]`, "unknown", false},
		{"empty", "[]", "missing", false},
		{"present", `[{"value":"Meaning","assetId":"column-1","attribute":"description"}]`, "present", false},
		{"direct-only", "null", "missing", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogMetadataSignIn(w, r) {
					return
				}
				fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"id":"ds-meta","luid":"ds-1","name":"Sales","description":"Sales data","items":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"field-1","name":"Sales","fullyQualifiedName":"[Sales]","description":"","descriptionInherited":%s,"upstreamColumnsConnection":{"totalCount":0,"pageInfo":{"hasNextPage":false},"nodes":[]}}]}}]}}}`, tc.inherited)
			}))
			defer server.Close()
			args := []string{"catalog", "audit", "--type", "datasource", "--id", "ds-1", "--check", "descriptions", "--env", "production", "--json", "--full"}
			if tc.directOnly {
				args = append(args, "--direct-only")
			}
			var out bytes.Buffer
			if code := app.Run(context.Background(), args, &out, catalogMetadataOptions(t, server, false)); code != 0 {
				t.Fatalf("exit=%d %s", code, &out)
			}
			var result struct {
				Findings []struct {
					MetadataID string `json:"metadata_id"`
					State      string `json:"state"`
				}
			}
			if e := json.Unmarshal(out.Bytes(), &result); e != nil {
				t.Fatal(e)
			}
			if len(result.Findings) != 2 || result.Findings[1].MetadataID != "field-1" || result.Findings[1].State != tc.state {
				t.Fatalf("unexpected finding: %s", &out)
			}
		})
	}
}
