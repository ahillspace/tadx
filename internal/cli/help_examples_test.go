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
		"auth", "env", "cache", "catalog", "catalog database", "catalog table", "catalog column",
		"content", "content workbook", "content datasource", "content datasource schema", "content flow", "content project", "content lineage", "content label",
		"admin", "admin user", "admin group", "admin group member", "admin permission", "admin label", "admin label value", "admin label category",
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
			if count == 0 {
				t.Errorf("category has no examples:\n%s", output.String())
			}
		})
	}
	if len(seen) < 50 {
		t.Errorf("validated only %d distinct examples", len(seen))
	}
}

func TestCategoryHelpExplainsRequiredAlternatives(t *testing.T) {
	directory := t.TempDir()
	options := app.Options{ConfigPath: filepath.Join(directory, "config.json"), UserHomeDir: func() (string, error) { return directory, nil }, Stderr: io.Discard}
	cases := []struct {
		path string
		want []string
	}{
		{"admin group", []string{"Create requires --name", "--id or --name"}},
		{"admin group member", []string{"--group-id", "exactly one of --user-id or --username"}},
		{"admin permission", []string{"--principal-type", "--principal-username", "--capability", "--mode"}},
		{"content workbook publish", []string{"--artifact, --file, --id, or --artifact-name", "--project-id or --project", `Batch JSON: {"items":`, "canonical flag names", "1-100"}},
		{"pulse definition", []string{"--name, --datasource-id, --measure-field, and --date-field", "--datasource-map"}},
		{"pulse metric", []string{"--period, --filter, or --exclude-filter", "CUSTOM_N_DAYS", "exactly one of --user-id or --group-id"}},
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
