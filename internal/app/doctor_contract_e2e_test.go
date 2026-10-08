package app_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync/atomic"
	"testing"

	doctorrun "github.com/ahillspace/tadx/actions/doctor/run"
	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/toon"
)

func TestDoctorProductionProbeContracts(t *testing.T) {
	for _, test := range []struct {
		name          string
		environment   string
		workspace     string
		partialPAT    bool
		authFailure   bool
		code          int
		statuses      []doctorrun.Status
		workspaceText string
	}{
		{name: "unknown alias", environment: "missing", code: 1, statuses: []doctorrun.Status{"pass", "fail", "blocked", "blocked", "blocked", "blocked", "warn"}},
		{name: "partial default PAT", partialPAT: true, code: 1, statuses: []doctorrun.Status{"pass", "fail", "blocked", "info", "warn", "warn"}, workspaceText: "Workspace status could not be resolved."},
		{name: "authentication failure", authFailure: true, code: 1, statuses: []doctorrun.Status{"pass", "pass", "fail", "info", "warn", "warn"}, workspaceText: "Workspace status could not be resolved."},
		{name: "omitted workspace", statuses: []doctorrun.Status{"pass", "pass", "pass", "info", "warn", "warn"}, workspaceText: "Workspace status could not be resolved."},
		{name: "explicit missing workspace", workspace: "missing", code: 1, statuses: []doctorrun.Status{"pass", "pass", "pass", "info", "fail", "warn"}, workspaceText: "The selected workspace could not be resolved."},
	} {
		t.Run(test.name, func(t *testing.T) {
			var requests atomic.Int32
			server := tableauFixtureServer(t, http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				requests.Add(1)
				if r.URL.Path != "/api/3.29/auth/signin" {
					t.Errorf("unexpected request: %s", r.URL.Path)
				}
				w.Header().Set("Content-Type", "application/json")
				if test.authFailure {
					w.WriteHeader(http.StatusUnauthorized)
					_, _ = io.WriteString(w, `{"error":{"code":"401001","summary":"private-auth-detail","detail":"private-auth-detail"}}`)
					return
				}
				_, _ = io.WriteString(w, `{"credentials":{"token":"private-session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
			}))
			defer server.Close()
			t.Setenv("TADX_DOCTOR_FIXTURE_PAT_NAME", "private-pat-name")
			secret := "private-pat-secret"
			if test.partialPAT {
				secret = ""
			}
			t.Setenv("TADX_DOCTOR_FIXTURE_PAT_SECRET", secret)
			t.Setenv("TADX_LOG_LEVEL", " ")
			directory := t.TempDir()
			path := filepath.Join(directory, "settings.yaml")
			configuration := fmt.Sprintf("version: 1\ndefault_environment: doctor-fixture\nenvironments:\n  doctor-fixture:\n    url: %s\n    auth:\n      type: pat\n", server.URL)
			if err := os.WriteFile(path, []byte(configuration), 0600); err != nil {
				t.Fatal(err)
			}
			for _, encoding := range []string{"json", "toon"} {
				t.Run(encoding, func(t *testing.T) {
					requests.Store(0)
					args := []string{"doctor", "--full"}
					if encoding == "json" {
						args = append(args, "--json")
					}
					if test.environment != "" {
						args = append(args, "--environment", test.environment)
					}
					if test.workspace != "" {
						args = append(args, "--workspace", test.workspace)
					}
					var out bytes.Buffer
					code := app.Run(t.Context(), args, &out, app.Options{ConfigPath: path, HTTPClient: server.Client(), UserHomeDir: func() (string, error) { return directory, nil }})
					body := out.Bytes()
					if encoding == "toon" {
						decoded, err := toon.Decode(body)
						if err != nil {
							t.Fatal(err)
						}
						body, err = json.Marshal(decoded)
						if err != nil {
							t.Fatal(err)
						}
					}
					var result doctorrun.Output
					if err := json.Unmarshal(body, &result); err != nil {
						t.Fatalf("decode: %v\n%s", err, out.String())
					}
					ids := []string{"config.valid", "auth.pat.references", "auth.tableau.connectivity", "cache.status", "workspace.status", "logging.context"}
					if test.environment != "" {
						ids = slices.Insert(ids, 1, "config.selection")
					}
					if code != test.code || len(result.Checks) != len(ids) {
						t.Fatalf("code=%d output=%s", code, out.String())
					}
					for index, id := range ids {
						if result.Checks[index].ID != id || result.Checks[index].Status != test.statuses[index] {
							t.Fatalf("check %d=%+v", index, result.Checks[index])
						}
					}
					for _, value := range []string{"private-pat-name", "private-pat-secret", "private-session-token", "private-auth-detail"} {
						if strings.Contains(out.String(), value) {
							t.Fatalf("private value exposed: %s", out.String())
						}
					}
					if test.environment != "" {
						if requests.Load() != 0 || !strings.Contains(result.Checks[1].Summary, "missing") || result.Checks[1].CorrectiveAction == "" || result.Checks[2].PAT != nil {
							t.Fatalf("selection failure=%+v requests=%d", result, requests.Load())
						}
						for _, check := range result.Checks[2:6] {
							if check.BlockedBy != "config.selection" {
								t.Fatalf("blocked check=%+v", check)
							}
						}
						return
					}
					wantRequests := int32(1)
					source := "environment"
					if test.partialPAT {
						wantRequests, source = 0, "incomplete_environment"
						if result.Checks[2].BlockedBy != "auth.pat.references" {
							t.Fatalf("connectivity=%+v", result.Checks[2])
						}
					}
					if requests.Load() != wantRequests || result.Checks[4].Summary != test.workspaceText {
						t.Fatalf("requests=%d workspace=%+v", requests.Load(), result.Checks[4])
					}
					if test.authFailure && result.Checks[2].Summary != "Tableau connectivity or PAT authentication failed." {
						t.Fatalf("connectivity=%+v", result.Checks[2])
					}
					var projection struct {
						Checks []struct{ PAT map[string]any }
					}
					if err := json.Unmarshal(body, &projection); err != nil {
						t.Fatal(err)
					}
					wantPAT := map[string]any{"references_configured": true, "name_variable_present": true, "secret_variable_present": !test.partialPAT, "stored_reference_configured": false, "name_variable": "TADX_DOCTOR_FIXTURE_PAT_NAME", "secret_variable": "TADX_DOCTOR_FIXTURE_PAT_SECRET", "source": source}
					if !reflect.DeepEqual(projection.Checks[1].PAT, wantPAT) {
						t.Fatalf("PAT projection=%#v, want=%#v", projection.Checks[1].PAT, wantPAT)
					}
				})
			}
		})
	}
}
