package definition

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/output"
)

func TestPublishOutputGolden(t *testing.T) {
	plan, err := preparePublish(publishInputFixture(), publishBundleFixture())
	if err != nil {
		t.Fatal(err)
	}
	result := PublishOutput{Status: "preview", Plan: plan, Mappings: []PublishMapping{}, Complete: true, Help: []string{"Publishing creates new objects; existing objects are never overwritten."}}
	for _, full := range []bool{false, true} {
		name := "compact.toon"
		if full {
			name = "full.toon"
		}
		var rendered bytes.Buffer
		if err := output.RenderWithOptions(&rendered, result, output.Options{Full: full}); err != nil {
			t.Fatal(err)
		}
		expected, err := os.ReadFile(filepath.Join("testdata", "publish", name))
		if err != nil || !bytes.Equal(bytes.TrimSpace(expected), bytes.TrimSpace(rendered.Bytes())) {
			t.Errorf("%s:\n%s", name, rendered.String())
		}
	}
}
