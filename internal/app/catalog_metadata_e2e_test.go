package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
)

func catalogMetadataOptions(t *testing.T, s *httptest.Server, mutations bool) app.Options {
	t.Helper()
	t.Setenv("PROD_PAT_NAME", "fixture-name")
	t.Setenv("PROD_PAT_SECRET", "fixture-secret")
	home := t.TempDir()
	return app.Options{ConfigPath: writePhaseOneConfig(t, s.URL), HTTPClient: s.Client(), UserHomeDir: func() (string, error) { return home, nil }, MutationsEnabled: mutations, MutationEnvironment: func() (string, bool) {
		if mutations {
			return "1", true
		}
		return "0", true
	}}
}
func catalogMetadataSignIn(w http.ResponseWriter, r *http.Request) bool {
	if r.URL.Path != "/api/3.29/auth/signin" {
		return false
	}
	w.Header().Set("Content-Type", "application/json")
	io.WriteString(w, `{"credentials":{"token":"fixture-token","site":{"id":"site-1","contentUrl":""},"user":{"id":"user-1"}}}`)
	return true
}
func TestCatalogMetadataReadProjectionsAndBounds(t *testing.T) {
	for _, full := range []bool{false, true} {
		t.Run(fmt.Sprint(full), func(t *testing.T) {
			calls, signins := 0, 0
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogMetadataSignIn(w, r) {
					signins++
					return
				}
				calls++
				if r.URL.Path != "/api/metadata/graphql" || r.Method != http.MethodPost {
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					w.WriteHeader(500)
					return
				}
				var body struct{ Variables map[string]any }
				if e := json.NewDecoder(r.Body).Decode(&body); e != nil {
					t.Error(e)
				}
				if body.Variables["first"] != float64(1) {
					t.Errorf("requested unbounded read: %+v", body)
				}
				io.WriteString(w, `{"data":{"items":{"totalCount":2,"pageInfo":{"hasNextPage":true,"endCursor":"private-cursor"},"nodes":[{"id":"metadata-db","luid":"db","name":"Warehouse","description":"Detailed asset meaning"}]}}}`)
			}))
			defer s.Close()
			opts := catalogMetadataOptions(t, s, false)
			args := []string{"catalog", "database", "list", "--env", "production", "--limit", "1", "--json"}
			if full {
				args = append(args, "--full")
			}
			var out bytes.Buffer
			if code := app.Run(context.Background(), args, &out, opts); code != 0 {
				t.Fatalf("exit %d %s", code, &out)
			}
			if calls != 1 || signins != 1 || !json.Valid(out.Bytes()) || strings.Contains(out.String(), "private-cursor") {
				t.Fatalf("calls=%d signins=%d %s", calls, signins, &out)
			}
			if full != strings.Contains(out.String(), "Detailed asset meaning") {
				t.Fatalf("full=%v projection: %s", full, &out)
			}
			if !strings.Contains(out.String(), `"more_available":true`) {
				t.Fatalf("missing continuation fact: %s", &out)
			}
		})
	}
}
func TestCatalogMetadataUpdatePreviewGateAndExecution(t *testing.T) {
	for _, preview := range []bool{false, true} {
		t.Run(fmt.Sprint(preview), func(t *testing.T) {
			writes, reads, signins := 0, 0, 0
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogMetadataSignIn(w, r) {
					signins++
					return
				}
				if r.URL.Path != "/api/3.29/sites/site-1/databases/db" {
					t.Errorf("unexpected path %s", r.URL)
					w.WriteHeader(500)
					return
				}
				switch r.Method {
				case http.MethodGet:
					reads++
					io.WriteString(w, `<tsResponse><database id="db" name="Warehouse" description="Before"><contact id="unchanged"/></database></tsResponse>`)
				case http.MethodPut:
					writes++
					b, _ := io.ReadAll(r.Body)
					if !strings.Contains(string(b), `description="After"`) || strings.Contains(string(b), "contact") {
						t.Errorf("unexpected patch %s", b)
					}
					io.WriteString(w, `<tsResponse><database id="db" name="Warehouse" description="After"/></tsResponse>`)
				default:
					t.Errorf("unexpected method %s", r.Method)
					w.WriteHeader(500)
				}
			}))
			defer s.Close()
			opts := catalogMetadataOptions(t, s, !preview)
			args := []string{"catalog", "database", "update", "--env", "production", "--id", "db", "--description", "After", "--json"}
			if preview {
				args = append(args, "--preview")
			}
			var out bytes.Buffer
			if code := app.Run(context.Background(), args, &out, opts); code != 0 {
				t.Fatalf("exit %d %s", code, &out)
			}
			expectedWrites, expectedReads := 1, 2
			if preview {
				expectedWrites, expectedReads = 0, 1
			}
			if writes != expectedWrites || reads != expectedReads || signins != 1 {
				t.Fatalf("preview %v writes %d reads %d signins %d %s", preview, writes, reads, signins, &out)
			}
		})
	}
}
func TestCatalogMetadataBatchOrderedPartialAndOneSession(t *testing.T) {
	signins := 0
	written := []string{}
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			signins++
			return
		}
		id := strings.TrimPrefix(r.URL.Path, "/api/3.29/sites/site-1/databases/")
		if id == "missing" {
			w.WriteHeader(404)
			io.WriteString(w, `<tsResponse><error code="404030"><summary>Not found</summary></error></tsResponse>`)
			return
		}
		description := "Before"
		if r.Method == http.MethodPut {
			written = append(written, id)
			description = "After"
		} else if r.Method != http.MethodGet {
			t.Errorf("unexpected %s", r.Method)
		}
		fmt.Fprintf(w, `<tsResponse><database id="%s" name="Warehouse" description="%s"/></tsResponse>`, id, description)
	}))
	defer s.Close()
	var out bytes.Buffer
	opts := catalogMetadataOptions(t, s, true)
	code := app.Run(context.Background(), []string{"catalog", "database", "update", "--env", "production", "--id", "first", "--id", "missing", "--id", "last", "--description", "After", "--json"}, &out, opts)
	if code == 0 || signins != 1 || !reflect.DeepEqual(written, []string{"first", "last"}) || !json.Valid(out.Bytes()) {
		t.Fatalf("code%d signins%d writes%v %s", code, signins, written, &out)
	}
	var result struct {
		Succeeded, Failed int
		Items             []json.RawMessage
	}
	if e := json.Unmarshal(out.Bytes(), &result); e != nil {
		t.Fatal(e)
	}
	if result.Succeeded != 2 || result.Failed != 1 || len(result.Items) != 3 {
		t.Fatalf("succeeded=%d failed=%d items=%d %s", result.Succeeded, result.Failed, len(result.Items), &out)
	}
}
func TestCatalogMetadataRejectsLocalErrorsBeforeAuth(t *testing.T) {
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer s.Close()
	opts := catalogMetadataOptions(t, s, true)
	cases := [][]string{{"database", "list", "--limit", "-1"}, {"database", "inspect", "--id", "one", "--metadata-id", "two"}, {"column", "update", "--id", "column", "--description", "Text"}, {"database", "update", "--id", "db", "--description", ""}}
	for _, args := range cases {
		var out bytes.Buffer
		code := app.Run(context.Background(), append([]string{"catalog"}, args...), &out, opts)
		if code == 0 || calls != 0 {
			t.Fatalf("%v code%d calls%d %s", args, code, calls, &out)
		}
	}
}
func TestCatalogMetadataWrongIdentityNeverWrites(t *testing.T) {
	writes := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			writes++
		}
		io.WriteString(w, `<tsResponse><database id="unrelated" name="Wrong target" description="Before"/></tsResponse>`)
	}))
	defer s.Close()
	opts := catalogMetadataOptions(t, s, true)
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"catalog", "database", "update", "--env", "production", "--id", "expected", "--description", "After"}, &out, opts)
	if code == 0 || writes != 0 {
		t.Fatalf("code%d writes%d %s", code, writes, &out)
	}
}
