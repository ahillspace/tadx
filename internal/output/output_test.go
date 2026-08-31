package output_test

import (
	"bytes"
	"errors"
	"os"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type projectableResult struct {
	Status string `json:"status"`
	Secret string `json:"secret"`
}

func (r projectableResult) CompactOutput() any {
	return struct {
		Status  string `json:"status"`
		Details string `json:"details"`
	}{Status: r.Status, Details: "--full"}
}

func (r projectableResult) FullOutput() any {
	return struct {
		Status string `json:"status"`
		Secret string `json:"secret"`
	}{Status: r.Status, Secret: r.Secret}
}

func TestRenderDefaultTOON(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	if err := output.Render(&buffer, struct {
		Name  string `json:"name"`
		Count int    `json:"count"`
	}{Name: "workbooks", Count: 2}); err != nil {
		t.Fatal(err)
	}
	if got, want := buffer.String(), "name: workbooks\ncount: 2"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderProjectsCompactOutputAndFullPreservesOriginal(t *testing.T) {
	t.Parallel()

	value := projectableResult{Status: "ready", Secret: "diagnostic"}
	var compact bytes.Buffer
	if err := output.RenderWithOptions(&compact, value, output.Options{}); err != nil {
		t.Fatal(err)
	}
	if got := compact.String(); got != "status: ready\ndetails: \"--full\"" {
		t.Fatalf("compact output = %q", got)
	}

	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, value, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if got := full.String(); got != "status: ready\nsecret: diagnostic" {
		t.Fatalf("full output = %q", got)
	}
}

func TestRenderRedactsAfterCompactProjection(t *testing.T) {
	t.Parallel()

	value := projectableResult{Status: "secret-value", Secret: "secret-value"}
	for _, full := range []bool{false, true} {
		var rendered bytes.Buffer
		if err := output.RenderWithOptions(&rendered, value, output.Options{Full: full, Secrets: []string{"secret-value"}}); err != nil {
			t.Fatal(err)
		}
		if strings.Contains(rendered.String(), "secret-value") || !strings.Contains(rendered.String(), output.Redacted) {
			t.Fatalf("full=%v output = %q", full, rendered.String())
		}
	}
}

func TestRenderCompactAndFullGolden(t *testing.T) {
	t.Parallel()

	value := map[string]any{"body": "abcdefghij"}
	var compact bytes.Buffer
	if err := output.RenderWithOptions(&compact, value, output.Options{MaxStringLength: 5}); err != nil {
		t.Fatal(err)
	}
	if got, want := compact.String(), readGolden(t, "testdata/compact.golden"); got != want {
		t.Fatalf("compact got %q, want %q", got, want)
	}
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, value, output.Options{Full: true, MaxStringLength: 5}); err != nil {
		t.Fatal(err)
	}
	if got, want := full.String(), readGolden(t, "testdata/full.golden"); got != want {
		t.Fatalf("full got %q, want %q", got, want)
	}
}

func readGolden(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return strings.TrimSuffix(string(data), "\n")
}

func TestRedactionPrecedesTOONAndRawRendering(t *testing.T) {
	t.Parallel()

	secret := "s3cr3t-token"
	value := map[string]any{"authorization": "Bearer " + secret, "nested": []any{secret}}
	for _, raw := range []bool{false, true} {
		var buffer bytes.Buffer
		err := output.RenderWithOptions(&buffer, value, output.Options{Raw: raw, RawCapable: true, Secrets: []string{secret}})
		if err != nil {
			t.Fatal(err)
		}
		if strings.Contains(buffer.String(), secret) {
			t.Fatalf("secret leaked in raw=%t output: %s", raw, buffer.String())
		}
		if !strings.Contains(buffer.String(), output.Redacted) {
			t.Fatalf("redaction marker missing: %s", buffer.String())
		}
	}
}

func TestRawCapabilityGate(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	err := output.RenderWithOptions(&buffer, "payload", output.Options{Raw: true})
	if err == nil || errs.ExitCode(err) != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestRenderStructuredError(t *testing.T) {
	t.Parallel()

	var buffer bytes.Buffer
	err := &errs.Error{Kind: errs.KindOperation, Operation: "catalog.search", Summary: "Search failed", Cause: errors.New("timeout"), TableauRequestID: "req-1"}
	if renderErr := output.RenderError(&buffer, err, output.Options{}); renderErr != nil {
		t.Fatal(renderErr)
	}
	want := "error:\n  kind: operation\n  operation: catalog.search\n  summary: Search failed\n  upstream_cause: timeout\n  tableau_request_id: req-1"
	if buffer.String() != want {
		t.Fatalf("render mismatch\nwant:\n%s\ngot:\n%s", want, buffer.String())
	}
}
