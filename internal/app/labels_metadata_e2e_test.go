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
	"strings"
	"testing"
)

const labelFixtureBefore = `<label id="label-1" contentId="table-1" contentType="table" value="Warning" category="Custom" message="Before" active="true" elevated="true"/>`
const labelFixtureAfter = `<label id="label-1" contentId="table-1" contentType="table" value="Warning" category="Custom" message="After" active="true" elevated="true"/>`
const valueFixture = `<labelValue name="Warning" category="Custom" description="Meaning" internal="false" elevatedDefault="true" builtIn="false"/>`

func TestLabelsMetadataHTTPPreviewUsesReadOnlyPOST(t *testing.T) {
	reads, writes, auth := 0, 0, 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			auth++
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/labels":
			if r.Method != http.MethodPost {
				writes++
				w.WriteHeader(500)
				return
			}
			reads++
			body, _ := io.ReadAll(r.Body)
			if !strings.Contains(string(body), `contentType="table"`) || strings.Contains(string(body), "<label") {
				t.Errorf("preview request: %s", body)
			}
			io.WriteString(w, "<tsResponse><labelList>"+labelFixtureBefore+"</labelList></tsResponse>")
		case "/api/3.29/sites/site-1/labelValues/Warning":
			io.WriteString(w, "<tsResponse>"+valueFixture+"</tsResponse>")
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"catalog", "label", "update", "--env", "production", "--type", "table", "--target-id", "table-1", "--value", "Warning", "--message", "After", "--preview", "--json"}, &out, catalogMetadataOptions(t, s, false))
	if code != 0 || reads != 1 || writes != 0 || auth != 1 || !strings.Contains(out.String(), `"mode":"preview"`) {
		t.Fatalf("code=%d reads=%d writes=%d auth=%d %s", code, reads, writes, auth, &out)
	}
}

func TestLabelsMetadataHTTPUpdatePreservesFieldsAndReceipt(t *testing.T) {
	for _, failReadback := range []bool{false, true} {
		t.Run(fmt.Sprint(failReadback), func(t *testing.T) {
			writes := 0
			s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogMetadataSignIn(w, r) {
					return
				}
				switch r.URL.Path {
				case "/api/3.29/sites/site-1/labels/label-1":
					switch r.Method {
					case http.MethodPut:
						writes++
						body, _ := io.ReadAll(r.Body)
						for _, part := range []string{`message="After"`, `value="Warning"`, `active="true"`, `elevated="true"`} {
							if !strings.Contains(string(body), part) {
								t.Errorf("missing %s in %s", part, body)
							}
						}
						io.WriteString(w, "<tsResponse>"+labelFixtureAfter+"</tsResponse>")
					case http.MethodGet:
						if writes > 0 && failReadback {
							w.WriteHeader(403)
							io.WriteString(w, `<tsResponse><error code="403004"><summary>Forbidden</summary></error></tsResponse>`)
							return
						}
						v := labelFixtureBefore
						if writes > 0 {
							v = labelFixtureAfter
						}
						io.WriteString(w, "<tsResponse>"+v+"</tsResponse>")
					default:
						t.Errorf("unexpected %s", r.Method)
						w.WriteHeader(500)
					}
				case "/api/3.29/sites/site-1/labelValues/Warning":
					io.WriteString(w, "<tsResponse>"+valueFixture+"</tsResponse>")
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL)
					w.WriteHeader(500)
				}
			}))
			defer s.Close()
			var out bytes.Buffer
			code := app.Run(context.Background(), []string{"catalog", "label", "update", "--env", "production", "--id", "label-1", "--message", "After", "--json"}, &out, catalogMetadataOptions(t, s, true))
			if (code != 0) != failReadback || writes != 1 || !json.Valid(out.Bytes()) || !strings.Contains(out.String(), "label-1") {
				t.Fatalf("code=%d writes=%d %s", code, writes, &out)
			}
			if failReadback && (!strings.Contains(out.String(), "confirmed") || !strings.Contains(out.String(), "label_write")) {
				t.Fatalf("lost confirmed receipt: %s", &out)
			}
		})
	}
}

func TestLabelsMetadataHTTPAdminNativeValueUpsert(t *testing.T) {
	writes := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/labelValues":
			if r.Method == http.MethodGet {
				io.WriteString(w, `<tsResponse><labelValueList/></tsResponse>`)
				return
			}
			if r.Method != http.MethodPut {
				t.Errorf("unexpected method %s", r.Method)
				w.WriteHeader(500)
				return
			}
			writes++
			body, _ := io.ReadAll(r.Body)
			for _, readonly := range []string{"elevatedDefault", "builtIn", "internal"} {
				if strings.Contains(string(body), readonly) {
					t.Errorf("read-only property serialized: %s", body)
				}
			}
			io.WriteString(w, "<tsResponse>"+valueFixture+"</tsResponse>")
		case "/api/3.29/sites/site-1/labelValues/Warning":
			io.WriteString(w, "<tsResponse>"+valueFixture+"</tsResponse>")
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"admin", "label-value", "update", "--env", "production", "--name", "Warning", "--category", "Custom", "--description", "Meaning", "--json"}, &out, catalogMetadataOptions(t, s, true))
	if code != 0 || writes != 1 {
		t.Fatalf("code=%d writes=%d %s", code, writes, &out)
	}
}

func TestLabelsMetadataHTTPAdminCategoryExactNameAndMembers(t *testing.T) {
	writes := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if catalogMetadataSignIn(w, r) {
			return
		}
		if r.Method != http.MethodGet {
			writes++
			w.WriteHeader(500)
			return
		}
		switch r.URL.Path {
		case "/api/3.29/sites/site-1/labelCategories":
			io.WriteString(w, `<tsResponse><labelCategoryList><labelCategory name="Custom" description="Meaning"/></labelCategoryList></tsResponse>`)
		case "/api/3.29/sites/site-1/labelValues":
			io.WriteString(w, "<tsResponse><labelValueList>"+valueFixture+"</labelValueList></tsResponse>")
		default:
			t.Errorf("unexpected %s", r.URL)
			w.WriteHeader(500)
		}
	}))
	defer s.Close()
	opts := catalogMetadataOptions(t, s, true)
	for _, args := range [][]string{{"admin", "label", "category", "inspect", "--name", "custom"}, {"admin", "label", "category", "delete", "--name", "Custom"}} {
		var out bytes.Buffer
		args = append(args, "--env", "production", "--json")
		code := app.Run(context.Background(), args, &out, opts)
		if code == 0 || writes != 0 {
			t.Fatalf("code=%d writes=%d %s", code, writes, &out)
		}
	}
}

func TestLabelsMetadataLocalErrorsBeforeAuthentication(t *testing.T) {
	calls := 0
	s := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { calls++; w.WriteHeader(500) }))
	defer s.Close()
	opts := catalogMetadataOptions(t, s, true)
	for _, args := range [][]string{
		{"catalog", "label", "list", "--type", "workbook", "--target-id", "book"},
		{"catalog", "label", "delete", "--id", "label", "--type", "workbook", "--target-id", "book"},
		{"catalog", "label", "update", "--id", "label"},
		{"admin", "label", "value", "update", "--name", "Warning", "--elevated-default"},
		{"admin", "label", "category", "create", "--name", "Custom"},
	} {
		var out bytes.Buffer
		args = append(args, "--env", "production", "--json")
		if code := app.Run(context.Background(), args, &out, opts); code == 0 || calls != 0 {
			t.Fatalf("code=%d calls=%d %s", code, calls, &out)
		}
	}
}
