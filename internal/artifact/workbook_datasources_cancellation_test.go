package artifact

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"os"
	"path/filepath"
	"testing"
)

func TestRewriteWorkbookPackageStopsCancelledArchiveReads(t *testing.T) {
	for _, phase := range []string{"archive", "definition header", "definition body", "extract before definition", "extract after definition"} {
		t.Run(phase, func(t *testing.T) {
			source := cancellationWorkbookPackage(t, phase == "extract before definition")
			archive, err := zip.NewReader(bytes.NewReader(source), int64(len(source)))
			if err != nil {
				t.Fatal(err)
			}
			var offset int64
			for _, entry := range archive.File {
				selected := entry.Name == "Book.twb"
				if selected != (phase == "definition header" || phase == "definition body") {
					continue
				}
				offset, err = entry.DataOffset()
				if err != nil {
					t.Fatal(err)
				}
				if phase == "definition header" {
					offset -= int64(30 + len(entry.Name))
				}
				break
			}
			ctx, cancel := context.WithCancel(t.Context())
			defer cancel()
			reader := &cancelWorkbookReaderAt{source: bytes.NewReader(source), ctx: ctx, cancel: cancel, offset: offset, first: phase == "archive"}
			var output bytes.Buffer
			_, err = rewriteWorkbookPackage(ctx, reader, int64(len(source)), &output, cancellationWorkbookTarget(t))
			if !reader.canceled || !errors.Is(err, context.Canceled) {
				t.Fatalf("canceled=%t error=%v, want cancellation from %s", reader.canceled, err, phase)
			}
			if reader.readsAfterCancel != 0 {
				t.Fatalf("continued with %d archive reads after cancellation", reader.readsAfterCancel)
			}
			if phase == "extract after definition" && !bytes.Contains(output.Bytes(), []byte("dbname='new'")) {
				t.Fatal("cancellation did not occur after the rewritten definition was emitted")
			}
			if output.Len() >= len(source)/2 {
				t.Fatalf("copied too much of canceled package: %d of %d bytes", output.Len(), len(source))
			}
		})
	}
}

func TestRewriteWorkbookPackageReportsCancellationDuringFinalWrite(t *testing.T) {
	source := workbookPackageFixture(t, workbookDatasourceFixture)
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	output := &cancelWorkbookWriter{cancel: cancel}
	_, err := rewriteWorkbookPackage(ctx, bytes.NewReader(source), int64(len(source)), output, cancellationWorkbookTarget(t))
	if !output.canceled || !errors.Is(err, context.Canceled) {
		t.Fatalf("written=%d canceled=%t error=%v, want cancellation during final directory output", output.Len(), output.canceled, err)
	}
}

func TestRewriteWorkbookDatasourcesRemovesCancelledUploadCopy(t *testing.T) {
	directory := t.TempDir()
	path := filepath.Join(directory, "Book.twbx")
	source := workbookPackageFixture(t, workbookDatasourceFixture)
	if err := os.WriteFile(path, source, 0o600); err != nil {
		t.Fatal(err)
	}
	request := workbookRewriteRequest(t, path, "https://target.example.com", "target-site")
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	// Observe the real temporary file to cancel after fingerprint validation,
	// without adding production hooks or relying on scheduler timing.
	observed := &cancelWhenWorkbookCopyExists{Context: ctx, cancel: cancel, directory: directory}
	rewritten, err := RewriteWorkbookDatasources(observed, request)
	if !observed.canceled || !errors.Is(err, context.Canceled) || rewritten.Path != "" {
		t.Fatalf("canceled=%t result=%#v error=%v", observed.canceled, rewritten, err)
	}
	entries, err := os.ReadDir(directory)
	if err != nil || len(entries) != 1 || entries[0].Name() != "Book.twbx" {
		t.Fatalf("canceled rewrite left temporary files: %v, %v", entries, err)
	}
	after, err := os.ReadFile(path)
	if err != nil || !bytes.Equal(after, source) {
		t.Fatalf("canceled rewrite changed source: %v", err)
	}
}

func cancellationWorkbookPackage(t *testing.T, extractFirst bool) []byte {
	t.Helper()
	var source bytes.Buffer
	archive := zip.NewWriter(&source)
	names := []string{"Book.twb", "Data/Extract.hyper"}
	if extractFirst {
		names[0], names[1] = names[1], names[0]
	}
	for _, name := range names {
		entry, err := archive.CreateHeader(&zip.FileHeader{Name: name, Method: zip.Store})
		if err != nil {
			t.Fatal(err)
		}
		var content []byte
		if name == "Book.twb" {
			content = []byte("<workbook><datasources><datasource><connection class='sqlproxy' dbname='old'/></datasource></datasources>" + string(bytes.Repeat([]byte("<item/>"), 1500)) + "</workbook>")
		} else {
			content = bytes.Repeat([]byte("raw-extract-contents"), 14000)
		}
		if _, err := entry.Write(content); err != nil {
			t.Fatal(err)
		}
	}
	if err := archive.Close(); err != nil {
		t.Fatal(err)
	}
	return source.Bytes()
}

func cancellationWorkbookTarget(t *testing.T) *datasourceRebinding {
	t.Helper()
	target, err := newDatasourceRebinding(WorkbookDatasourceRewrite{ServerOrigin: "https://target.example.com", ContentURLs: map[string]string{"old": "new", "SalesMargin": "SalesMargin_1"}})
	if err != nil {
		t.Fatal(err)
	}
	return target
}

type cancelWorkbookReaderAt struct {
	source           io.ReaderAt
	ctx              context.Context
	cancel           context.CancelFunc
	offset           int64
	first            bool
	canceled         bool
	readsAfterCancel int
}

func (r *cancelWorkbookReaderAt) ReadAt(data []byte, offset int64) (int, error) {
	if r.ctx.Err() != nil {
		r.readsAfterCancel++
	}
	n, err := r.source.ReadAt(data, offset)
	if !r.canceled && (r.first || offset == r.offset) {
		r.canceled = true
		r.cancel()
	}
	return n, err
}

type cancelWorkbookWriter struct {
	bytes.Buffer
	cancel   context.CancelFunc
	canceled bool
}

func (w *cancelWorkbookWriter) Write(data []byte) (int, error) {
	n, err := w.Buffer.Write(data)
	if bytes.Contains(data, []byte{'P', 'K', 1, 2}) {
		w.canceled = true
		w.cancel()
	}
	return n, err
}

type cancelWhenWorkbookCopyExists struct {
	context.Context
	cancel    context.CancelFunc
	directory string
	canceled  bool
}

func (c *cancelWhenWorkbookCopyExists) Err() error {
	if !c.canceled {
		matches, _ := filepath.Glob(filepath.Join(c.directory, ".tadx-publish-rewrite-*"))
		if len(matches) != 0 {
			c.canceled = true
			c.cancel()
		}
	}
	return c.Context.Err()
}
