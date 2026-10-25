// The host fixture implements only synthetic public CLI output for extension tests.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

type document = map[string]any

func main() {
	root := os.Getenv("TADX_HOST_FIXTURE_ROOT")
	if root == "" {
		fail("Host fixture root is missing.")
	}
	args := os.Args[1:]
	file, err := os.CreateTemp(filepath.Join(root, "calls"), "call-*.json")
	if err != nil {
		fail("Host fixture could not record invocation.")
	}
	if err := json.NewEncoder(file).Encode(args); err != nil {
		fail("Host fixture could not encode invocation.")
	}
	if err := file.Close(); err != nil {
		fail("Host fixture could not finish invocation record.")
	}
	if !slices.Contains(args, "--json") || !slices.Contains(args, "--full") {
		fail("Host fixture requires full JSON output.")
	}
	var scenario struct {
		MissingCapability     string `json:"missingCapability"`
		MalformedCapabilities bool   `json:"malformedCapabilities"`
		FailScope             string `json:"failScope"`
		IncompletePermissions bool   `json:"incompletePermissions"`
		WarningScope          string `json:"warningScope"`
		NoDefaultSites        bool   `json:"noDefaultSites"`
	}
	data, err := os.ReadFile(filepath.Join(root, "scenario.json"))
	if err != nil || json.Unmarshal(data, &scenario) != nil {
		fail("Host fixture scenario is invalid.")
	}
	workspaceRoot := filepath.Join(root, "workspace")
	workspace := document{"name": "work", "id": "ws-fixture", "root": workspaceRoot, "default": true, "available": true, "manifest_valid": true}
	page := document{"returned": 1, "total": 1, "limit": 10000, "more_available": false}
	source := document{"mode": "cache", "coverage": "complete", "stale": false, "generation_id": "generation-fixture", "observed_at": "2026-01-01T00:00:00Z"}
	switch {
	case starts(args, "version"):
		emit(document{"status": "ready", "version": "0.1.0-host-fixture"})
	case starts(args, "env", "list"):
		environments := []document{
			{"alias": "sandbox", "default": true, "server_url": "https://tableau.example.test", "site_content_url": "sandbox", "default_workspace": "work"},
			{"alias": "staging", "default": false, "server_url": "https://tableau.example.test", "site_content_url": "staging", "default_workspace": "work"},
		}
		if scenario.NoDefaultSites {
			environments = []document{
				{"alias": "alpha", "default": false, "server_url": "https://tableau.example.test", "site_content_url": "alpha"},
				{"alias": "beta", "default": false, "server_url": "https://tableau.example.test", "site_content_url": "beta"},
			}
		}
		emit(document{"status": "listed", "page": document{"returned": 2, "total": 2, "limit": 10000, "more_available": false}, "environments": environments})
	case starts(args, "workspace", "list"):
		emit(document{"status": "listed", "page": page, "workspaces": []document{workspace}})
	case starts(args, "workspace", "status"):
		emit(document{"status": "ready", "workspace": workspace, "help": []string{}, "artifacts": document{
			"returned": 3, "total": 3, "limit": 10000, "more_available": false, "scan_complete": true,
			"clean": 3, "dirty": 0, "missing": 0, "invalid": 0,
			"artifacts": []document{{"kind": "workbook", "luid": "workbook-1", "name": "Finance local", "state": "clean",
				"path": "artifacts/workbook/Finance--fixture", "canonical_path": "artifacts/workbook/Finance--fixture/Finance.twb",
				"source_server_origin": "https://tableau.example.test", "source_site_luid": "site-fixture", "source_environment": "sandbox", "source_site": "sandbox"},
				{"kind": "pulse-definition", "luid": "definition-1", "name": "Revenue definition", "state": "clean",
					"path": "artifacts/pulse-definition/Revenue--fixture", "canonical_path": "artifacts/pulse-definition/Revenue--fixture/definition.json",
					"source_server_origin": "https://tableau.example.test", "source_site_luid": "site-fixture", "source_environment": "sandbox", "source_site": "sandbox"},
				{"kind": "lineage", "luid": "workbook-1", "name": "Finance lineage", "state": "clean",
					"path": "artifacts/lineage/workbook/Finance--fixture", "canonical_path": "artifacts/lineage/workbook/Finance--fixture/lineage.json",
					"source_server_origin": "https://tableau.example.test", "source_site_luid": "site-fixture", "source_environment": "sandbox", "source_site": "sandbox"}},
		}})
	case starts(args, "cache", "status"):
		result := document{"status": "current", "generation": document{"id": "generation-fixture", "environment": flag(args, "--environment"), "site": flag(args, "--environment"),
			"generated_at": "2026-01-01T00:00:00Z", "complete": true, "stale": false,
			"coverage": []document{{"scope": "projects", "requested": true, "complete": true, "records": 2},
				{"scope": "workbooks", "requested": true, "complete": true, "records": 1},
				{"scope": "datasources", "requested": true, "complete": true, "records": 1},
				{"scope": "flows", "requested": true, "complete": true, "records": 1}}}}
		if scenario.IncompletePermissions {
			result["status"] = "partial"
			result["warnings"] = []string{"Cache permission coverage is incomplete because some workbook permission reads were denied (HTTP 403). Missing rules are unknown, not empty permissions."}
			generation := result["generation"].(document)
			generation["complete"] = false
			generation["coverage"] = append(generation["coverage"].([]document), document{"scope": "permissions", "requested": true, "complete": false, "records": 0})
		}
		emit(result)
	case len(args) >= 3 && args[0] == "content" && args[2] == "list":
		if !slices.Contains(args, "--cache") || !slices.Contains(args, "--all") || flag(args, "--environment") == "" {
			fail("Host fixture rejects live or unscoped content discovery.")
		}
		kind := args[1]
		if scenario.FailScope == kind {
			fail("Synthetic " + kind + " scope unavailable.")
		}
		var items []document
		if kind == "project" {
			items = []document{{"luid": "project-root", "name": "Analytics", "parent_luid": ""}, {"luid": "project-1", "name": "Daily", "parent_luid": "project-root"}}
		} else {
			names := map[string]string{"workbook": "Finance", "datasource": "Sales", "flow": "Preparation"}
			name, ok := names[kind]
			if !ok {
				fail("Host fixture does not support this content kind.")
			}
			items = []document{{"luid": kind + "-1", "name": name, "project_luid": "project-1", "project_path": "Analytics/Daily", "content_url": "https://tableau.example.test/#/site/sandbox/workbooks/1"}}
		}
		page["returned"], page["total"] = len(items), len(items)
		result := document{"status": "listed", "environment": flag(args, "--environment"), "site": flag(args, "--environment"), "page": page, "source": source, kind + "s": items}
		if scenario.WarningScope == kind {
			result["warnings"] = []string{"Synthetic " + kind + " inventory diagnostic."}
		}
		emit(result)
	case starts(args, "capability", "list"):
		var capabilities []document
		for _, id := range []string{"env.profile.list", "workspace.list", "workspace.status", "cache.status", "cache.refresh", "project.list", "workbook.list", "datasource.list", "flow.list", "mutation.status", "job.inspect", "workspace.move", "workspace.artifact.delete"} {
			capabilities = append(capabilities, document{"id": id, "implementation_state": "implemented", "execution_enabled": true, "supports_preview": true, "supports_batch": true})
		}
		for _, kind := range []string{"workbook", "datasource", "flow"} {
			for _, verb := range []string{"pull", "publish"} {
				capabilities = append(capabilities, document{"id": kind + "." + verb, "implementation_state": "implemented", "execution_enabled": true, "supports_preview": true, "supports_batch": true})
			}
		}
		capabilities = slices.DeleteFunc(capabilities, func(item document) bool { return item["id"] == scenario.MissingCapability })
		if scenario.MalformedCapabilities {
			emit(document{"capabilities": capabilities})
			return
		}
		emit(document{"page": document{"returned": len(capabilities), "total": len(capabilities), "limit": 10000, "more_available": false}, "capabilities": capabilities})
	case starts(args, "mutation", "status"):
		emit(document{"status": "enabled", "enabled": true, "environment": flag(args, "--environment"), "site": flag(args, "--environment")})
	case len(args) >= 3 && args[0] == "content" && slices.Contains([]string{"pull", "publish"}, args[2]) && slices.Contains(args, "--preview"):
		emit(document{"status": "preview", "operation": args[1] + "." + args[2], "environment": flag(args, "--environment"), "workspace": "work", "luid": flag(args, "--id")})
	default:
		fail("Host fixture rejects unsupported commands and non-preview mutations: " + strings.Join(args, " "))
	}
}

func starts(args []string, prefix ...string) bool {
	return len(args) >= len(prefix) && slices.Equal(args[:len(prefix)], prefix)
}

func flag(args []string, name string) string {
	for index, arg := range args {
		if arg == name && index+1 < len(args) {
			return args[index+1]
		}
	}
	return ""
}

func emit(value document) {
	if err := json.NewEncoder(os.Stdout).Encode(value); err != nil {
		fmt.Fprintln(os.Stderr, "Host fixture could not encode output.")
		os.Exit(1)
	}
}

func fail(summary string) {
	emit(document{"error": document{"id": "fixture.failed", "kind": "operation", "summary": summary, "retryable": false}})
	os.Exit(1)
}
