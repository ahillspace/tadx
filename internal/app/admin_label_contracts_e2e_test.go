package app_test

import (
	"bytes"
	"fmt"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAdminLabelBatchRejectsInvalidLaterRowBeforeAuthentication(t *testing.T) {
	calls := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()
	options := catalogMetadataOptions(t, server, true)
	path := filepath.Join(t.TempDir(), "categories.json")
	if err := os.WriteFile(path, []byte(`{"items":[{"name":"First","description":"Valid"},{"name":"Second","description":""}]}`), 0600); err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"admin", "label-category", "create", "--environment", "production", "--batch-file", path, "--json"}, &out, options)
	if code == 0 || calls != 0 || !strings.Contains(out.String(), "batch item 2") {
		t.Fatalf("code=%d calls=%d output=%s", code, calls, &out)
	}
}

func TestAdminVocabularyDuplicateNormalizationThroughCLI(t *testing.T) {
	for _, kind := range []string{"category", "value"} {
		for _, conflict := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/conflict=%t", kind, conflict), func(t *testing.T) {
				reads := 0
				server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					if catalogMetadataSignIn(w, r) {
						return
					}
					reads++
					first := `<labelCategory name="Fixture" description="Meaning"/>`
					container := "labelCategoryList"
					if kind == "value" {
						first = `<labelValue name="Fixture" category="Custom" description="Meaning" internal="false" elevatedDefault="false" builtIn="false"/>`
						container = "labelValueList"
					}
					second := first
					if conflict {
						second = strings.Replace(first, "Meaning", "Changed", 1)
					}
					fmt.Fprintf(w, "<tsResponse><%s>%s%s</%s></tsResponse>", container, first, second, container)
				}))
				defer server.Close()
				var out bytes.Buffer
				code := app.Run(t.Context(), []string{"admin", "label-" + kind, "list", "--environment", "production", "--json"}, &out, catalogMetadataOptions(t, server, false))
				if reads != 1 || conflict && code == 0 || !conflict && (code != 0 || !strings.Contains(out.String(), `"returned":1`)) {
					t.Fatalf("code=%d reads=%d output=%s", code, reads, &out)
				}
			})
		}
	}
}

func TestAdminLabelValueInspectionRejectsWrongReturnedName(t *testing.T) {
	reads := 0
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			return
		}
		reads++
		io.WriteString(w, `<tsResponse><labelValue name="Different" category="Custom" description="Meaning" internal="false" elevatedDefault="false" builtIn="false"/></tsResponse>`)
	}))
	defer server.Close()
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"admin", "label-value", "inspect", "--environment", "production", "--name", "Expected", "--json"}, &out, catalogMetadataOptions(t, server, false))
	if code == 0 || reads != 1 || !strings.Contains(out.String(), "name mismatch") {
		t.Fatalf("code=%d reads=%d output=%s", code, reads, &out)
	}
}

func TestAdminVocabularyMalformedAcknowledgementRetainsReceipt(t *testing.T) {
	for _, kind := range []string{"category", "value"} {
		t.Run(kind, func(t *testing.T) {
			reads, writes := 0, 0
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogMetadataSignIn(w, r) {
					return
				}
				if r.Method == http.MethodGet {
					reads++
					if kind == "category" {
						io.WriteString(w, `<tsResponse><labelCategoryList/></tsResponse>`)
					} else {
						io.WriteString(w, `<tsResponse><labelValueList/></tsResponse>`)
					}
					return
				}
				writes++
				w.Header().Set("X-Tableau-Request-Id", "ack-request")
				io.WriteString(w, `<tsResponse><broken`)
			}))
			defer server.Close()
			args := []string{"admin", "label-category", "create", "--name", "Fixture", "--description", "Meaning"}
			if kind == "value" {
				args = []string{"admin", "label-value", "update", "--name", "Fixture", "--category", "Custom", "--description", "Meaning"}
			}
			var out bytes.Buffer
			code := app.Run(t.Context(), append(args, "--environment", "production", "--json"), &out, catalogMetadataOptions(t, server, true))
			if code == 0 || reads != 2 || writes != 1 {
				t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads, writes, &out)
			}
			for _, evidence := range []string{`"verification_pending"`, `"definition_write"`, `"Fixture"`, `"confirmed"`, `"ack-request"`} {
				if !strings.Contains(out.String(), evidence) {
					t.Errorf("missing %s: %s", evidence, &out)
				}
			}
		})
	}
}
