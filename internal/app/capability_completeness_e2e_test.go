package app_test

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/capability"
)

func TestCapabilityListAllMatchesRegistryInventory(t *testing.T) {
	var stdout bytes.Buffer
	home := t.TempDir()
	options := app.Options{
		ConfigPath: filepath.Join(home, "config.yaml"),
		UserHomeDir: func() (string, error) {
			return home, nil
		},
	}
	exitCode := app.Run(t.Context(), []string{"capability", "list", "--all", "--full", "--json"}, &stdout, options)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}

	var document struct {
		Page struct {
			Returned      int  `json:"returned"`
			Total         int  `json:"total"`
			Limit         int  `json:"limit"`
			MoreAvailable bool `json:"more_available"`
		} `json:"page"`
		Capabilities []struct {
			ID string `json:"id"`
		} `json:"capabilities"`
		NextCommand string `json:"next_command"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &document); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout.String())
	}

	expected := capability.AllDiscoveries()
	if document.Page.Returned != len(expected) || document.Page.Total != len(expected) || document.Page.Limit != 10000 || document.Page.MoreAvailable || document.NextCommand != "" {
		t.Fatalf("page = %#v, next_command = %q, registry size = %d", document.Page, document.NextCommand, len(expected))
	}
	expectedIDs := make(map[string]struct{}, len(expected))
	for _, item := range expected {
		expectedIDs[item.ID] = struct{}{}
	}
	for _, item := range document.Capabilities {
		if _, ok := expectedIDs[item.ID]; !ok {
			t.Fatalf("unexpected capability %q", item.ID)
		}
		delete(expectedIDs, item.ID)
	}
	if len(expectedIDs) != 0 {
		t.Fatalf("missing capabilities: %v", expectedIDs)
	}
}

func TestCapabilityListHelpExposesBoundedContinuationFlags(t *testing.T) {
	home := t.TempDir()
	options := app.Options{
		ConfigPath: filepath.Join(home, "config.yaml"),
		UserHomeDir: func() (string, error) {
			return home, nil
		},
	}
	var stdout bytes.Buffer
	exitCode := app.Run(t.Context(), []string{"capability", "list", "--help"}, &stdout, options)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	for _, flag := range []string{"--all", "--cursor"} {
		if !strings.Contains(stdout.String(), flag) {
			t.Fatalf("help output does not contain %s:\n%s", flag, stdout.String())
		}
	}
}

func TestUnsupportedCursorFlagsRemainHidden(t *testing.T) {
	home := t.TempDir()
	options := app.Options{
		ConfigPath: filepath.Join(home, "config.yaml"),
		UserHomeDir: func() (string, error) {
			return home, nil
		},
	}
	var stdout bytes.Buffer
	exitCode := app.Run(t.Context(), []string{"content", "datasource", "schema", "--help"}, &stdout, options)
	if exitCode != 0 {
		t.Fatalf("exit code = %d, output = %s", exitCode, stdout.String())
	}
	if strings.Contains(stdout.String(), "--cursor") {
		t.Fatalf("unsupported datasource schema cursor leaked into help:\n%s", stdout.String())
	}
}
