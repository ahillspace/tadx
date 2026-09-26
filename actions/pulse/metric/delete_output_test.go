package metric_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	pulsemetric "github.com/ahillspace/tadx/actions/pulse/metric"
	render "github.com/ahillspace/tadx/internal/output"
)

func TestDeleteOutputGolden(t *testing.T) {
	b := &deleteBackend{targets: []pulsemetric.DeleteMetric{deleteTarget(), deleteTarget()}}
	output, err := pulsemetric.Delete(context.Background(), b, b, deleteInput())
	if err != nil {
		t.Fatal(err)
	}
	for _, test := range []struct {
		name string
		full bool
	}{{"compact.toon", false}, {"full.toon", true}} {
		var buffer bytes.Buffer
		if err := render.RenderWithOptions(&buffer, output, render.Options{Full: test.full}); err != nil {
			t.Fatal(err)
		}
		want, err := os.ReadFile(filepath.Join("testdata", "delete", test.name))
		if err != nil {
			t.Fatal(err)
		}
		if !bytes.Equal(bytes.TrimSpace(buffer.Bytes()), bytes.TrimSpace(want)) {
			t.Fatalf("%s mismatch\nwant:\n%s\ngot:\n%s", test.name, want, buffer.Bytes())
		}
	}
}
