package app

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestHelpAuditRequiredFacts(t *testing.T) {
	for _, test := range []struct {
		path string
		want []string
	}{
		{"search", []string{"Term or --type required", "type omitted: all types", "bounded inventory"}},
		{"auth", []string{"login:", "logout:", "--environment <environment-name>", "local readiness", "live PAT"}},
		{"pulse definition", []string{"mutually exclusive: --all, --limit", "--all: <=10000"}},
		{"pulse metric", []string{"mutually exclusive: --all, --limit", "--all: <=10000"}},
		{"catalog audit", []string{"default: descriptions+tags", "--direct-only: field-owned descriptions", "limit: assessed assets"}},
		{"catalog search", []string{"default types: database+table", "--all: <=10000", "incomplete"}},
		{"update", []string{"--check: no installation changes", "target omitted: auto"}},
		{"capability", []string{"Filters: AND", "product: substring", "--mutation=false", "omitted: both"}},
		{"workspace", []string{"registered workspace", "Use tadx workspace <resource> -h"}},
		{"workspace clone", []string{"destination root must not exist", "--name", "--path"}},
		{"workspace artifact", []string{"--force: delete dirty artifacts", "destination artifact must not exist", "workbook|datasource|flow|pulse-definition|lineage"}},
	} {
		t.Run(test.path, func(t *testing.T) {
			var out strings.Builder
			args := append(strings.Fields(test.path), "-h")
			if code := Run(t.Context(), args, &out, Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}); code != 0 {
				t.Fatalf("help exit %d: %s", code, out.String())
			}
			for _, want := range test.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("missing %q in %s help:\n%s", want, test.path, out.String())
				}
			}
		})
	}
}

func TestHelpAuditUtilityReferences(t *testing.T) {
	for _, command := range []string{"help", "completion"} {
		t.Run(command, func(t *testing.T) {
			var out strings.Builder
			if code := Run(t.Context(), []string{command, "-h"}, &out, Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}); code != 0 {
				t.Fatalf("help exit %d: %s", code, out.String())
			}
			if command == "help" {
				if !strings.Contains(out.String(), "help [command ...]") || !strings.Contains(out.String(), "Omitted: root help") || strings.Contains(out.String(), "Commands:") {
					t.Fatalf("help utility reference is malformed:\n%s", out.String())
				}
			} else if strings.Contains(out.String(), "--json") || strings.Contains(out.String(), "--full") || !strings.Contains(out.String(), "shell script to stdout") {
				t.Fatalf("completion advertises ineffective output controls:\n%s", out.String())
			}
		})
	}
}
