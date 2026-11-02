package app

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/managedpolicy"
)

func TestDeniedBatchPreparationDoesNotAuthenticateOrReadTargets(t *testing.T) {
	for _, test := range []struct {
		name, body string
		command    []string
	}{
		{"workbook alias", `{"items":[{"id":"w1","description":"A"},{"name":"Workbook","project":"Project","description":"B"}]}`, []string{"content", "workbook", "update", "--env", "dev"}},
		{"membership username", `{"items":[{"group-id":"g1","username":"alice"}]}`, []string{"admin", "group-member", "add", "--env", "dev"}},
		{"permission username", `{"items":[{"kind":"workbook","id":"w1","principal-type":"user","principal-username":"alice","capability":"Read","mode":"Allow"}]}`, []string{"admin", "permission", "create", "--env", "dev"}},
		{"label target", `{"items":[{"type":"datasource","target-id":"d1","value":"Reviewed"}]}`, []string{"catalog", "label", "update", "--env", "dev"}},
		{"local artifact", `{"items":[{"kind":"workbook","id":"w1"}]}`, []string{"workspace", "artifact", "delete", "--workspace", "source"}},
	} {
		t.Run(test.name, func(t *testing.T) {
			options := overviewOptions(t, t.TempDir())
			options.ConfigPath = authConfig(t, "")
			options.managedPolicy = fixtureManagedPolicy{state: managedpolicy.StateActive, allowed: map[string]bool{
				"workbook.inspect": true, "project.inspect": true, "admin.user.inspect": true,
				"content.label.inspect": true, "admin.label.value.inspect": true,
			}}
			file := filepath.Join(t.TempDir(), "items.json")
			if err := os.WriteFile(file, []byte(test.body), 0600); err != nil {
				t.Fatal(err)
			}
			var output bytes.Buffer
			args := append(append([]string(nil), test.command...), "--batch-file", file, "--preview", "--json")
			code := Run(t.Context(), args, &output, options)
			if code == 0 || !strings.Contains(output.String(), "policy.denied") {
				t.Fatalf("code=%d output=%s", code, output.String())
			}
		})
	}
}
