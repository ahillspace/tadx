package app

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestTDSEarlyFileDiagnosticsDoNotEchoLocalValues(t *testing.T) {
	h := newTDSWorkflowCLI(t)
	// Use only isolated options with a fixture-scoped consent and a forbidden
	// HTTP transport and credential store. Every case must finish offline.
	options := withSiteMutationConsent(t, h.options, true)
	const marker = "synthetic-local-marker"
	if err := os.WriteFile(filepath.Join(h.ws, marker), []byte("existing file"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(h.ws, "invalid.yaml"), []byte("target:\n  "+marker+": "+marker+"\nchanges: []\n"), 0600); err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"audit collision", []string{"checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", marker}, []string{"--audit-file", "already exists"}},
		{"changes collision", []string{"work-copy", "--work-copy", fixtureTDSCopy, "--workspace", "work", "--changes-file", marker}, []string{"--changes-file", "already exists"}},
		{"identical outputs", []string{"checkout", "--id", fixtureTDSOriginal, "--workspace", "work", "--audit-file", marker, "--changes-file", marker}, []string{"--changes-file", "--audit-file", "distinct"}},
		{"missing input", []string{"validate-changes", "--workspace", "work", "--from-changes", marker + ".missing"}, []string{"--from-changes", "exist"}},
		{"input parent", []string{"apply-changes", "--preview", "--workspace", "work", "--from-changes", marker + "-parent/plan.yaml"}, []string{"--from-changes", "directory"}},
		{"output parent", []string{"work-copy", "--work-copy", fixtureTDSCopy, "--workspace", "work", "--audit-file", marker + "-parent/out.md"}, []string{"--audit-file", "directory"}},
		{"workspace selector", []string{"validate-changes", "--workspace", marker, "--from-changes", "file.yaml"}, []string{"--workspace", "workspace list"}},
		{"invalid input path", []string{"save", "overwrite", "--preview", "--workspace", "work", "--from-changes", "../" + marker}, []string{"--from-changes", "workspace-relative"}},
		{"invalid YAML", []string{"validate-changes", "--workspace", "work", "--from-changes", "invalid.yaml"}, []string{"--from-changes", "unknown or duplicate key"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out, stderr bytes.Buffer
			opts := options
			opts.Stderr = &stderr
			args := append([]string{"content", "datasource"}, tc.args...)
			args = append(args, "--full", "--json")
			var exit int
			if tc.args[0] == "validate-changes" {
				exit = Run(t.Context(), args, &out, opts)
			} else {
				exit = run(t.Context(), args, &out, opts, qualifiedTDSDefinitions())
			}
			if exit == 0 || !json.Valid(out.Bytes()) {
				t.Fatalf("unexpected result %d %s", exit, &out)
			}
			var last bytes.Buffer
			if code := Run(t.Context(), []string{"last", "--full", "--json"}, &last, opts); code != 0 {
				t.Fatalf("reading last result: %d %s", code, &last)
			}
			for _, result := range []string{out.String(), last.String(), stderr.String()} {
				if strings.Contains(result, marker) || strings.Contains(result, h.ws) {
					t.Errorf("diagnostic included a user-controlled path or document value")
				}
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) || !strings.Contains(last.String(), want) {
					t.Errorf("command or saved diagnostic missing %q: %s", want, &out)
				}
			}
		})
	}
}
