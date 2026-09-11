package output_test

import (
	"bytes"
	"errors"
	"os"
	"reflect"
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
	if got, want := buffer.String(), "name: workbooks\ncount: 2\n"; got != want {
		t.Fatalf("got %q, want %q", got, want)
	}
}

func TestRenderJSONPreservesProjectionAndRedaction(t *testing.T) {
	t.Parallel()

	value := projectableResult{Status: "ready", Secret: "token"}
	var compact bytes.Buffer
	if err := output.RenderWithOptions(&compact, value, output.Options{JSON: true}); err != nil {
		t.Fatal(err)
	}
	if got, want := compact.String(), "{\"details\":\"--full\",\"status\":\"ready\"}\n"; got != want {
		t.Fatalf("compact JSON output = %q, want %q", got, want)
	}

	var buffer bytes.Buffer
	if err := output.RenderWithOptions(&buffer, value, output.Options{JSON: true, Full: true, Secrets: []string{"token"}}); err != nil {
		t.Fatal(err)
	}
	if got, want := buffer.String(), "{\"secret\":\"[REDACTED]\",\"status\":\"ready\"}\n"; got != want {
		t.Fatalf("JSON output = %q, want %q", got, want)
	}
}

func TestRenderJSONRejectsRawOutput(t *testing.T) {
	var buffer bytes.Buffer
	err := output.RenderWithOptions(&buffer, "payload", output.Options{JSON: true, Raw: true, RawCapable: true})
	if err == nil || errs.ExitCode(err) != 2 {
		t.Fatalf("expected usage error, got %v", err)
	}
}

func TestRenderProjectsCompactOutputAndFullPreservesOriginal(t *testing.T) {
	t.Parallel()

	value := projectableResult{Status: "ready", Secret: "diagnostic"}
	var compact bytes.Buffer
	if err := output.RenderWithOptions(&compact, value, output.Options{}); err != nil {
		t.Fatal(err)
	}
	if got := compact.String(); got != "status: ready\ndetails: \"--full\"\n" {
		t.Fatalf("compact output = %q", got)
	}

	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, value, output.Options{Full: true}); err != nil {
		t.Fatal(err)
	}
	if got := full.String(); got != "status: ready\nsecret: diagnostic\n" {
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
	if got, want := compact.String(), readGolden(t, "testdata/compact.golden")+"\n"; got != want {
		t.Fatalf("compact got %q, want %q", got, want)
	}
	var full bytes.Buffer
	if err := output.RenderWithOptions(&full, value, output.Options{Full: true, MaxStringLength: 5}); err != nil {
		t.Fatal(err)
	}
	if got, want := full.String(), readGolden(t, "testdata/full.golden")+"\n"; got != want {
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
	err := &errs.Error{Kind: errs.KindOperation, Operation: "cache.search", Summary: "Search failed", Cause: errors.New("timeout"), TableauRequestID: "req-1"}
	if renderErr := output.RenderError(&buffer, err, output.Options{}); renderErr != nil {
		t.Fatal(renderErr)
	}
	want := "error:\n  kind: operation\n  operation: cache.search\n  summary: Search failed\n  upstream_cause: timeout\n  tableau_request_id: req-1\n"
	if buffer.String() != want {
		t.Fatalf("render mismatch\nwant:\n%s\ngot:\n%s", want, buffer.String())
	}
}

func TestConfigPathBindsOnlyRecoveryHints(t *testing.T) {
	t.Parallel()

	value := struct {
		Help             []string `json:"help"`
		CorrectiveAction string   `json:"corrective_action"`
		Resource         string   `json:"resource"`
	}{[]string{"Run tadx cache status --full."}, "Run tadx auth status, then retry.", "tadx cache status"}
	var buffer bytes.Buffer
	if err := output.RenderWithOptions(&buffer, value, output.Options{ConfigPath: `C:\work\tadx.yaml`}); err != nil {
		t.Fatal(err)
	}
	text := buffer.String()
	if !strings.Contains(text, "--config") || !strings.Contains(text, "tadx cache status") {
		t.Fatalf("bound hints missing: %s", text)
	}
	if strings.Contains(text, "resource: tadx --config") {
		t.Fatalf("resource field was rewritten: %s", text)
	}
}

func TestConfigPathLeavesUserMapKeysUntouched(t *testing.T) {
	var buffer bytes.Buffer
	value := map[string]any{"help": "tadx cache status", "resource": "tadx cache status"}
	if err := output.RenderWithOptions(&buffer, value, output.Options{ConfigPath: `C:\work\tadx.yaml`}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buffer.String(), "--config") {
		t.Fatalf("user map was rewritten: %s", buffer.String())
	}
}

func TestConfigPathPreservesTypedTOONFieldOrder(t *testing.T) {
	value := struct {
		Status           string   `json:"status"`
		CorrectiveAction string   `json:"corrective_action"`
		Help             []string `json:"help"`
	}{"failed", "Run tadx auth status, then retry.", []string{"Run tadx cache status --full."}}
	var plain, bound bytes.Buffer
	if err := output.RenderWithOptions(&plain, value, output.Options{}); err != nil {
		t.Fatal(err)
	}
	if err := output.RenderWithOptions(&bound, value, output.Options{ConfigPath: `C:\work\tadx.yaml`}); err != nil {
		t.Fatal(err)
	}
	keys := func(text string) []string {
		lines := strings.Split(strings.TrimSpace(text), "\n")
		result := make([]string, len(lines))
		for index, line := range lines {
			result[index] = strings.SplitN(line, ":", 2)[0]
		}
		return result
	}
	if !reflect.DeepEqual(keys(plain.String()), keys(bound.String())) {
		t.Fatalf("field order changed\nplain=%q\nbound=%q", plain.String(), bound.String())
	}
}

func TestConfigPathCycleFailsThroughSerialization(t *testing.T) {
	type node struct {
		Next *node    `json:"next,omitempty"`
		Help []string `json:"help,omitempty"`
	}
	value := &node{Help: []string{"Run tadx cache status."}}
	value.Next = value
	var buffer bytes.Buffer
	if err := output.RenderWithOptions(&buffer, value, output.Options{ConfigPath: `C:\work\tadx.yaml`}); err == nil {
		t.Fatal("cyclic value unexpectedly rendered")
	}
}

func TestConfigPathDoesNotMutateInputHints(t *testing.T) {
	value := struct {
		Help []string `json:"help"`
	}{[]string{"Run tadx cache status."}}
	var buffer bytes.Buffer
	if err := output.RenderWithOptions(&buffer, &value, output.Options{ConfigPath: `C:\one\tadx.yaml`}); err != nil {
		t.Fatal(err)
	}
	if value.Help[0] != "Run tadx cache status." {
		t.Fatalf("input help mutated: %#v", value.Help)
	}
	buffer.Reset()
	if err := output.RenderWithOptions(&buffer, &value, output.Options{ConfigPath: `C:\two\tadx.yaml`}); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(buffer.String(), "one\\\\tadx") || !strings.Contains(buffer.String(), "two") {
		t.Fatalf("second config binding incorrect: %s", buffer.String())
	}
}

func TestConfigPathBindsSharedPointerEachOccurrence(t *testing.T) {
	type hint struct {
		Help []string `json:"help"`
	}
	shared := &hint{Help: []string{"Run tadx cache status."}}
	value := []*hint{shared, shared}
	var buffer bytes.Buffer
	if err := output.RenderWithOptions(&buffer, value, output.Options{ConfigPath: `C:\work\tadx.yaml`}); err != nil {
		t.Fatal(err)
	}
	if strings.Count(buffer.String(), "--config") != 2 {
		t.Fatalf("shared pointers were not independently bound: %s", buffer.String())
	}
}
