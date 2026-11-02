package app_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestContentPullBatchFailuresRenderOneDocumentAndReturnNonzero(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		t.Run(kind, func(t *testing.T) {
			args := []string{"content", kind, "pull", "--id", "first", "--id", "second"}
			var stdout bytes.Buffer
			exit := app.Run(t.Context(), args, &stdout, withSiteMutationConsent(t, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}, true))
			text := stdout.String()
			if exit == 0 || !strings.HasPrefix(text, "operation: "+kind+".pull\n") || !strings.Contains(text, "failed: 2") || !strings.Contains(text, "first") || !strings.Contains(text, "second") {
				t.Fatalf("exit=%d, output=%s", exit, text)
			}
			if strings.Contains(text, "\nerror:\n") {
				t.Fatalf("duplicate error document: %s", text)
			}
		})
	}
}

func TestContentPublicationBatchRejectsMissingWorkspaceBeforeDispatch(t *testing.T) {
	for _, kind := range []string{"workbook", "datasource", "flow"} {
		for _, jsonOutput := range []bool{false, true} {
			name := kind + "/compact"
			if jsonOutput {
				name = kind + "/json"
			}
			t.Run(name, func(t *testing.T) {
				args := []string{"content", kind, "publish", "--environment", "production", "--project-id", "project-1", "--artifact", "artifacts/" + kind + "/first", "--artifact", "artifacts/" + kind + "/second", "--preview"}
				if kind == "datasource" {
					args = append(args, "--create")
				}
				if jsonOutput {
					args = append(args, "--json")
				}
				var stdout bytes.Buffer
				exit := app.Run(t.Context(), args, &stdout, withSiteMutationConsent(t, app.Options{ConfigPath: filepath.Join(t.TempDir(), "missing.yaml")}, true))
				if exit == 0 {
					t.Fatalf("missing prerequisite succeeded: %s", stdout.String())
				}
				if !jsonOutput {
					text := stdout.String()
					if !strings.HasPrefix(text, "error:\n") || strings.Contains(text, "\nerror:\n") || strings.Contains(text, "failed:") || !strings.Contains(text, "phase: setup") || !strings.Contains(text, "outcome: not_attempted") || !strings.Contains(text, "kind: workspace") {
						t.Fatalf("preflight failure was not one before-dispatch document: %s", text)
					}
					return
				}
				decoder := json.NewDecoder(bytes.NewReader(stdout.Bytes()))
				var document struct {
					Error struct {
						ID, Operation, Phase, Outcome string
						Prerequisite                  struct{ Kind string }
					}
					Items []any
				}
				if err := decoder.Decode(&document); err != nil {
					t.Fatalf("invalid JSON: %v\n%s", err, stdout.String())
				}
				if err := decoder.Decode(new(any)); !errors.Is(err, io.EOF) {
					t.Fatalf("expected one document: %v\n%s", err, stdout.String())
				}
				if document.Error.ID != kind+".publish.workspace" || document.Error.Operation != kind+".publish" || document.Error.Phase != "setup" || document.Error.Outcome != "not_attempted" || document.Error.Prerequisite.Kind != "workspace" || document.Items != nil {
					t.Fatalf("unexpected preflight result: %#v", document)
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
