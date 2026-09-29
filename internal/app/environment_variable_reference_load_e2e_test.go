package app_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestConfigurationWithSecretShapedPATReferenceFailsWithoutEchoing(t *testing.T) {
	const secret = "abc123DEF==:ghiJKL456"
	root := t.TempDir()
	path := filepath.Join(root, "config.yaml")
	data := "version: 1\ndefault_environment: main\nenvironments:\n  main:\n    url: https://tableau.example.test\n    auth:\n      type: pat\n      pat_name_env: MAIN_PAT_NAME\n      pat_secret_env: \"" + secret + "\"\n"
	if err := os.WriteFile(path, []byte(data), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, args := range [][]string{{"env", "get", "main"}, {"env", "list", "--full"}, {"auth", "status", "--json"}, {"env", "update", "main", "--clear-pat-secret-env"}} {
		var out strings.Builder
		code := app.Run(context.Background(), args, &out, app.Options{ConfigPath: path})
		if code == 0 {
			t.Fatalf("%v accepted a configuration whose PAT reference is not a variable name:\n%s", args, out.String())
		}
		if strings.Contains(out.String(), secret) {
			t.Fatalf("%v echoed the stored reference: %s", args, out.String())
		}
	}
}
