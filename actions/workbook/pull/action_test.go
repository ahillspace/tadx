package pull_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	workbook pull.Workbook
	download pull.Download
	err      error
}

func (r reader) ResolveWorkbook(context.Context, identity.Selector) (pull.Workbook, error) {
	return r.workbook, r.err
}

func (r reader) DownloadWorkbook(context.Context, string, *bool) (pull.Download, error) {
	return r.download, r.err
}

type writer struct {
	input  pull.Artifact
	result pull.ArtifactResult
	err    error
}

func (w *writer) WriteWorkbook(_ context.Context, input pull.Artifact) (pull.ArtifactResult, error) {
	w.input = input
	return w.result, w.err
}

func TestActionPullsOneResolvedWorkbookIntoArtifact(t *testing.T) {
	w := &writer{result: pull.ArtifactResult{Path: `C:\workspace\artifacts\workbook\Finance`, BaselineFingerprint: "sha256:abc"}}
	action := pull.New(reader{
		workbook: pull.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"},
		download: pull.Download{Filename: "Finance.twbx", Content: []byte("native")},
	}, w)
	include := false
	output, err := action.Execute(context.Background(), pull.Input{
		Environment: "production", Site: "marketing", Workspace: `C:\workspace`,
		Selector: identity.Selector{Name: "Finance", ProjectPath: "Ops"}, IncludeExtract: &include,
	})
	if err != nil {
		t.Fatal(err)
	}
	if output.Workbook.LUID != "wb-1" || output.Artifact.BaselineFingerprint != "sha256:abc" || w.input.TableauID != "wb-1" {
		t.Fatalf("output = %#v, artifact input = %#v", output, w.input)
	}
}

func TestActionLeavesArtifactWriterUntouchedWhenDownloadFails(t *testing.T) {
	w := &writer{}
	action := pull.New(reader{workbook: pull.Workbook{LUID: "wb-1"}, err: errors.New("download forbidden")}, w)
	_, err := action.Execute(context.Background(), pull.Input{Environment: "production", Site: "marketing", Workspace: `C:\workspace`, Selector: identity.Selector{LUID: "wb-1"}})
	if err == nil || !strings.Contains(err.Error(), "download forbidden") || w.input.TableauID != "" {
		t.Fatalf("error = %v, write input = %#v", err, w.input)
	}
}

func TestActionGoldenOutput(t *testing.T) {
	value := pull.Output{
		Status:   "pulled",
		Workbook: pull.Workbook{LUID: "wb-1", Name: "Finance", ProjectLUID: "project-1", ProjectPath: "Ops"},
		Artifact: pull.ArtifactResult{Path: `C:\workspace\artifacts\workbook\Finance`, CanonicalPath: `C:\workspace\artifacts\workbook\Finance\Finance.twbx`, BaselineFingerprint: "sha256:abc"},
		Warnings: []string{"existing clean artifact replaced"}, RequestID: "request-1",
	}
	var actual bytes.Buffer
	if err := output.Render(&actual, value); err != nil {
		t.Fatal(err)
	}
	expected, err := os.ReadFile("testdata/output.toon")
	if err != nil {
		t.Fatal(err)
	}
	expected = bytes.TrimSuffix(expected, []byte("\n"))
	if !bytes.Equal(actual.Bytes(), expected) {
		t.Fatalf("golden mismatch\nexpected:\n%s\nactual:\n%s", expected, actual.Bytes())
	}
}
