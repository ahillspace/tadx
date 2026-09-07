package app_test

import (
	"bytes"
	"context"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestContentBatchFailuresRenderOneDocumentAndReturnNonzero(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, verb := range []string{"pull", "publish"} {
			t.Run(kind+"."+verb, func(t *testing.T) {
				args := []string{"content", kind, verb}
				if verb == "pull" {
					args = append(args, "--id", "first", "--id", "second")
				} else {
					args = append(args, "--artifact", "artifacts/"+kind+"/first", "--artifact", "artifacts/"+kind+"/second", "--preview")
					if kind == "datasource" {
						args = append(args, "--create")
					}
				}
				var stdout bytes.Buffer
				exit := app.Run(context.Background(), args, &stdout, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml"), MutationsEnabled: true})
				text := stdout.String()
				if exit == 0 || !strings.HasPrefix(text, "operation: "+kind+"."+verb+"\n") || !strings.Contains(text, "failed: 2") || !strings.Contains(text, "first") || !strings.Contains(text, "second") {
					t.Fatalf("exit=%d, output=%s", exit, text)
				}
				if strings.Contains(text, "\nerror:\n") {
					t.Fatalf("duplicate error document: %s", text)
				}
			})
		}
	}
}
