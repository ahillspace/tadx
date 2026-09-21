package app

import (
	"strings"
	"testing"
)

func TestContentInspectHelpUsesAvailableProjectSelectors(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	for _, resource := range []string{"workbook", "datasource", "flow"} {
		t.Run(resource, func(t *testing.T) {
			for _, path := range [][]string{{"content", resource}, {"content", resource, "inspect"}} {
				output := contentHelpPilotRun(t, dir, options, append(path, "--help")...)
				if !strings.Contains(output, "--project or --project-id") {
					t.Errorf("%v does not document both project selectors:\n%s", path, output)
				}
				if len(path) == 3 && strings.Contains(output, "--name requires --project;") {
					t.Errorf("%v incorrectly requires the project path", path)
				}
			}
		})
	}
}

func TestContentResourceHelpScopesWorkspaceToLocalFileActions(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	for _, resource := range []string{"workbook", "datasource", "flow"} {
		t.Run(resource, func(t *testing.T) {
			complete := contentHelpPilotRun(t, dir, options, "content", resource, "--help")
			if !strings.Contains(complete, "--workspace (publish,pull)") {
				t.Errorf("workspace scope is not explicit:\n%s", complete)
			}
			for _, verb := range []string{"list", "inspect", "move", "update", "delete"} {
				focused := contentHelpPilotRun(t, dir, options, "content", resource, verb, "--help")
				if strings.Contains(focused, "--workspace") {
					t.Errorf("%s incorrectly advertises workspace scope", verb)
				}
			}
		})
	}
}

func TestCompleteInventoryRejectsPageLimitAcrossFamilies(t *testing.T) {
	_, options := contentHelpPilotSetup(t)
	for _, path := range []string{
		"content workbook list", "content datasource list", "content flow list", "content project list",
		"admin user list", "admin group list", "pulse definition list", "pulse metric list", "capability list",
		"catalog database list", "catalog table list", "catalog column list", "catalog search", "content datasource schema",
		"admin label-value list", "admin label-category list", "catalog label list",
		"env list", "workspace list", "workspace status",
	} {
		t.Run(path, func(t *testing.T) {
			args := append(strings.Fields(path), "--all", "--limit", "1", "--json")
			switch path {
			case "pulse metric list":
				args = append(args, "--definition-id", "definition-id")
			case "catalog column list":
				args = append(args, "--table-id", "table-id")
			case "catalog search":
				args = append(args, "Revenue")
			case "catalog label list":
				args = append(args, "--type", "datasource", "--target-id", "datasource-id")
			case "content datasource schema":
				args = append(args, "--id", "datasource-id")
			}
			var stdout strings.Builder
			code := Run(t.Context(), args, &stdout, options)
			output := stdout.String()
			if code != 2 || !strings.Contains(output, "all") || !strings.Contains(output, "limit") || !strings.Contains(output, `"outcome":"not_attempted"`) {
				t.Fatalf("conflicting completeness request did not fail before config/remote work: code=%d output=%s", code, output)
			}
		})
	}
}

func TestContentFocusedExamplesAreIncludedInCompleteReference(t *testing.T) {
	dir, options := contentHelpPilotSetup(t)
	for _, resource := range []string{"workbook", "datasource", "flow", "project"} {
		t.Run(resource, func(t *testing.T) {
			complete := contentHelpPilotRun(t, dir, options, "content", resource, "--help")
			for _, verb := range []string{"list", "inspect", "move", "update", "delete", "publish", "pull", "schema", "create"} {
				if !strings.Contains(complete, "\n  "+verb+":") {
					continue
				}
				focused := contentHelpPilotRun(t, dir, options, "content", resource, verb, "--help")
				_, examples, _ := strings.Cut(focused, "Examples:\n")
				for line := range strings.SplitSeq(examples, "\n") {
					if example := strings.TrimSpace(line); example != "" && !strings.Contains(complete, example) {
						t.Errorf("%s example is absent from complete reference: %s", verb, example)
					}
				}
			}
		})
	}
}
