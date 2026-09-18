package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
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
					args = append(args, "--environment", "production", "--project-id", "project-1", "--artifact", "artifacts/"+kind+"/first", "--artifact", "artifacts/"+kind+"/second", "--preview")
					if kind == "datasource" {
						args = append(args, "--create")
					}
				}
				var stdout bytes.Buffer
				exit := app.Run(context.Background(), args, &stdout, withSiteMutationConsent(t, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}, true))
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

func TestContentBatchFailuresRenderOneJSONDocument(t *testing.T) {
	args := []string{"content", "workbook", "pull", "--id", "first", "--id", "second", "--json"}
	var stdout bytes.Buffer
	exit := app.Run(context.Background(), args, &stdout, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")})
	if exit == 0 {
		t.Fatalf("expected nonzero exit, output=%s", stdout.String())
	}
	decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
	var document map[string]any
	if err := decoder.Decode(&document); err != nil {
		t.Fatalf("output is not JSON: %v\n%s", err, stdout.String())
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		t.Fatalf("expected one JSON document, extra=%v err=%v", extra, err)
	}
	if document["operation"] != "workbook.pull" || document["status"] != "failed" {
		t.Fatalf("unexpected batch document: %#v", document)
	}
}
