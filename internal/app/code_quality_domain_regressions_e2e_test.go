package app_test

import (
	"fmt"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestProjectCreateRetainsAcknowledgedIdentityAfterVerificationFailureThroughCLI(t *testing.T) {
	creates := 0
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="0"/><projects/></tsResponse>`, r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/projects"):
			creates++
			w.Header().Set("X-Tableau-Request-Id", "create-receipt")
			w.WriteHeader(http.StatusCreated)
			io.WriteString(w, `<tsResponse><project id="acknowledged-project" name="Operations" description="Observed description"/></tsResponse>`)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	var output strings.Builder
	code := app.Run(t.Context(), []string{"content", "project", "create", "--environment", "test", "--name", "Operations", "--description", "Requested description", "--json", "--full"}, &output, diagnosticOptions(t, server))
	if code == 0 || creates != 1 {
		t.Fatalf("code=%d creates=%d output=%s", code, creates, output.String())
	}
	for _, want := range []string{`"luid":"acknowledged-project"`, `"description":"Observed description"`, `"phase":"verification"`, `"outcome":"confirmed"`, `"retryable":false`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("acknowledged creation missing %s: %s", want, output.String())
		}
	}
}

func TestCatalogTagUpdateRetainsAcknowledgedSubsetThroughCLI(t *testing.T) {
	for _, kind := range []string{"database", "table", "column"} {
		t.Run(kind, func(t *testing.T) {
			tagWrites, reads := 0, 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if catalogMetadataSignIn(w, r) {
					return
				}
				if r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s/asset/tags" {
					tagWrites++
					io.WriteString(w, `<tsResponse><tags><tag label="sales"/></tags></tsResponse>`)
					return
				}
				base := "/api/3.29/sites/site-1/" + kind + "s/asset"
				if kind == "column" {
					base = "/api/3.29/sites/site-1/tables/parent/columns/asset"
				}
				if r.Method == http.MethodGet && r.URL.Path == base {
					reads++
					fmt.Fprintf(w, `<tsResponse><%s id="asset" name="Asset" parentTableId="parent"><tags/></%s></tsResponse>`, kind, kind)
					return
				}
				t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
				w.WriteHeader(http.StatusInternalServerError)
			}))
			defer server.Close()
			args := []string{"catalog", kind, "update", "--env", "production", "--id", "asset", "--add-tag", "sales", "--add-tag", "retail", "--json", "--full"}
			if kind == "column" {
				args = append(args, "--table-id", "parent")
			}
			var output strings.Builder
			code := app.Run(t.Context(), args, &output, catalogMetadataOptions(t, server, true))
			if code == 0 || reads != 2 || tagWrites != 1 {
				t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads, tagWrites, output.String())
			}
			for _, want := range []string{`"tags":["sales"]`, `"tags_observed":false`, `"acknowledged_additions":["sales"]`, `"unconfirmed_additions":["retail"]`} {
				if !strings.Contains(output.String(), want) {
					t.Errorf("partial acknowledgement missing %s: %s", want, output.String())
				}
			}
		})
	}
}

func TestProjectMoveUsesParentIdentityForLiteralSlashNamesThroughCLI(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(fmt.Sprint(preview), func(t *testing.T) {
			reads, writes := 0, 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/projects"):
					reads++
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="2"/><projects><project id="source" name="Ops"/><project id="destination" name="Ops/Reports"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
				case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/projects/source"):
					writes++
					io.WriteString(w, `<tsResponse><project id="source" name="Ops" parentProjectId="destination"/></tsResponse>`)
				default:
					t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusInternalServerError)
				}
			}))
			defer server.Close()
			args := []string{"content", "project", "move", "--environment", "test", "--project-id", "source", "--parent-id", "destination", "--json"}
			if preview {
				args = append(args, "--preview")
			}
			var output strings.Builder
			code := app.Run(t.Context(), args, &output, diagnosticOptions(t, server))
			wantReads, wantWrites := 2, 1
			if preview {
				wantReads, wantWrites = 1, 0
			}
			if code != 0 || reads != wantReads || writes != wantWrites {
				t.Fatalf("code=%d reads=%d writes=%d output=%s", code, reads, writes, output.String())
			}
		})
	}
}

func TestGroupUpdateStopsOnUnconfirmedRenameThroughCLI(t *testing.T) {
	metadataWrites, membershipWrites := 0, 0
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/groups"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><groups><group id="group-1" name="Before"/></groups></tsResponse>`, r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodGet && strings.HasSuffix(r.URL.Path, "/groups/group-1/users"):
			fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><users><user id="old-user" name="Old"/></users></tsResponse>`, r.URL.Query().Get("pageSize"))
		case r.Method == http.MethodPut && strings.HasSuffix(r.URL.Path, "/groups/group-1"):
			metadataWrites++
			io.WriteString(w, `<tsResponse><group id="group-1" name="Before"/></tsResponse>`)
		case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/groups/group-1/users"):
			membershipWrites++
			io.WriteString(w, `<tsResponse><user id="new-user" name="New"/></tsResponse>`)
		case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/groups/group-1/users/old-user"):
			membershipWrites++
			w.WriteHeader(http.StatusNoContent)
		default:
			t.Errorf("unexpected request %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusInternalServerError)
		}
	}))
	defer server.Close()
	var output strings.Builder
	code := app.Run(t.Context(), []string{"admin", "group", "update", "--environment", "test", "--id", "group-1", "--new-name", "Requested", "--set-members", "--member-id", "new-user", "--json", "--full"}, &output, diagnosticOptions(t, server))
	if code == 0 || metadataWrites != 1 || membershipWrites != 0 {
		t.Fatalf("code=%d metadata writes=%d membership writes=%d output=%s", code, metadataWrites, membershipWrites, output.String())
	}
	for _, want := range []string{`"name":"Before"`, `"status":"partial"`, `"phase":"verification"`, `"retryable":false`} {
		if !strings.Contains(output.String(), want) {
			t.Errorf("unconfirmed rename missing %s: %s", want, output.String())
		}
	}
}
