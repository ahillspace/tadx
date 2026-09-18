package app_test

import (
	"context"
	"encoding/xml"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestWorkbookDescriptionUpdateThroughCLI(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(fmt.Sprint(preview), func(t *testing.T) {
			puts := 0
			description := "Existing description"
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/xml")
				switch {
				case r.URL.Path == "/api/3.29/auth/signin":
					w.Header().Set("Content-Type", "application/json")
					io.WriteString(w, `{"credentials":{"token":"test-token","site":{"id":"site-1","contentUrl":""},"user":{"id":"user-1"}}}`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/projects":
					io.WriteString(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="1"/><projects><project id="project-1" name="Operations"/></projects></tsResponse>`)
				case r.URL.Path == "/api/3.29/sites/site-1/workbooks/wb-1":
					if r.Method == http.MethodPut {
						puts++
						var payload struct {
							Workbook struct {
								Description *string   `xml:"description,attr"`
								Name        *string   `xml:"name,attr"`
								Project     *struct{} `xml:"project"`
								Owner       *struct{} `xml:"owner"`
							} `xml:"workbook"`
						}
						if err := xml.NewDecoder(r.Body).Decode(&payload); err != nil {
							t.Error(err)
						}
						if payload.Workbook.Description == nil || *payload.Workbook.Description != "Sales & margin" || payload.Workbook.Name != nil || payload.Workbook.Project != nil || payload.Workbook.Owner != nil {
							t.Errorf("unexpected update payload: %#v", payload.Workbook)
						}
						description = "Sales &amp; margin"
					} else if r.Method != http.MethodGet {
						t.Errorf("unexpected method %s", r.Method)
					}
					fmt.Fprintf(w, `<tsResponse><workbook id="wb-1" name="Finance" description="%s"><project id="project-1" name="Operations"/><owner id="user-1"/></workbook></tsResponse>`, description)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected request", 404)
				}
			}))
			defer server.Close()
			config := writePhaseOneConfig(t, server.URL)
			t.Setenv("PROD_PAT_NAME", "test-name")
			t.Setenv("PROD_PAT_SECRET", "test-secret")
			args := []string{"content", "workbook", "update", "--environment", "production", "--id", "wb-1", "--description", "Sales & margin"}
			if preview {
				args = append(args, "--preview")
			}
			var output strings.Builder
			if code := app.Run(context.Background(), args, &output, withSiteMutationConsent(t, app.Options{ConfigPath: config, HTTPClient: server.Client()}, !preview)); code != 0 {
				t.Fatalf("exit %d: %s", code, output.String())
			}
			if (preview && puts != 0) || (!preview && puts != 1) {
				t.Fatalf("preview %v: %d writes", preview, puts)
			}
			if !strings.Contains(output.String(), "description") || !strings.Contains(output.String(), "Sales & margin") {
				t.Fatalf("missing decision-ready change: %s", output.String())
			}
		})
	}
}
