package app_test

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"
)

func TestCatalogMetadataIDListToInspectThroughCLI(t *testing.T) {
	for _, kind := range []string{"database", "table", "column"} {
		t.Run(kind, func(t *testing.T) {
			reads := 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogMetadataSignIn(w, r) {
					return
				}
				if r.Method != http.MethodPost || r.URL.Path != "/api/metadata/graphql" {
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", http.StatusNotFound)
					return
				}
				reads++
				var request struct {
					Variables struct {
						Filter map[string]json.RawMessage `json:"filter"`
						First  int                        `json:"first"`
					} `json:"variables"`
				}
				if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
					t.Error(err)
					http.Error(w, "invalid body", http.StatusBadRequest)
					return
				}
				// Retained Cloud diagnostics return this execution error for scalar
				// id, although the official schema admits both id and idWithin.
				if _, scalar := request.Variables.Filter["id"]; scalar {
					io.WriteString(w, `{"errors":[{"message":"Internal Server Error(s) while executing query","locations":[]}]}`)
					return
				}
				if request.Variables.First == 2 && (len(request.Variables.Filter) != 1 || string(request.Variables.Filter["idWithin"]) != `["metadata-asset"]`) {
					t.Errorf("inspection did not select exactly the listed Metadata ID: %+v", request.Variables)
					http.Error(w, "unexpected selector", http.StatusBadRequest)
					return
				}
				page := `{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"metadata-asset","luid":"rest-asset","name":"Warehouse","description":"Confirmed asset meaning"}]}`
				if kind == "column" && request.Variables.First == 1 {
					fmt.Fprintf(w, `{"data":{"parents":{"nodes":[{"luid":"parent-table","items":%s}]}}}`, page)
					return
				}
				fmt.Fprintf(w, `{"data":{"items":%s}}`, page)
			}))
			defer server.Close()
			options := catalogMetadataOptions(t, server, false)
			list := []string{"catalog", kind, "list", "--env", "production", "--limit", "1"}
			if kind == "column" {
				list = append(list, "--table-id", "parent-table")
			}
			code, payload := resultContractJSON(t, list, options)
			items, ok := payload["items"].([]any)
			if code != 0 || !ok || len(items) != 1 {
				t.Fatalf("list failed: exit %d: %#v", code, payload)
			}
			metadataID, _ := contractObject(t, items[0])["metadata_id"].(string)
			if metadataID == "" {
				t.Fatalf("list omitted Metadata ID: %#v", payload)
			}
			for _, full := range []bool{false, true} {
				args := []string{"catalog", kind, "inspect", "--env", "production", "--metadata-id", metadataID}
				if full {
					args = append(args, "--full")
				}
				code, payload = resultContractJSON(t, args, options)
				if code != 0 {
					t.Fatalf("inspect full=%v failed for listed Metadata ID: exit %d: %#v", full, code, payload)
				}
				item := contractObject(t, payload["item"])
				if payload["status"] != "inspected" || item["metadata_id"] != metadataID || item["luid"] != "rest-asset" || item["description"] != "Confirmed asset meaning" {
					t.Fatalf("inspect full=%v lost exact identity or metadata: %#v", full, payload)
				}
			}
			if reads != 3 {
				t.Fatalf("list and two inspections made %d metadata requests, want 3", reads)
			}
		})
	}
}
