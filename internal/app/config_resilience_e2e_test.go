package app

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	coreauth "github.com/ahillspace/tadx/internal/auth"
	"github.com/ahillspace/tadx/internal/config"
	"gopkg.in/yaml.v3"
)

const resilienceRejectedValue = "fixtureRejectedPAT==:not-a-variable"

func resilienceConfig(t *testing.T, extra string) string {
	t.Helper()
	t.Setenv("GOOD_PAT_NAME", "")
	t.Setenv("GOOD_PAT_SECRET", "")
	path := filepath.Join(t.TempDir(), "config.yaml")
	data := "version: 1\ndefault_environment: good\nenvironments:\n  good:\n    url: https://tableau.example.test\n    auth:\n      type: pat\n      pat_name_env: GOOD_PAT_NAME\n      pat_secret_env: GOOD_PAT_SECRET\n  broken:\n    url: https://tableau.example.test\n    auth:\n      type: pat\n      pat_secret_env: \"" + resilienceRejectedValue + "\"\n" + extra
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func resilienceRun(t *testing.T, options Options, args ...string) (int, string) {
	t.Helper()
	var out, stderr strings.Builder
	options.Stderr = &stderr
	if options.HTTPClient == nil {
		options.HTTPClient = &http.Client{Transport: &warningTestNoNetwork{}}
	}
	code := Run(t.Context(), args, &out, options)
	for _, text := range []string{out.String(), stderr.String()} {
		if strings.Contains(text, resilienceRejectedValue) {
			t.Fatal("CLI exposed a rejected PAT reference")
		}
	}
	if strings.Contains(strings.Join(args, " "), "--json") && !json.Valid([]byte(out.String())) {
		t.Fatalf("CLI did not return parseable JSON: %s", out.String())
	}
	return code, out.String()
}

func resilienceRequire(t *testing.T, options Options, args ...string) string {
	t.Helper()
	code, out := resilienceRun(t, options, args...)
	if code != 0 {
		t.Fatalf("%v failed with exit %d: %s", args, code, out)
	}
	return out
}

func resilienceReplace(t *testing.T, path, old, replacement string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(strings.ReplaceAll(string(data), old, replacement)), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestConfigResilienceDatasourcePreviewThroughCLI(t *testing.T) {
	var requests []string
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requests = append(requests, r.Method+" "+r.URL.Path)
		switch r.URL.Path {
		case "/api/2.4/serverinfo":
			_, _ = io.WriteString(w, `<tsResponse><serverInfo><restApiVersion>3.29</restApiVersion></serverInfo></tsResponse>`)
		case "/api/3.29/auth/signin":
			w.Header().Set("Content-Type", "application/json")
			_, _ = io.WriteString(w, `{"credentials":{"token":"session-token","site":{"id":"site-1"},"user":{"id":"user-1"}}}`)
		case "/api/3.29/sites/site-1/datasources/datasource-1":
			_, _ = io.WriteString(w, `<tsResponse><datasource id="datasource-1" name="Example"><project id="project-1" name="Shared"/></datasource></tsResponse>`)
		case "/api/3.29/sites/site-1/projects":
			_, _ = fmt.Fprintf(w, `<tsResponse><pagination pageNumber="1" pageSize="%s" totalAvailable="1"/><projects><project id="project-1" name="Shared"/></projects></tsResponse>`, r.URL.Query().Get("pageSize"))
		default:
			t.Errorf("unexpected preview request: %s %s", r.Method, r.URL.Path)
			http.Error(w, "unexpected preview request", http.StatusBadRequest)
		}
	}))
	defer server.Close()
	path := resilienceConfig(t, "")
	resilienceReplace(t, path, "https://tableau.example.test", server.URL)
	options := Options{ConfigPath: path, HTTPClient: server.Client(), PATStore: &fakePATStore{}}
	root := filepath.Join(t.TempDir(), "workspace")
	resilienceRequire(t, options, "workspace", "create", "work", "--path", root)
	t.Setenv("GOOD_PAT_NAME", "fixture-name")
	t.Setenv("GOOD_PAT_SECRET", "fixture-secret")
	code, invalid := resilienceRun(t, options, "content", "datasource", "pull", "--id", "datasource-1", "--env", "broken", "--workspace", "work", "--preview", "--json")
	if code == 0 || len(requests) != 0 || !strings.Contains(invalid, "setup") || !strings.Contains(invalid, "not_attempted") || !strings.Contains(invalid, "pat_secret_env") {
		t.Fatalf("invalid preview target reached Tableau or lost setup failure: %s", invalid)
	}
	for _, flags := range [][]string{{"--json"}, {"--full"}} {
		args := append([]string{"content", "datasource", "pull", "--id", "datasource-1", "--env", "good", "--workspace", "work", "--preview"}, flags...)
		out := resilienceRequire(t, options, args...)
		for _, want := range []string{"preview", "broken", "pat_secret_env", "revok"} {
			if !strings.Contains(out, want) {
				t.Fatalf("preview omitted %q: %s", want, out)
			}
		}
		last := resilienceRequire(t, options, "last", "--full", "--json")
		if !strings.Contains(last, "broken") || !strings.Contains(last, "pat_secret_env") {
			t.Fatalf("saved result lost invalid-entry warning: %s", last)
		}
		var saved struct {
			Metadata map[string]any `json:"metadata"`
			Result   struct {
				Metadata struct {
					Warnings []string `json:"warnings"`
				} `json:"metadata"`
			} `json:"result"`
		}
		if err := json.Unmarshal([]byte(last), &saved); err != nil || len(saved.Metadata) != 0 || len(saved.Result.Metadata.Warnings) != 1 {
			t.Fatalf("replay duplicated configuration warnings: %s err=%v", last, err)
		}
		if repeated := resilienceRequire(t, options, "last", "--json"); repeated != last {
			t.Fatal("replay changed saved configuration warnings")
		}
	}
	if len(requests) == 0 {
		t.Fatal("preview did not resolve its remote datasource")
	}
	entries, err := os.ReadDir(filepath.Join(root, "artifacts", "datasource"))
	if err != nil && !os.IsNotExist(err) {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatal("preview acquired a datasource artifact")
	}
}

func TestConfigResilienceInvalidSelectionAndDefaultRepair(t *testing.T) {
	path := resilienceConfig(t, "")
	options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
	for _, selected := range []bool{true, false} {
		args := []string{"auth", "status", "--json"}
		if selected {
			args = append(args, "--env", "broken")
		} else {
			resilienceReplace(t, path, "default_environment: good", "default_environment: broken")
		}
		code, out := resilienceRun(t, options, args...)
		if code == 0 {
			t.Fatal("invalid environment was usable")
		}
		for _, want := range []string{"broken", "pat_secret_env", "setup", "not_attempted", "revok", "env update broken", "env remove broken", "auth logout"} {
			if !strings.Contains(out, want) {
				t.Fatalf("invalid selection omitted %q: %s", want, out)
			}
		}
		var failure struct {
			Error struct {
				CorrectiveAction string `json:"corrective_action"`
			} `json:"error"`
		}
		if err := json.Unmarshal([]byte(out), &failure); err != nil || strings.Count(failure.Error.CorrectiveAction, "--config") != 3 || strings.Count(failure.Error.CorrectiveAction, path) != 3 {
			t.Fatalf("repair commands lost selected configuration: %s err=%v", out, err)
		}
	}
	resilienceRequire(t, options, "env", "set-default", "good", "--json")
	resilienceRequire(t, options, "auth", "status", "--json")
}

func TestConfigResilienceRepairAndInspectionThroughCLI(t *testing.T) {
	for _, operation := range []string{"remove", "update", "get", "list", "doctor"} {
		t.Run(operation, func(t *testing.T) {
			path := resilienceConfig(t, "")
			options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
			switch operation {
			case "remove":
				resilienceRequire(t, options, "env", "remove", "broken", "--json")
				out := resilienceRequire(t, options, "env", "list", "--json", "--full")
				if strings.Contains(out, "broken") {
					t.Fatalf("removed entry remained visible: %s", out)
				}
			case "update":
				code, out := resilienceRun(t, options, "env", "update", "broken", "--site", "changed", "--json")
				if code == 0 || !strings.Contains(out, "pat_secret_env") {
					t.Fatalf("partial repair accepted or lost remaining violation: %s", out)
				}
				resilienceRequire(t, options, "env", "update", "broken", "--pat-secret-env", "VALID_NAME", "--json")
				resilienceRequire(t, options, "auth", "status", "--env", "broken", "--json")
			default:
				args := []string{operation, "--json", "--full"}
				if operation == "get" {
					args = []string{"env", "get", "broken", "--json", "--full"}
				} else if operation == "list" {
					args = []string{"env", "list", "--json", "--full"}
				}
				code, out := resilienceRun(t, options, args...)
				if operation == "doctor" {
					var document struct {
						Checks []struct {
							ID     string `json:"id"`
							Status string `json:"status"`
						} `json:"checks"`
					}
					if err := json.Unmarshal([]byte(out), &document); err != nil {
						t.Fatal(err)
					}
					fileValid, entryFinding := false, false
					for _, check := range document.Checks {
						fileValid = fileValid || (check.ID == "config.valid" && check.Status == "pass")
						entryFinding = entryFinding || (check.ID == "config.environment.broken" && check.Status == "fail")
					}
					if code != 1 || !fileValid || !entryFinding {
						t.Fatalf("doctor treated invalid entry as a file failure: %s", out)
					}
				} else if code != 0 {
					t.Fatalf("inspection failed: %s", out)
				}
				for _, want := range []string{"broken", "invalid", "pat_secret_env"} {
					if !strings.Contains(out, want) {
						t.Fatalf("inspection omitted %q: %s", want, out)
					}
				}
			}
		})
	}
}

func TestConfigResilienceDuplicateURLRepairThroughCLI(t *testing.T) {
	path := resilienceConfig(t, "    url: https://second.example.test\n    url: https://third.example.test\n")
	options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
	before, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	code, out := resilienceRun(t, options, "env", "update", "broken", "--pat-secret-env", "VALID_NAME", "--json")
	if code == 0 || !strings.Contains(out, "url") {
		t.Fatalf("incomplete duplicate repair succeeded or hid remaining URL violation: %s", out)
	}
	after, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(after) != string(before) {
		t.Fatal("incomplete repair changed the invalid entry")
	}
	resilienceRequire(t, options, "env", "update", "broken", "--url", "https://repaired.example.test", "--pat-secret-env", "VALID_NAME", "--json")
	out = resilienceRequire(t, options, "env", "get", "broken", "--json", "--full")
	if !strings.Contains(out, "https://repaired.example.test") || strings.Contains(out, "invalid") || strings.Contains(out, "violations") {
		t.Fatalf("explicit URL repair did not produce a valid environment: %s", out)
	}
	after, err = os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Count(string(after), "https://repaired.example.test") != 1 || strings.Contains(string(after), "https://second.example.test") || strings.Contains(string(after), "https://third.example.test") {
		t.Fatal("explicit URL repair retained duplicate URL values")
	}
	resilienceRequire(t, options, "auth", "status", "--env", "broken", "--json")
}

func TestConfigResilienceStoredPATRemoval(t *testing.T) {
	const reference = coreauth.CredentialReference("cred_77777777777777777777777777777777")
	path := resilienceConfig(t, "      credential_ref: "+string(reference)+"\n")
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}}}
	options := Options{ConfigPath: path, PATStore: store}
	if code, _ := resilienceRun(t, options, "env", "remove", "broken", "--json"); code == 0 {
		t.Fatal("environment removal bypassed stored PAT guard")
	}
	resilienceRequire(t, options, "auth", "logout", "--environment", "broken", "--json")
	if len(store.deleted) != 1 || store.deleted[0] != reference {
		t.Fatal("logout did not remove the invalid environment's stored PAT")
	}
	resilienceRequire(t, options, "env", "remove", "broken", "--json")
}

func TestConfigResilienceDuplicateFieldsRetainUniqueCredentialCleanup(t *testing.T) {
	const reference = coreauth.CredentialReference("cred_99999999999999999999999999999999")
	path := resilienceConfig(t, "      credential_ref: "+string(reference)+"\n")
	resilienceReplace(t, path, "  broken:\n    url: https://tableau.example.test", "  broken:\n    url: https://tableau.example.test\n    url: https://another.example.test")
	store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{reference: {Name: "fixture-name", Secret: "fixture-secret"}}}
	options := Options{ConfigPath: path, PATStore: store}
	code, out := resilienceRun(t, options, "env", "remove", "broken", "--json")
	if code == 0 || !strings.Contains(out, "stored PAT") || len(store.deleted) != 0 {
		t.Fatalf("duplicate unrelated field bypassed known stored PAT guard: %s", out)
	}
	resilienceRequire(t, options, "auth", "logout", "--environment", "broken", "--json")
	if len(store.deleted) != 1 || store.deleted[0] != reference {
		t.Fatal("duplicate unrelated field hid the unique credential from logout")
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), string(reference)) || strings.Count(string(data), "url: https://another.example.test") != 1 {
		t.Fatal("logout did not clear only the known reference while preserving invalid fields")
	}
	resilienceRequire(t, options, "env", "remove", "broken", "--json")
}

func TestConfigResilienceAmbiguousCredentialsAreNeverGuessed(t *testing.T) {
	const first = coreauth.CredentialReference("cred_aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa")
	const second = coreauth.CredentialReference("cred_bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb")
	for _, duplicateAuth := range []bool{false, true} {
		t.Run(fmt.Sprintf("duplicate_auth_%t", duplicateAuth), func(t *testing.T) {
			extra := "      credential_ref: " + string(first) + "\n      credential_ref: " + string(second) + "\n"
			if duplicateAuth {
				extra = "      credential_ref: " + string(first) + "\n    auth:\n      type: pat\n      credential_ref: " + string(second) + "\n"
			}
			path := resilienceConfig(t, extra)
			store := &fakePATStore{records: map[coreauth.CredentialReference]coreauth.PATCredentials{first: {Name: "fixture-first", Secret: "fixture-first-secret"}, second: {Name: "fixture-second", Secret: "fixture-second-secret"}}}
			options := Options{ConfigPath: path, PATStore: store}
			out := resilienceRequire(t, options, "auth", "logout", "--environment", "broken", "--json")
			if !strings.Contains(out, "No stored credential can be located") || len(store.deleted) != 0 || len(store.records) != 2 {
				t.Fatalf("ambiguous credential cleanup guessed a reference: %s", out)
			}
			for _, value := range []string{string(first), string(second), "fixture-first-secret", "fixture-second-secret"} {
				if strings.Contains(out, value) {
					t.Fatal("ambiguous credential cleanup exposed an entry value")
				}
			}
			resilienceRequire(t, options, "env", "remove", "broken", "--json")
		})
	}
}

func TestConfigResilienceMalformedCredentialReferenceCanBeRemoved(t *testing.T) {
	for _, fixture := range []struct{ name, value string }{
		{"scalar", "\"" + resilienceRejectedValue + "\""},
		{"sequence", "[\"" + resilienceRejectedValue + "\"]"},
		{"mapping", "{token: \"" + resilienceRejectedValue + "\"}"},
	} {
		t.Run(fixture.name, func(t *testing.T) {
			path := resilienceConfig(t, "      credential_ref: "+fixture.value+"\n")
			store := &fakePATStore{}
			options := Options{ConfigPath: path, PATStore: store}
			for _, preview := range []bool{true, false} {
				args := []string{"auth", "logout", "--environment", "broken", "--json"}
				if preview {
					args = append(args, "--preview")
				}
				out := resilienceRequire(t, options, args...)
				if !strings.Contains(out, "No stored credential can be located") || len(store.deleted) != 0 {
					t.Fatalf("malformed reference lacked safe local recovery: %s", out)
				}
			}
			resilienceRequire(t, options, "env", "remove", "broken", "--json")
		})
	}
}

func TestConfigResilienceWritesPreserveUntouchedInvalidEntry(t *testing.T) {
	for _, save := range []bool{false, true} {
		t.Run(fmt.Sprintf("save_%t", save), func(t *testing.T) {
			path := resilienceConfig(t, "    future_field:\n      nested: [one, two]\n")
			before := resilienceRawEnvironment(t, path, "broken")
			if save {
				cfg, err := config.Load(path)
				if err != nil {
					t.Fatalf("load failed before unrelated Save: %v", err)
				}
				cfg.Environments["another"] = config.Environment{URL: "https://another.example.test", Auth: config.Auth{Type: "pat"}}
				if err := config.Save(path, cfg); err != nil {
					t.Fatalf("unrelated Save failed: %v", err)
				}
			} else {
				resilienceRequire(t, Options{ConfigPath: path, PATStore: &fakePATStore{}}, "env", "add", "another", "--url", "https://another.example.test", "--json")
			}
			if !reflect.DeepEqual(before, resilienceRawEnvironment(t, path, "broken")) {
				t.Fatal("unrelated write changed raw invalid entry")
			}
		})
	}
}

func resilienceRawEnvironment(t *testing.T, path, alias string) any {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	return raw["environments"].(map[string]any)[alias]
}

func TestConfigResilienceUnknownFieldAndSharedCredentials(t *testing.T) {
	for _, shared := range []bool{false, true} {
		t.Run(fmt.Sprintf("shared_%t", shared), func(t *testing.T) {
			path := resilienceConfig(t, "")
			resilienceReplace(t, path, "\""+resilienceRejectedValue+"\"", "BROKEN_PAT_SECRET")
			data, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if shared {
				data = append(data, []byte("      credential_ref: cred_88888888888888888888888888888888\n  peer:\n    url: https://peer.example.test\n    auth:\n      type: pat\n      credential_ref: cred_88888888888888888888888888888888\n")...)
			} else {
				data = append(data, []byte("    future_field: fixture\n")...)
			}
			if err := os.WriteFile(path, data, 0o600); err != nil {
				t.Fatal(err)
			}
			options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
			resilienceRequire(t, options, "auth", "status", "--env", "good", "--json")
			for _, alias := range []string{"broken", "peer"} {
				if alias == "peer" && !shared {
					continue
				}
				code, out := resilienceRun(t, options, "auth", "status", "--env", alias, "--json")
				if code == 0 || !strings.Contains(out, "broken") || (shared && (!strings.Contains(out, "peer") || !strings.Contains(out, "credential_ref"))) {
					t.Fatalf("invalid cross-entry or unknown field was usable: %s", out)
				}
			}
		})
	}
}

func TestConfigResilienceFileFailuresRemainFatal(t *testing.T) {
	for _, text := range []string{"version: [\n", "version: 2\n", "version: 1\nsite_mutations: invalid\n", "version: 1\n---\nversion: 1\n"} {
		t.Run(fmt.Sprintf("fixture_%x", len(text)), func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "config.yaml")
			if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
				t.Fatal(err)
			}
			if code, _ := resilienceRun(t, Options{ConfigPath: path, PATStore: &fakePATStore{}}, "env", "list", "--json"); code == 0 {
				t.Fatal("file-level failure was accepted")
			}
		})
	}
}

func TestConfigResilienceLegacyAPIVersionIsIgnoredAndRemoved(t *testing.T) {
	path := resilienceConfig(t, "")
	resilienceReplace(t, path, "    url: https://tableau.example.test", "    api_version: obsolete-value\n    url: https://tableau.example.test")
	network := &warningTestNoNetwork{}
	options := Options{ConfigPath: path, PATStore: &fakePATStore{}, HTTPClient: &http.Client{Transport: network}}
	out := resilienceRequire(t, options, "auth", "status", "--env", "good", "--json", "--full")
	if network.calls != 0 || strings.Contains(out, "api_version") {
		t.Fatal("local auth status contacted Tableau or exposed the removed setting")
	}
	resilienceRequire(t, options, "env", "remove", "broken", "--json")
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(data), "api_version") {
		t.Fatal("write retained legacy API version")
	}
	if code, _ := resilienceRun(t, options, "env", "add", "another", "--url", "https://another.example.test", "--api-version", "3.29", "--json"); code == 0 {
		t.Fatal("removed API version flag was accepted")
	}
}

func TestConfigResilienceWorkspaceRepairThroughCLI(t *testing.T) {
	path := resilienceConfig(t, "")
	options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
	resilienceRequire(t, options, "workspace", "create", "good-work", "--path", filepath.Join(t.TempDir(), "workspace"))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var raw map[string]any
	if err := yaml.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	raw["workspaces"].(map[string]any)["broken-work"] = map[string]any{"id": "invalid-identity", "path": "relative-root"}
	data, err = yaml.Marshal(raw)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
	resilienceReplace(t, path, "default_environment: good", "default_environment: broken")
	resilienceRequire(t, options, "workspace", "status", "--workspace", "good-work", "--json")
	resilienceReplace(t, path, "default_environment: broken", "default_environment: good")
	code, out := resilienceRun(t, options, "workspace", "status", "--workspace", "broken-work", "--json")
	if code == 0 || !strings.Contains(out, "workspace unregister broken-work") || !strings.Contains(out, "not_attempted") {
		t.Fatalf("invalid workspace lost typed recovery: %s", out)
	}
	var failure struct {
		Error struct {
			CorrectiveAction string `json:"corrective_action"`
		} `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &failure); err != nil || strings.Count(failure.Error.CorrectiveAction, "--config") != 2 || strings.Count(failure.Error.CorrectiveAction, path) != 2 {
		t.Fatalf("workspace repair hints lost selected configuration: %s err=%v", out, err)
	}
	out = resilienceRequire(t, options, "workspace", "list", "--full", "--json")
	for _, unwanted := range []string{"invalid-identity", "relative-root"} {
		if strings.Contains(out, unwanted) {
			t.Fatal("workspace listing exposed invalid registration values")
		}
	}
	if !strings.Contains(out, "broken-work") || !strings.Contains(out, "invalid") {
		t.Fatalf("workspace listing hid invalid registration: %s", out)
	}
	resilienceReplace(t, path, "default_workspace: good-work", "default_workspace: broken-work")
	resilienceRequire(t, options, "workspace", "set-default", "good-work", "--json")
	resilienceRequire(t, options, "workspace", "unregister", "broken-work", "--preview", "--json")
	resilienceRequire(t, options, "workspace", "unregister", "broken-work", "--json")
	resilienceRequire(t, options, "workspace", "status", "--workspace", "good-work", "--json")
}

func TestConfigResilienceRemoveInvalidDefaultEnvironment(t *testing.T) {
	path := resilienceConfig(t, "")
	resilienceReplace(t, path, "default_environment: good", "default_environment: broken")
	options := Options{ConfigPath: path, PATStore: &fakePATStore{}}
	resilienceRequire(t, options, "env", "remove", "broken", "--json")
	cfg, err := config.Load(path)
	if err != nil || cfg.DefaultEnvironment != "" {
		t.Fatalf("removal stranded invalid default environment: default=%q err=%v", cfg.DefaultEnvironment, err)
	}
	resilienceRequire(t, options, "auth", "status", "--env", "good", "--json")
}

func TestConfigResilienceHealthyEnvironmentThroughCLI(t *testing.T) {
	path := resilienceConfig(t, "")
	var out strings.Builder
	code := Run(t.Context(), []string{"auth", "status", "--env", "good", "--json"}, &out, Options{ConfigPath: path, PATStore: &fakePATStore{}})
	if strings.Contains(out.String(), resilienceRejectedValue) {
		t.Fatal("CLI exposed a rejected PAT reference")
	}
	if code != 0 {
		t.Fatalf("healthy environment failed with exit %d: %s", code, out.String())
	}
	for _, want := range []string{"broken", "pat_secret_env", "revoke", "warnings"} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("healthy command omitted warning %q: %s", want, out.String())
		}
	}
	var document struct {
		Metadata struct {
			Warnings []string `json:"warnings"`
		} `json:"metadata"`
	}
	if err := json.Unmarshal([]byte(out.String()), &document); err != nil || len(document.Metadata.Warnings) != 1 {
		t.Fatalf("healthy command must carry one invalid-entry warning: %s err=%v", out.String(), err)
	}
	if warning := document.Metadata.Warnings[0]; strings.Count(warning, "--config") != 3 || strings.Count(warning, path) != 3 {
		t.Fatalf("warning repair hints lost selected configuration: %s", warning)
	}
}
