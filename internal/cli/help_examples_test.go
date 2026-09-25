package cli_test

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

type helpExampleTransport struct{ t *testing.T }

func (transport helpExampleTransport) RoundTrip(*http.Request) (*http.Response, error) {
	transport.t.Error("help attempted a network request")
	return nil, fmt.Errorf("network disabled for help examples")
}

// Run the real composition root with --help so Cobra resolves every command and
// parses every example flag without executing actions or accessing Tableau.
func TestCategoryHelpExamplesResolve(t *testing.T) {
	directory := t.TempDir()
	options := app.Options{
		ConfigPath:  filepath.Join(directory, "config.json"),
		UserHomeDir: func() (string, error) { return directory, nil },
		HTTPClient:  &http.Client{Transport: helpExampleTransport{t}},
		Stderr:      io.Discard,
	}
	categories := []string{
		"auth", "env", "cache", "catalog", "catalog database", "catalog table", "catalog column", "catalog lineage", "catalog label",
		"content", "content workbook", "content datasource", "content datasource schema", "content flow", "content project",
		"admin", "admin user", "admin group", "admin group-member", "admin permission", "admin label-value", "admin label-category",
		"pulse", "pulse definition", "pulse metric", "pulse metric followers", "workspace", "workspace artifact", "agent", "mutation", "capability",
	}
	seen := map[string]bool{}
	for _, category := range categories {
		t.Run(category, func(t *testing.T) {
			var output bytes.Buffer
			args := append(strings.Fields(category), "--help")
			if code := app.Run(context.Background(), args, &output, options); code != 0 {
				t.Fatalf("category help failed (%d): %s", code, output.String())
			}
			count := 0
			inExamples := false
			for _, line := range strings.Split(output.String(), "\n") {
				trimmed := strings.TrimSpace(line)
				if strings.EqualFold(trimmed, "Examples:") {
					inExamples = true
					continue
				}
				if !inExamples || !strings.HasPrefix(trimmed, "tadx ") {
					continue
				}
				count++
				if seen[trimmed] {
					continue
				}
				seen[trimmed] = true
				var exampleOutput bytes.Buffer
				exampleArgs := append(strings.Fields(trimmed)[1:], "--help")
				if code := app.Run(context.Background(), exampleArgs, &exampleOutput, options); code != 0 {
					t.Errorf("example %q failed (%d): %s", trimmed, code, exampleOutput.String())
				}
			}
			if count == 0 && (strings.Contains(category, "schema") || strings.Contains(category, "permission") || category == "pulse definition" || category == "pulse metric") {
				t.Errorf("complex command help has no examples:\n%s", output.String())
			}
		})
	}
	// Compact references may deduplicate or combine examples. Preserve coverage
	// of the operations that need examples instead of prescribing a total count.
	for _, path := range []string{
		"content workbook publish", "content datasource publish",
		"content flow publish", "content project move",
		"admin user create", "admin permission create",
		"pulse definition publish", "pulse metric fork",
		"workspace artifact delete",
	} {
		covered := false
		for example := range seen {
			if strings.HasPrefix(example, "tadx "+path+" ") {
				covered = true
				break
			}
		}
		if !covered {
			t.Errorf("no validated example covers %s", path)
		}
	}
}

func TestCategoryHelpExplainsRequiredAlternatives(t *testing.T) {
	directory := t.TempDir()
	options := app.Options{ConfigPath: filepath.Join(directory, "config.json"), UserHomeDir: func() (string, error) { return directory, nil }, Stderr: io.Discard}
	cases := []struct {
		path string
		want []string
	}{
		{"admin group", []string{"create:", "--name <group-name>", "exactly one of: --id, --name"}},
		{"admin group-member", []string{"--group-id", "exactly one of: --user-id, --username"}},
		{"admin permission", []string{"--principal-type", "--principal-username", "--capability", "--mode"}},
		{"content workbook publish", []string{"Usage: tadx content workbook publish", "(--artifact <path>... | --file <file.twb|file.twbx> | --id <luid> | --artifact-name <name>)", "(--project-id <luid> | --project <path>)", "Batch:", `{"items":[{"<flag>":"<value>"}]}`, "required per row", "1-100"}},
		{"pulse definition", []string{"--name", "--datasource-id", "--measure-field", "--date-field", "--dimension", "--datasource-map"}},
		{"pulse metric", []string{"at least one of: --period, --filter, --exclude-filter", "CUSTOM_N_DAYS", "exactly one of: --user-id, --group-id"}},
		{"workspace artifact", []string{"--artifact", "both --kind and --id"}},
		{"pulse", []string{"does not retrieve current metric values or generated insights"}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var output bytes.Buffer
			if code := app.Run(context.Background(), append(strings.Fields(tc.path), "--help"), &output, options); code != 0 {
				t.Fatalf("help failed (%d): %s", code, output.String())
			}
			for _, want := range tc.want {
				if !strings.Contains(output.String(), want) {
					t.Errorf("help missing %q:\n%s", want, output.String())
				}
			}
		})
	}
}

func TestFocusedHelpCleanupKeepsOperationalGuidance(t *testing.T) {
	directory := t.TempDir()
	options := app.Options{ConfigPath: filepath.Join(directory, "config.json"), UserHomeDir: func() (string, error) { return directory, nil }, Stderr: io.Discard}
	cases := []struct {
		path  string
		want  []string
		avoid []string
	}{
		{path: "", want: []string{"<category> <resource> <verb> --help"}, avoid: []string{"<category> <verb> --help"}},
		{path: "policy", want: []string{"install:", "samples:", "status:", "validate <file>:", "Default install uses superuser", "replaces the policy", "saved consent", "available for recovery", "All templates allow reads", "read-only blocks Tableau mutations", "read-write-no-admin blocks administrative mutations", "superuser allows supported operations", "tadx policy install -h", "tadx policy samples -h"}, avoid: []string{"Default paths:", "sudo", "rename it managed-policy.json", "Booleans:", "Examples:"}},
		{path: "policy status", want: []string{"resolved policy path", "effective restrictions", "protection checks", "Available when managed policy blocks"}, avoid: []string{"--output creates inactive", "Booleans:"}},
		{path: "policy install", want: []string{"managed-policy.json", "/Library/Application Support/TADX", "Windows requests UAC approval", "sudo from an interactive terminal", "password only in that terminal", "sudo \"$(command -v tadx)\" policy install --template read-only", "sudo might not have tadx in its PATH", "tadx policy samples -h", "root-owned", "saved consent", "User configuration cannot override", "tadx policy install --template read-write-no-admin", "tadx policy install --template superuser"}, avoid: []string{"Booleans:"}},
		{path: "policy samples", want: []string{"read-only.json", "read-write-no-admin.json", "superuser.json", "rename it managed-policy.json", "protected permissions", "tadx policy status", "Default paths:"}, avoid: []string{"policy status --full", "Booleans:"}},
		{path: "admin group list", avoid: []string{"Booleans:"}},
		{path: "mutation set", want: []string{"--enabled=true|false"}, avoid: []string{"Booleans:"}},
		{path: "admin group create", want: []string{"--external-user-enabled=true|false"}},
		{path: "content workbook publish", want: []string{"Save accepted IDs", "returned status command", "Do not republish"}, avoid: []string{"Publish polls:"}},
		{path: "workspace register", avoid: []string{"artifact file operations", "workspace status -h"}},
		{path: "workspace clean", want: []string{"workspace status -h"}, avoid: []string{"artifact file operations"}},
		{path: "pulse metric follow", avoid: []string{"current metric values or generated insights"}},
		{path: "pulse metric inspect", want: []string{"current metric values or generated insights"}},
		{path: "job wait", want: []string{"twenty minutes", "never resubmits", "Accepted work continues", "status command"}},
		{path: "completion", want: []string{"Out-String | Invoke-Expression", "shell profile"}, avoid: []string{"Capture and verify", "bash -n", "scriptblock"}},
		{path: "last", want: []string{"never repeats its command"}, avoid: []string{"Never re-execute it"}},
		{path: "admin group list", avoid: []string{"Site roles (site/version dependent)"}},
		{path: "admin group inspect", avoid: []string{"Site roles (site/version dependent)"}},
		{path: "admin group create", want: []string{"Site roles (site/version dependent)"}},
		{path: "admin group update", want: []string{"Site roles (site/version dependent)"}},
		{path: "admin user list", avoid: []string{"Site roles (site/version dependent)"}},
		{path: "admin user inspect", avoid: []string{"Site roles (site/version dependent)"}},
		{path: "admin user create", want: []string{"Site roles (site/version dependent)"}},
		{path: "admin user update", want: []string{"Site roles (site/version dependent)"}},
		{path: "auth", want: []string{"configured PAT source", "env add/update", "Never put PAT values in config"}},
		{path: "auth check", want: []string{"configured PAT source", "env add/update", "Never put PAT values in config"}},
		{path: "auth login", want: []string{"Configured environment credentials take precedence", "env add/update", "Never put PAT values in config"}},
		{path: "auth status", want: []string{"local readiness", "env add/update", "Never put PAT values in config"}},
	}
	for _, tc := range cases {
		t.Run(tc.path, func(t *testing.T) {
			var output bytes.Buffer
			args := append(strings.Fields(tc.path), "--help")
			if code := app.Run(t.Context(), args, &output, options); code != 0 {
				t.Fatalf("help failed (%d): %s", code, output.String())
			}
			got := output.String()
			for _, want := range tc.want {
				if !strings.Contains(got, want) {
					t.Errorf("help missing %q:\n%s", want, got)
				}
			}
			for _, avoid := range tc.avoid {
				if strings.Contains(got, avoid) {
					t.Errorf("help contains unrelated %q:\n%s", avoid, got)
				}
			}
			if tc.path == "auth" || tc.path == "auth check" {
				if count := strings.Count(got, "env add/update"); count != 1 {
					t.Errorf("PAT setup is repeated %d times:\n%s", count, got)
				}
			}
		})
	}
}
