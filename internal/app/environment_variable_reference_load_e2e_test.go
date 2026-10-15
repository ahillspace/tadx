package app_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestConfigurationWithSecretShapedPATReferenceIsIsolatedAndRepairable(t *testing.T) {
	const secret = "abc123DEF==:ghiJKL456"
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	data := "version: 1\ndefault_environment: main\nenvironments:\n  main:\n    url: https://tableau.example.test\n    auth:\n      type: pat\n      pat_name_env: MAIN_PAT_NAME\n      pat_secret_env: \"" + secret + "\"\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, scenario := range []struct {
		args    []string
		success bool
		invalid bool
	}{
		{[]string{"env", "get", "main"}, true, true},
		{[]string{"env", "list", "--full"}, true, true},
		{[]string{"auth", "status", "--json"}, false, true},
		{[]string{"env", "update", "main", "--clear-pat-secret-env"}, true, false},
	} {
		var out strings.Builder
		code := app.Run(t.Context(), scenario.args, &out, app.Options{ConfigPath: path})
		if (code == 0) != scenario.success {
			t.Fatalf("%v exit=%d expected success=%t:\n%s", scenario.args, code, scenario.success, out.String())
		}
		if scenario.invalid && (!strings.Contains(out.String(), "invalid") || !strings.Contains(out.String(), "pat_secret_env")) {
			t.Fatalf("%v omitted the invalid reference finding: %s", scenario.args, out.String())
		}
		if strings.Contains(out.String(), secret) {
			t.Fatalf("%v echoed the stored reference: %s", scenario.args, out.String())
		}
	}
	after, err := os.ReadFile(path)
	if err != nil || strings.Contains(string(after), secret) {
		t.Fatalf("repair retained the rejected reference: %v", err)
	}
}
