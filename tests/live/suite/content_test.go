package suite

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/tests/live/harness"
)

// Build a separate candidate process so these checks exercise the actual harness
// process boundary, argument forwarding, REST oracle, and mismatch handling.
func contentCandidate(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, "candidate.go")
	source := `package main
import("fmt";"os")
func main(){fmt.Print(os.Getenv("LIVE_CANDIDATE_JSON"))}`
	if err := os.WriteFile(path, []byte(source), 0600); err != nil {
		t.Fatal(err)
	}
	binary := filepath.Join(root, "candidate")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}
	cmd := exec.CommandContext(t.Context(), "go", "build", "-o", binary, path)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("build candidate process: %v %s", err, output)
	}
	return binary
}

func TestContentJobInspectRejectsWrongAcceptedIdentity(t *testing.T) {
	binary := contentCandidate(t)
	for _, id := range []string{"job-accepted", "different-job"} {
		t.Run(id, func(t *testing.T) {
			submissions, reads := 0, 0
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "POST" && r.URL.Path == "/api/3.29/sites/site/jobs/submit":
					submissions++
					_, _ = w.Write([]byte(`{"job":{"id":"job-accepted"}}`))
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/jobs/job-accepted"):
					reads++
					_, _ = w.Write([]byte(`{"job":{"id":"job-accepted","type":"PublishWorkbook","progress":"100","finishCode":"0"}}`))
				default:
					t.Errorf("unexpected independent request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(404)
				}
			}))
			defer server.Close()
			root := t.TempDir()
			journal, err := harness.NewJournal(filepath.Join(root, "journal.json"), "run", "target", "tadx-live-case")
			if err != nil {
				t.Fatal(err)
			}
			result, _ := json.Marshal(map[string]any{"job": map[string]any{"id": id, "type": "PublishWorkbook", "status": "succeeded", "progress": 100}})
			c := &harness.Case{Root: "root-project", RunDir: root, ConfigPath: filepath.Join(root, "config.json"), Binary: binary, Environment: "fixture", Journal: journal, API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, ExtraEnv: []string{"LIVE_CANDIDATE_JSON=" + string(result)}, Profile: harness.Profile{Budgets: harness.Budgets{CommandSeconds: 20}, Bindings: map[string]harness.Binding{"job-inspect": {Params: map[string]string{"submit_method": "POST", "submit_path": "/api/3.29/sites/site/jobs/submit"}}}}}
			err = contentJob("inspect")(t.Context(), c)
			if (err == nil) != (id == "job-accepted") {
				t.Fatalf("scenario error=%v for candidate identity %s", err, id)
			}
			if submissions != 1 || reads != 2 {
				t.Fatalf("submission/read counts=%d/%d; scenario must not resubmit", submissions, reads)
			}
			if len(journal.Jobs) != 1 || journal.Jobs[0] != "job-accepted" {
				t.Fatalf("journal loses accepted identity: %#v", journal.Jobs)
			}
		})
	}
}

func TestWorkbookInspectUsesRetainedReadOnlyFixtures(t *testing.T) {
	binary := contentCandidate(t)
	for _, selected := range []string{"primary", "decoy"} {
		t.Run(selected, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch {
				case r.Method == "GET" && strings.Contains(r.URL.Path, "/workbooks/"):
					id := filepath.Base(r.URL.Path)
					_ = json.NewEncoder(w).Encode(map[string]any{"workbook": map[string]any{"id": id, "name": "Shared exact name", "project": map[string]any{"id": "project-" + id}, "owner": map[string]any{"id": "actor"}}})
				case r.Method == "GET" && strings.HasSuffix(r.URL.Path, "/projects"):
					_, _ = w.Write([]byte(`{"pagination":{"totalAvailable":1},"projects":{"project":[{"id":"project-primary","name":"Primary project"}]}}`))
				case r.Method == "POST" && r.URL.Path == "/api/metadata/graphql":
					_, _ = w.Write([]byte(`{"data":{"workbooks":{"totalCount":1,"pageInfo":{"hasNextPage":false},"nodes":[{"id":"metadata-primary","luid":"primary","name":"Shared exact name"}]}}}`))
				default:
					t.Errorf("retained inspect submits a mutation or unexpected request: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(400)
				}
			}))
			defer server.Close()
			root := t.TempDir()
			result, _ := json.Marshal(map[string]any{"workbook": map[string]any{"luid": selected, "name": "Shared exact name", "project_luid": "project-" + selected, "owner_luid": "actor"}, "datasource_status": "complete"})
			c := &harness.Case{RunDir: root, ConfigPath: filepath.Join(root, "config.json"), Binary: binary, Environment: "fixture", API: &harness.Client{Server: server.URL, Version: "3.29", SiteID: "site", HTTP: server.Client()}, ExtraEnv: []string{"LIVE_CANDIDATE_JSON=" + string(result)}, Profile: harness.Profile{Budgets: harness.Budgets{CommandSeconds: 20, ObservationSeconds: 1}, Bindings: map[string]harness.Binding{
				"workbook-inspect-primary": {ID: "primary", Name: "Shared exact name", Params: map[string]string{"project_id": "project-primary"}},
				"workbook-inspect-decoy":   {ID: "decoy", Name: "Shared exact name", Params: map[string]string{"project_id": "project-decoy"}},
			}}}
			if err := contentInspect(t.Context(), c, "workbook"); (err == nil) != (selected == "primary") {
				t.Fatalf("retained inspect identity %s: %v", selected, err)
			}
		})
	}
}

func TestContentManifestRejectsDuplicateAndWrongProvenance(t *testing.T) {
	expected := map[string]any{"fields": []any{map[string]any{"id": "a", "type": "integer", "source": map[string]any{"id": "parent-a"}}, map[string]any{"id": "b", "type": "string", "source": map[string]any{"id": "parent-b"}}}}
	for name, actual := range map[string]map[string]any{
		"reordered-valid": {"fields": []any{map[string]any{"id": "b", "type": "string", "source": map[string]any{"id": "parent-b"}}, map[string]any{"id": "a", "type": "integer", "source": map[string]any{"id": "parent-a"}}}},
		"duplicate":       {"fields": []any{map[string]any{"id": "a", "type": "integer", "source": map[string]any{"id": "parent-a"}}, map[string]any{"id": "a", "type": "integer", "source": map[string]any{"id": "parent-a"}}}},
		"wrong-source":    {"fields": []any{map[string]any{"id": "a", "type": "integer", "source": map[string]any{"id": "parent-b"}}, map[string]any{"id": "b", "type": "string", "source": map[string]any{"id": "parent-a"}}}},
	} {
		t.Run(name, func(t *testing.T) {
			err := contentSubset(actual, expected)
			if (err == nil) != (name == "reordered-valid") {
				t.Fatalf("manifest mismatch result=%v", err)
			}
		})
	}
}

func TestContentJobStateRequiresCompletionEvidence(t *testing.T) {
	for _, tc := range []struct {
		fields map[string]any
		want   string
	}{
		{map[string]any{"type": "ExtractRefresh", "progress": "0", "finishCode": "0"}, "pending"},
		{map[string]any{"type": "ExtractRefresh", "progress": "64", "finishCode": "0"}, "running"},
		{map[string]any{"type": "ExtractRefresh", "progress": "100", "finishCode": "0"}, "succeeded"},
		{map[string]any{"type": "ExtractRefresh", "progress": "100", "finishCode": "2"}, "cancelled"},
		{map[string]any{"type": "BridgeRefresh", "progress": "100", "finishCode": "0"}, "running"},
		{map[string]any{"finishCode": "0"}, "unknown"},
	} {
		if got := contentJobState(harness.Object{Fields: tc.fields}); got != tc.want {
			t.Errorf("state(%v)=%s, want %s", tc.fields, got, tc.want)
		}
	}
}
