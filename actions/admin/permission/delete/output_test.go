package delete_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	action "github.com/ahillspace/tadx/actions/admin/permission/delete"
	"github.com/ahillspace/tadx/internal/output"
)

func TestOutputFixtures(t *testing.T) {
	f := &fake{source: "direct", mode: "Allow", status: "deleted"}
	out, err := action.New(f, f).Execute(context.Background(), input(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		name := "compact"
		if full {
			name = "full"
		}
		expected, err := os.ReadFile("testdata/" + name + ".toon")
		if err != nil {
			t.Fatal(err)
		}
		var rendered bytes.Buffer
		if err := output.RenderWithOptions(&rendered, out, output.Options{Full: full}); err != nil {
			t.Fatal(err)
		}
		if strings.TrimSpace(rendered.String()) != strings.TrimSpace(string(expected)) {
			t.Fatalf("%s output differs:\n%s", name, rendered.String())
		}
	}
}
