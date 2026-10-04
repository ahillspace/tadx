package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"github.com/ahillspace/tadx/internal/app"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAdminCreateSubmittedHTTP502IsUnknownThroughCLI(t *testing.T) {
	for _, kind := range []string{"user", "group"} {
		t.Run(kind, func(t *testing.T) {
			var posts int
			var applied bool
			server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				if r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s" {
					_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="100" totalAvailable="0"/><%ss/></tsResponse>`, kind)
					return
				}
				if r.Method == http.MethodPost && r.URL.Path == "/api/3.29/sites/site-1/"+kind+"s" {
					posts++
					applied = true
					w.Header().Set("X-Tableau-Request-Id", "submitted-create-1")
					w.WriteHeader(http.StatusBadGateway)
					_, _ = io.WriteString(w, `<tsResponse><error code="502000"><summary>Gateway failed after apply</summary></error></tsResponse>`)
					return
				}
				t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			}))
			defer server.Close()
			args := []string{"admin", kind, "create", "--environment", "test", "--name", "alice"}
			if kind == "user" {
				args = append(args, "--site-role", "Viewer", "--auth-setting", "ServerDefault")
			}
			var out bytes.Buffer
			code := app.Run(t.Context(), append(args, "--json"), &out, diagnosticOptions(t, server))
			var result struct {
				Error struct {
					ID               string `json:"id"`
					Phase            string `json:"phase"`
					Outcome          string `json:"outcome"`
					Resource         string `json:"resource"`
					TableauRequestID string `json:"tableau_request_id"`
					CorrectiveAction string `json:"corrective_action"`
					Retryable        *bool  `json:"retryable"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &result); err != nil {
				t.Fatal(err)
			}
			e := result.Error
			if e.ID != "admin."+kind+".create.outcome_unknown" || e.Phase != "submission" || e.Outcome != "unknown" || e.Resource != "alice" || e.TableauRequestID != "submitted-create-1" || e.Retryable == nil || *e.Retryable || !strings.Contains(e.CorrectiveAction, "--name alice") {
				t.Errorf("unexpected error payload: %s", out.String())
			}
			if code == 0 || posts != 1 || !applied {
				t.Errorf("code=%d posts=%d applied=%t output=%s", code, posts, applied, out.String())
			}
		})
	}
}
