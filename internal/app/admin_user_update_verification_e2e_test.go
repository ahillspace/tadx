package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestAdminUserUpdateRejectsPartialAcknowledgementThroughCLI(t *testing.T) {
	gets, puts := 0, 0
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-1":
			_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="admin" siteRole="ServerAdministrator"/></tsResponse>`)
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
			gets++
			_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" fullName="Old Name" email="old@example.com" language="fr" locale="fr_FR" siteRole="Viewer"/></tsResponse>`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
			puts++
			w.Header().Set("X-Tableau-Request-Id", "update-request-1")
			_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" fullName="Old Name" email="new@example.com" language="en" locale="en_US" siteRole="Viewer"/></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"admin", "user", "update", "--environment", "test", "--id", "user-2", "--full-name", "New Name", "--email", "new@example.com", "--language", "en", "--locale", "en_US", "--json"}, &out, diagnosticOptions(t, server))
	var result struct {
		Output struct {
			Result struct {
				Status   string `json:"status"`
				UserLUID string `json:"user_luid"`
			} `json:"result"`
		} `json:"output"`
		Error struct {
			ID               string   `json:"id"`
			Phase            string   `json:"phase"`
			Outcome          string   `json:"outcome"`
			Resource         string   `json:"resource"`
			TableauRequestID string   `json:"tableau_request_id"`
			Completed        []string `json:"completed"`
			Failed           string   `json:"failed"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatalf("invalid JSON: %v: %s", err, out.String())
	}
	if code == 0 || puts != 1 || gets != 2 || result.Output.Result.Status != "partial" || result.Output.Result.UserLUID != "user-2" || result.Error.Resource != "user-2" || result.Error.TableauRequestID != "update-request-1" || result.Error.Phase != "verification" || result.Error.Outcome != "confirmed" || result.Error.Failed != "full_name" || !strings.Contains(strings.Join(result.Error.Completed, ","), "email") {
		t.Fatalf("code=%d gets=%d puts=%d result=%+v output=%s", code, gets, puts, result, out.String())
	}
}

func TestAdminUserUpdateVerificationCasesThroughCLI(t *testing.T) {
	for _, tt := range []struct {
		name, before, after string
		fields              []string
		wantStatus          string
		wantFields          []string
		wantPuts            int
	}{
		{"complete", `email="old@example.com" language="fr"`, `email="new@example.com" language="en"`, []string{"--email", "new@example.com", "--language", "en"}, "updated", []string{"confirmed", "confirmed"}, 1},
		{"all mismatched", `email="old@example.com" language="fr"`, `email="old@example.com" language="fr"`, []string{"--email", "new@example.com", "--language", "en"}, "not_applied", []string{"mismatch", "mismatch"}, 1},
		{"one omitted", `email="old@example.com" language="fr"`, `email="new@example.com"`, []string{"--email", "new@example.com", "--language", "en"}, "partial", []string{"confirmed", "unknown"}, 1},
		{"all omitted", `email="old@example.com" language="fr"`, ``, []string{"--email", "new@example.com", "--language", "en"}, "unverified", []string{"unknown", "unknown"}, 1},
		{"explicit empty confirmed", `email="old@example.com"`, `email=""`, []string{"--email", ""}, "updated", []string{"confirmed"}, 1},
		{"omitted before explicit empty", `language="fr"`, `email="" language="en"`, []string{"--email", "", "--language", "en"}, "updated", []string{"confirmed", "confirmed"}, 1},
		{"matched unchanged field", `email="old@example.com" language="fr"`, `email="old@example.com" language="en"`, []string{"--email", "old@example.com", "--language", "en"}, "updated", []string{"matched", "confirmed"}, 1},
		{"documented auth spelling", `email="old@example.com" authSetting="TableauIdWithMFA"`, `email="new@example.com" authSetting="TableauIdWithMFA"`, []string{"--email", "new@example.com", "--auth-setting", "TableauIDWithMFA"}, "updated", []string{"confirmed", "matched"}, 1},
		{"preview", `email="old@example.com"`, `email="new@example.com"`, []string{"--email", "new@example.com", "--preview"}, "", nil, 0},
	} {
		t.Run(tt.name, func(t *testing.T) {
			puts := 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
					_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" `+tt.before+`/></tsResponse>`)
				case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
					puts++
					w.Header().Set("X-Tableau-Request-Id", "update-request-2")
					_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" `+tt.after+`/></tsResponse>`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			defer server.Close()
			args := append([]string{"admin", "user", "update", "--environment", "test", "--id", "user-2"}, tt.fields...)
			args = append(args, "--json")
			var out bytes.Buffer
			code := app.Run(t.Context(), args, &out, diagnosticOptions(t, server))
			var document struct {
				Plan struct {
					Mode string `json:"mode"`
				} `json:"plan"`
				Result struct {
					Status       string `json:"status"`
					FieldResults []struct {
						Field, Status string
						Actual        *string `json:"actual"`
					} `json:"field_results"`
				} `json:"result"`
				Error struct {
					Outcome string `json:"outcome"`
				} `json:"error"`
			}
			if err := json.Unmarshal(out.Bytes(), &document); err != nil {
				t.Fatalf("invalid JSON: %v: %s", err, out.String())
			}
			if code != 0 {
				var wrapped struct {
					Output json.RawMessage `json:"output"`
					Error  struct {
						Outcome string `json:"outcome"`
					} `json:"error"`
				}
				if err := json.Unmarshal(out.Bytes(), &wrapped); err != nil {
					t.Fatal(err)
				}
				if err := json.Unmarshal(wrapped.Output, &document); err != nil {
					t.Fatal(err)
				}
				document.Error.Outcome = wrapped.Error.Outcome
			}
			if puts != tt.wantPuts {
				t.Fatalf("puts=%d output=%s", puts, out.String())
			}
			if tt.name == "preview" {
				if code != 0 || document.Plan.Mode != "preview" {
					t.Fatalf("code=%d output=%s", code, out.String())
				}
				return
			}
			if (code == 0) != (tt.wantStatus == "updated") || document.Result.Status != tt.wantStatus || len(document.Result.FieldResults) != len(tt.wantFields) {
				t.Fatalf("code=%d output=%s", code, out.String())
			}
			for i, want := range tt.wantFields {
				if document.Result.FieldResults[i].Status != want {
					t.Errorf("field %d = %s, want %s: %s", i, document.Result.FieldResults[i].Status, want, out.String())
				}
			}
			if tt.name == "one omitted" && (document.Result.FieldResults[1].Actual != nil || document.Error.Outcome != "unknown") {
				t.Errorf("omitted field not distinguished: %s", out.String())
			}
			if tt.name == "explicit empty confirmed" && (document.Result.FieldResults[0].Actual == nil || *document.Result.FieldResults[0].Actual != "") {
				t.Errorf("explicit empty value lost: %s", out.String())
			}
		})
	}
}

func TestAdminUserUpdateRejectsKnownSiteAdministratorFullNameThroughCLI(t *testing.T) {
	for _, preview := range []bool{true, false} {
		t.Run(map[bool]string{true: "preview", false: "execute"}[preview], func(t *testing.T) {
			puts, callerReads := 0, 0
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
					_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" fullName="Old Name" email="old@example.com"/></tsResponse>`)
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-1":
					callerReads++
					_, _ = io.WriteString(w, `<tsResponse><user id="user-1" name="admin" siteRole="SiteAdministratorCreator"/></tsResponse>`)
				case r.Method == http.MethodPut:
					puts++
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			defer server.Close()
			args := []string{"admin", "user", "update", "--environment", "test", "--id", "user-2", "--full-name", "New Name", "--email", "new@example.com", "--json"}
			if preview {
				args = append(args, "--preview")
			}
			var out bytes.Buffer
			code := app.Run(t.Context(), args, &out, diagnosticOptions(t, server))
			if puts != 0 || (preview && (code != 0 || callerReads != 0)) || (!preview && (code == 0 || callerReads != 1 || !strings.Contains(out.String(), "unsupported_full_name"))) {
				t.Fatalf("preview=%t code=%d puts=%d callerReads=%d output=%s", preview, code, puts, callerReads, out.String())
			}
		})
	}
}

func TestAdminUserUpdatePartialReceiptFormatsThroughCLI(t *testing.T) {
	for _, tt := range []struct {
		name  string
		flags []string
		want  []string
	}{
		{"full JSON", []string{"--full", "--json"}, []string{`"tableau_request_id":"update-request-3"`, `"status":"partial"`, `"status":"mismatch"`, `"status":"confirmed"`}},
		{"compact TOON", nil, []string{"status: partial", "field_results[2]{field,status,requested,actual}", "email,mismatch,new@example.com,old@example.com", "language,confirmed,en,en"}},
		{"full TOON", []string{"--full"}, []string{"status: partial", "field_results[2]{field,status,requested,actual}", "email,mismatch,new@example.com,old@example.com", "language,confirmed,en,en", "tableau_request_id: update-request-3"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if diagnosticSignIn(w, r) {
					return
				}
				switch {
				case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
					_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" email="old@example.com" language="fr"/></tsResponse>`)
				case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
					w.Header().Set("X-Tableau-Request-Id", "update-request-3")
					_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" email="old@example.com" language="en"/></tsResponse>`)
				default:
					t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
					http.Error(w, "unexpected", http.StatusNotFound)
				}
			}))
			defer server.Close()
			args := append([]string{"admin", "user", "update", "--environment", "test", "--id", "user-2", "--email", "new@example.com", "--language", "en"}, tt.flags...)
			var out bytes.Buffer
			code := app.Run(t.Context(), args, &out, diagnosticOptions(t, server))
			if code == 0 {
				t.Fatalf("unexpected success: %s", out.String())
			}
			for _, want := range tt.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q: %s", want, out.String())
				}
			}
		})
	}
}

func TestAdminUserUpdateSubmittedFailureRetainsUnknownOutcomeThroughCLI(t *testing.T) {
	gets, puts := 0, 0
	server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if diagnosticSignIn(w, r) {
			return
		}
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
			gets++
			_, _ = io.WriteString(w, `<tsResponse><user id="user-2" name="alex" email="old@example.com"/></tsResponse>`)
		case r.Method == http.MethodPut && r.URL.Path == "/api/3.29/sites/site-1/users/user-2":
			puts++
			w.Header().Set("X-Tableau-Request-Id", "submitted-update-1")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = io.WriteString(w, `<tsResponse><error code="502000"><summary>Gateway failed after apply</summary></error></tsResponse>`)
		default:
			t.Errorf("unexpected %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected", http.StatusNotFound)
		}
	}))
	defer server.Close()
	var out bytes.Buffer
	code := app.Run(t.Context(), []string{"admin", "user", "update", "--environment", "test", "--id", "user-2", "--email", "new@example.com", "--json"}, &out, diagnosticOptions(t, server))
	var result struct {
		Output struct {
			Result struct {
				Status   string `json:"status"`
				UserLUID string `json:"user_luid"`
			} `json:"result"`
		} `json:"output"`
		Error struct {
			Phase            string `json:"phase"`
			Outcome          string `json:"outcome"`
			TableauRequestID string `json:"tableau_request_id"`
			Retryable        *bool  `json:"retryable"`
		} `json:"error"`
	}
	if err := json.Unmarshal(out.Bytes(), &result); err != nil {
		t.Fatal(err)
	}
	if code == 0 || gets != 2 || puts != 1 || result.Output.Result.Status != "unknown" || result.Output.Result.UserLUID != "user-2" || result.Error.Phase != "submission" || result.Error.Outcome != "unknown" || result.Error.TableauRequestID != "submitted-update-1" || result.Error.Retryable == nil || *result.Error.Retryable {
		t.Fatalf("code=%d gets=%d puts=%d result=%+v output=%s", code, gets, puts, result, out.String())
	}
}
