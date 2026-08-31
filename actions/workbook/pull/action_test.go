package pull_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/actions/workbook/pull"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/identity"
	"github.com/ahillspace/tadx/internal/output"
)

type reader struct {
	workbook      pull.Workbook
	download      pull.Download
	resolveErr    error
	downloadErr   error
	downloadCalls int
}

func (r *reader) ResolveWorkbook(context.Context, identity.Selector) (pull.Workbook, error) {
	return r.workbook, r.resolveErr
}

func (r *reader) DownloadWorkbook(context.Context, string, *bool) (pull.Download, error) {
	r.downloadCalls++
	return r.download, r.downloadErr
}

type writer struct {
	input  pull.Artifact
	result pull.ArtifactResult
	err    error
	calls  int
}

type retryableReadError struct{}

func (retryableReadError) Error() string            { return "Tableau unavailable" }
func (retryableReadError) Retryable() bool          { return true }
func (retryableReadError) CorrectiveAction() string { return "Retry after Tableau recovers." }

func (w *writer) WriteWorkbook(_ context.Context, input pull.Artifact) (pull.ArtifactResult, error) {
	w.calls++
	w.input = input
	return w.result, w.err
}

func TestActionPullsOneResolvedWorkbookIntoArtifact(t *testing.T) {
	w := &writer{result: pull.ArtifactResult{Path: `C:\workspace\artifacts\workbook\Finance`, BaselineFingerprint: "sha256:abc"}}
	action := pull.New(&reader{
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
	r := &reader{workbook: pull.Workbook{LUID: "wb-1"}, downloadErr: errors.New("download forbidden")}
	w := &writer{}
	action := pull.New(r, w)
	_, err := action.Execute(context.Background(), pull.Input{Environment: "production", Site: "marketing", Workspace: `C:\workspace`, Selector: identity.Selector{LUID: "wb-1"}})
	if err == nil || !strings.Contains(err.Error(), "download forbidden") || r.downloadCalls != 1 || w.calls != 0 {
		t.Fatalf("error = %v, download calls = %d, write calls = %d", err, r.downloadCalls, w.calls)
	}
}

func TestActionPreservesRetryAdviceForWorkbookReads(t *testing.T) {
	for _, test := range []struct {
		name   string
		reader *reader
	}{
		{name: "resolve", reader: &reader{resolveErr: retryableReadError{}}},
		{name: "download", reader: &reader{workbook: pull.Workbook{LUID: "wb-1"}, downloadErr: retryableReadError{}}},
	} {
		t.Run(test.name, func(t *testing.T) {
			_, err := pull.New(test.reader, &writer{}).Execute(context.Background(), pull.Input{Environment: "production", Site: "marketing", Selector: identity.Selector{LUID: "wb-1"}})
			payload := errs.Structure(err).Error
			if payload.Retryable == nil || !*payload.Retryable || payload.CorrectiveAction != "Retry after Tableau recovers." {
				t.Fatalf("structured error = %#v", payload)
			}
		})
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
