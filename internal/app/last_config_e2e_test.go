package app_test

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
	"github.com/ahillspace/tadx/internal/value"
)

func TestLastResultUsesExplicitConfigurationDirectoryThroughCLI(t *testing.T) {
	for _, flag := range []string{"--config", "--cfg"} {
		t.Run(flag, func(t *testing.T) {
			root := t.TempDir()
			defaultConfig := filepath.Join(root, "default", "config.yaml")
			selectedConfig := filepath.Join(root, "selected", "config.yaml")
			options := app.Options{ConfigPath: defaultConfig}
			run := func(args ...string) string {
				t.Helper()
				var out bytes.Buffer
				if code := app.Run(t.Context(), args, &out, options); code != 0 {
					t.Fatalf("args=%v exit=%d output=%s", args, code, &out)
				}
				return out.String()
			}
			run("capability", "get", "version.get", "--json")
			defaultResult := filepath.Join(filepath.Dir(defaultConfig), "last-result.json")
			original, err := os.ReadFile(defaultResult)
			if err != nil {
				t.Fatal(err)
			}
			run("version", flag, selectedConfig, "--json")
			selectedResult := filepath.Join(filepath.Dir(selectedConfig), "last-result.json")
			selected, err := os.ReadFile(selectedResult)
			if err != nil {
				t.Fatalf("explicit configuration did not receive its saved result: %v", err)
			}
			unchanged, err := os.ReadFile(defaultResult)
			if err != nil || !bytes.Equal(original, unchanged) {
				t.Fatalf("selected configuration changed the default saved result: err=%v", err)
			}
			var record value.SavedExecution
			if err := json.Unmarshal(selected, &record); err != nil || record.Operation != "version.get" {
				t.Fatalf("selected record operation=%q err=%v", record.Operation, err)
			}
			var displayed value.SavedExecution
			if err := json.Unmarshal([]byte(run("last", flag, selectedConfig, "--json")), &displayed); err != nil || displayed.Operation != "version.get" {
				t.Fatalf("selected last operation=%q err=%v", displayed.Operation, err)
			}
			preserved, err := os.ReadFile(selectedResult)
			if err != nil || !bytes.Equal(selected, preserved) {
				t.Fatalf("last replaced its selected saved result: err=%v", err)
			}
		})
	}
}
