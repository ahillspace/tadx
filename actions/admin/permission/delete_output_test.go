package permission_test

import (
	"bytes"
	"context"
	"os"
	"strings"
	"testing"

	action "github.com/ahillspace/tadx/actions/admin/permission"
	"github.com/ahillspace/tadx/internal/output"
)

func TestDeleteOutputFixtures(t *testing.T) {
	f := &deleteFake{source: "direct", mode: "Allow", status: "deleted"}
	out, err := action.NewDelete(f, f).Execute(context.Background(), deleteInput(), false)
	if err != nil {
		t.Fatal(err)
	}
	for _, full := range []bool{false, true} {
		name := "compact"
		if full {
			name = "full"
		}
		expected, err := os.ReadFile("delete/testdata/" + name + ".toon")
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
