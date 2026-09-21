package app_test

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/toon"
)

func decodeAuditOutput(t *testing.T, data []byte, asJSON bool) map[string]any {
	t.Helper()
	var decoded any
	var err error
	if asJSON {
		err = json.Unmarshal(data, &decoded)
	} else {
		decoded, err = toon.Decode(data)
	}
	if err != nil {
		t.Fatalf("decode: %v: %s", err, data)
	}
	// Normalize decoder numeric representations through JSON.
	encoded, err := json.Marshal(decoded)
	if err != nil {
		t.Fatal(err)
	}
	var result map[string]any
	if err := json.Unmarshal(encoded, &result); err != nil {
		t.Fatal(err)
	}
	return result
}

func TestCancellationFailurePhasesAndEncoding(t *testing.T) {
	for _, phase := range []string{"setup", "submission", "verification"} {
		t.Run(phase, func(t *testing.T) {
			var previous map[string]any
			for _, full := range []bool{false, true} {
				for _, asJSON := range []bool{false, true} {
					reads, writes := 0, 0
					server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
						if strings.HasSuffix(r.URL.Path, "/auth/signin") {
							io.WriteString(w, `{"credentials":{"token":"fixture-session","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
							return
						}
						if strings.HasSuffix(r.URL.Path, "/auth/signout") {
							w.WriteHeader(http.StatusNoContent)
							return
						}
						if r.Method == http.MethodPut {
							writes++
							if phase == "submission" {
								http.Error(w, "submission failed", 403)
								return
							}
							w.Header().Set("X-Tableau-Request-Id", "cancel-request")
							return
						}
						reads++
						if phase == "setup" || (phase == "verification" && reads > 1) {
							http.Error(w, "observation failed", 403)
							return
						}
						io.WriteString(w, `<tsResponse><job id="job-1" type="RefreshExtract" progress="50" finishCode="0"/></tsResponse>`)
					}))
					options := catalogMetadataOptions(t, server, true)
					args := []string{"job", "cancel", "--id", "job-1", "--env", "production"}
					if full {
						args = append(args, "--full")
					}
					if asJSON {
						args = append(args, "--json")
					}
					var out bytes.Buffer
					code := app.Run(t.Context(), args, &out, options)
					server.Close()
					result := decodeAuditOutput(t, out.Bytes(), asJSON)
					failure, ok := result["error"].(map[string]any)
					if code == 0 || !ok || failure["phase"] != phase {
						t.Fatalf("code=%d result=%v", code, result)
					}
					outcome := "unknown"
					if phase == "setup" {
						outcome = "not_attempted"
					}
					if failure["outcome"] != outcome || failure["tableau_job_id"] != "job-1" || failure["operation"] != "job.cancel" {
						t.Errorf("failure contract: %v", failure)
					}
					if phase == "setup" && writes != 0 {
						t.Fatal("setup failure submitted cancellation")
					}
					if phase == "verification" && failure["tableau_request_id"] != "cancel-request" {
						t.Errorf("lost accepted request: %v", failure)
					}
					// Config-preserving recovery paths differ between isolated runs.
					delete(failure, "corrective_action")
					if partial, ok := result["output"].(map[string]any); ok {
						delete(partial, "help")
						if job, ok := partial["job"].(map[string]any); ok {
							delete(job, "checked_at")
						}
					}
					if previous != nil && !reflect.DeepEqual(previous, result) {
						t.Errorf("encoding/full changed operation: before=%v after=%v", previous, result)
					}
					previous = result
				}
			}
		})
	}
}
