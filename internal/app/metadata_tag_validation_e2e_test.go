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

func metadataTagUpdateArgs(kind string) []string {
	args := []string{"catalog", kind, "update", "--env", "production", "--id", "asset", "--description", "After", "--json"}
	if kind == "column" {
		args = append(args, "--table-id", "parent")
	}
	return args
}

func TestMetadataTagLengthRejectedBeforeAnyHTTP(t *testing.T) {
	for _, kind := range []string{"database", "table", "column"} {
		for _, preview := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/preview=%v", kind, preview), func(t *testing.T) {
				calls := 0
				s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					calls++
					w.WriteHeader(http.StatusInternalServerError)
				}))
				defer s.Close()
				args := append(metadataTagUpdateArgs(kind), "--add-tag", strings.Repeat("x", 129))
				if preview {
					args = append(args, "--preview")
				}
				var out bytes.Buffer
				code := app.Run(context.Background(), args, &out, catalogMetadataOptions(t, s, true))
				if code == 0 || calls != 0 || !strings.Contains(out.String(), "128") {
					t.Fatalf("code=%d HTTP calls=%d output=%s", code, calls, &out)
				}
			})
		}
	}
}

func TestMetadataUnicodeTagPreviewAndUnconfirmedWrite(t *testing.T) {
	for _, kind := range []string{"database", "table", "column"} {
		for _, tc := range []struct {
			name, tag string
			preview   bool
		}{
			{"ascii-execution", "sales", false},
			{"unicode-execution", strings.Repeat("界", 128), false},
			{"unicode-preview", strings.Repeat("界", 128), true},
		} {
			t.Run(kind+"/"+tc.name, func(t *testing.T) {
				preview := tc.preview
				propertyWrites, tagWrites := 0, 0
				s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if catalogMetadataSignIn(w, r) {
						return
					}
					base := "/api/3.29/sites/site-1/" + kind + "s/asset"
					if kind == "column" {
						base = "/api/3.29/sites/site-1/tables/parent/columns/asset"
					}
					if r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s/asset/tags" {
						tagWrites++
						io.WriteString(w, `<tsResponse><tags/></tsResponse>`)
						return
					}
					if r.URL.Path != base || (r.Method != http.MethodGet && r.Method != http.MethodPut) {
						t.Errorf("unexpected %s %s", r.Method, r.URL)
						w.WriteHeader(http.StatusInternalServerError)
						return
					}
					description := "Before"
					if r.Method == http.MethodPut {
						propertyWrites++
						description = "After"
					}
					fmt.Fprintf(w, `<tsResponse><%s id="asset" name="Asset" parentTableId="parent" description="%s"><tags/></%s></tsResponse>`, kind, description, kind)
				}))
				defer s.Close()
				args := append(metadataTagUpdateArgs(kind), "--add-tag", tc.tag)
				if preview {
					args = append(args, "--preview")
				}
				var out bytes.Buffer
				code := app.Run(context.Background(), args, &out, catalogMetadataOptions(t, s, !preview))
				if !json.Valid(out.Bytes()) {
					t.Fatalf("invalid JSON: %s", &out)
				}
				if preview {
					if code != 0 || propertyWrites != 0 || tagWrites != 0 {
						t.Fatalf("preview code=%d writes=%d/%d output=%s", code, propertyWrites, tagWrites, &out)
					}
					return
				}
				if code == 0 || propertyWrites != 1 || tagWrites != 1 {
					t.Fatalf("execution code=%d writes=%d/%d output=%s", code, propertyWrites, tagWrites, &out)
				}
				for _, expected := range []string{`"completed":["description"]`, `"failed":"add_tags"`, `"luid":"asset"`, `"status":"partial"`, `"phase":"verification"`, `"outcome":"unknown"`, `"retryable":false`} {
					if !strings.Contains(out.String(), expected) {
						t.Errorf("missing %s in %s", expected, &out)
					}
				}
			})
		}
	}
}
