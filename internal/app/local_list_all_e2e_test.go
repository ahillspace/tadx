package app_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestLocalListAllCommandsReturnCompleteBoundedPages(t *testing.T) {
	home := t.TempDir()
	options := app.Options{
		ConfigPath: filepath.Join(home, "config.yaml"),
		UserHomeDir: func() (string, error) {
			return home, nil
		},
	}
	run := func(args ...string) string {
		t.Helper()
		var stdout bytes.Buffer
		if exitCode := app.Run(t.Context(), args, &stdout, options); exitCode != 0 {
			t.Fatalf("Run(%v) exit = %d, output = %s", args, exitCode, stdout.String())
		}
		return stdout.String()
	}
	run("env", "add", "alpha", "--url", "https://alpha.example.test")
	run("env", "add", "beta", "--url", "https://beta.example.test")
	run("workspace", "create", "alpha", "--path", filepath.Join(home, "workspaces", "alpha"))
	run("workspace", "create", "beta", "--path", filepath.Join(home, "workspaces", "beta"))

	var environments struct {
		Page struct {
			Returned      int  `json:"returned"`
			Total         int  `json:"total"`
			Limit         int  `json:"limit"`
			MoreAvailable bool `json:"more_available"`
		} `json:"page"`
		Environments []struct {
			Alias string `json:"alias"`
		} `json:"environments"`
	}
	environmentOutput := run("env", "list", "--all", "--full", "--json")
	decodeLocalListAll(t, environmentOutput, &environments)
	if environments.Page.Returned != 2 || environments.Page.Total != 2 || environments.Page.Limit != 10000 || environments.Page.MoreAvailable || len(environments.Environments) != 2 {
		t.Fatalf("environment page = %#v, output = %s", environments.Page, environmentOutput)
	}

	var workspaces struct {
		Page struct {
			Returned      int  `json:"returned"`
			Total         int  `json:"total"`
			Limit         int  `json:"limit"`
			MoreAvailable bool `json:"more_available"`
		} `json:"page"`
		Workspaces []struct {
			Name string `json:"name"`
		} `json:"workspaces"`
	}
	workspaceOutput := run("workspace", "list", "--all", "--full", "--json")
	decodeLocalListAll(t, workspaceOutput, &workspaces)
	if workspaces.Page.Returned != 2 || workspaces.Page.Total != 2 || workspaces.Page.Limit != 10000 || workspaces.Page.MoreAvailable || len(workspaces.Workspaces) != 2 {
		t.Fatalf("workspace page = %#v, output = %s", workspaces.Page, workspaceOutput)
	}

	var status struct {
		Inventory struct {
			Returned      int  `json:"returned"`
			Total         int  `json:"total"`
			Limit         int  `json:"limit"`
			MoreAvailable bool `json:"more_available"`
			ScanComplete  bool `json:"scan_complete"`
		} `json:"artifacts"`
	}
	statusOutput := run("workspace", "status", "--workspace", "alpha", "--all", "--full", "--json")
	decodeLocalListAll(t, statusOutput, &status)
	if status.Inventory.Returned != 0 || status.Inventory.Total != 0 || status.Inventory.Limit != 10000 || status.Inventory.MoreAvailable || !status.Inventory.ScanComplete {
		t.Fatalf("workspace status inventory = %#v, output = %s", status.Inventory, statusOutput)
	}
}

func decodeLocalListAll(t *testing.T, rendered string, target any) {
	t.Helper()
	if strings.Contains(rendered, "next_cursor") || strings.Contains(rendered, "next_command") {
		t.Fatalf("complete --all output exposed continuation: %s", rendered)
	}
	if err := json.Unmarshal([]byte(rendered), target); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, rendered)
	}
}
