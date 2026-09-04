package pull_test

import (
	"bytes"
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	definitionpull "github.com/ahillspace/tadx/actions/pulse/definition/pull"
	"github.com/ahillspace/tadx/internal/errs"
	render "github.com/ahillspace/tadx/internal/output"
)

func TestOutputGolden(t *testing.T) {
	output := definitionpull.Output{
		Status: "pulled", Definition: definitionpull.Definition{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1"},
		Artifact:  definitionpull.ArtifactResult{Path: "artifacts/pulse-definition/Revenue--identity", CanonicalPath: "artifacts/pulse-definition/Revenue--identity/resource.json", BaselineFingerprint: "sha256:value"},
		RequestID: "request-1", Help: []string{"Inspect artifacts/pulse-definition/Revenue--identity/resource.json."},
	}
	assertGolden(t, "compact.toon", output, false)
	assertGolden(t, "full.toon", output, true)
}

func assertGolden(t *testing.T, name string, value any, full bool) {
	t.Helper()
	var buffer bytes.Buffer
	if err := render.RenderWithOptions(&buffer, value, render.Options{Full: full}); err != nil {
		t.Fatal(err)
	}
	want, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
		t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", name, want, buffer.Bytes())
	}
}

type reader struct{ definition definitionpull.Definition }

func (r reader) GetDefinition(context.Context, string) (definitionpull.Definition, error) {
	return r.definition, nil
}

type writer struct {
	input  definitionpull.Artifact
	result definitionpull.ArtifactResult
	err    error
}

func (w *writer) WriteDefinition(_ context.Context, input definitionpull.Artifact) (definitionpull.ArtifactResult, error) {
	w.input = input
	if w.result.Path == "" {
		w.result = definitionpull.ArtifactResult{Path: `artifacts\pulse-definition\Revenue--identity`, CanonicalPath: `artifacts\pulse-definition\Revenue--identity\resource.json`, BaselineFingerprint: "sha256:value"}
	}
	return w.result, w.err
}

func TestPullWritesCanonicalDefinitionArtifact(t *testing.T) {
	definition := definitionpull.Definition{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1", Configuration: []byte(`{"name":"Revenue"}`), RequestID: "request-1"}
	w := &writer{}
	output, err := definitionpull.New(reader{definition: definition}, w).Execute(context.Background(), definitionpull.Input{
		Environment: "dev", Site: "sales", ServerOrigin: "https://example.test", SiteLUID: "site-1", Workspace: "workspace", LUID: "definition-1",
	})
	if err != nil {
		t.Fatal(err)
	}
	if w.input.DefinitionLUID != "definition-1" || string(w.input.Configuration) != `{"name":"Revenue"}` || output.Artifact.Path != "artifacts/pulse-definition/Revenue--identity" {
		t.Fatalf("input=%#v output=%#v", w.input, output)
	}
}

func TestPullRejectsAbsoluteWriterPath(t *testing.T) {
	w := &writer{result: definitionpull.ArtifactResult{Path: `C:\outside`}}
	definition := definitionpull.Definition{LUID: "definition-1", Name: "Revenue", DatasourceLUID: "datasource-1", Configuration: []byte(`{"metadata":{"id":"definition-1"}}`)}
	_, err := definitionpull.New(reader{definition: definition}, w).Execute(context.Background(), definitionpull.Input{Workspace: "workspace", LUID: "definition-1"})
	var structured *errs.Error
	if !errors.As(err, &structured) || structured.Kind != errs.KindOperation || structured.ID != "pulse.definition.pull.normalize" {
		t.Fatalf("error=%#v", err)
	}
}
