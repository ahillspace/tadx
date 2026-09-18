package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestAdminCreateMalformedAcknowledgementRetainsRecoveryThroughCLI(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		t.Run(kind, func(t *testing.T) {
			var writes atomic.Int32
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.URL.Path != "/api/3.29/sites/site-1/"+kind+"s" {
					t.Errorf("unexpected path %s", r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
					return
				}
				switch r.Method {
				case http.MethodGet:
					fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="1000" totalAvailable="0"/><%ss/></tsResponse>`, kind)
				case http.MethodPost:
					writes.Add(1)
					w.Header().Set("X-Tableau-Request-Id", "create-request")
					w.WriteHeader(http.StatusCreated)
					fmt.Fprint(w, `<tsResponse/>`)
				default:
					t.Errorf("unexpected method %s", r.Method)
				}
			}))
			defer server.Close()
			args := []string{"admin", kind, "create", "--environment", "test", "--name", "New target", "--json"}
			if kind == "user" {
				args = append(args, "--site-role", "Viewer", "--auth-setting", "ServerDefault")
			}
			var out bytes.Buffer
			code := app.Run(t.Context(), args, &out, diagnosticOptions(t, server))
			var result struct {
				Output struct {
					Plan struct{ Mode, Name string } `json:"plan"`
					Help []string                    `json:"help"`
				} `json:"output"`
				Error struct{ Phase, Outcome, TableauRequestID string } `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			if code == 0 || writes.Load() != 1 || result.Output.Plan.Mode != "execute" || result.Output.Plan.Name != "New target" || result.Error.Phase != "submission" || result.Error.Outcome != "unknown" || strings.Contains(out.String(), "Run without --preview") || !strings.Contains(out.String(), "create-request") {
				t.Fatalf("code=%d writes=%d output=%s", code, writes.Load(), out.String())
			}
		})
	}
}
