package app

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/toon"
)

func TestOutputEscapesDisplayControlsFromUserText(t *testing.T) {
	const crafted = "ops\u009b31m‮"
	config := filepath.Join(t.TempDir(), "config.yaml")
	for _, format := range []string{"toon", "json"} {
		args := []string{"--config", config, "workspace", "status", "--workspace", crafted}
		if format == "json" {
			args = append(args, "--json")
		}
		var out strings.Builder
		if code := Run(context.Background(), args, &out, Options{}); code == 0 {
			t.Fatalf("%s: workspace status for an unknown workspace succeeded:\n%s", format, out.String())
		}
		for _, r := range out.String() {
			if toon.IsDisplayControl(r) {
				t.Fatalf("%s output emits display control %U raw:\n%q", format, r, out.String())
			}
		}
		var decoded any
		if format == "json" {
			if err := json.Unmarshal([]byte(out.String()), &decoded); err != nil {
				t.Fatalf("JSON output does not decode: %v\n%s", err, out.String())
			}
		} else {
			var err error
			if decoded, err = toon.Decode([]byte(out.String())); err != nil {
				t.Fatalf("TOON output does not decode: %v\n%s", err, out.String())
			}
		}
		if encoded, _ := json.Marshal(decoded); !strings.Contains(string(encoded), crafted) {
			t.Fatalf("%s output lost the supplied name: %s", format, encoded)
		}
	}
}
