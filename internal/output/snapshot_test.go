package output

import (
	"errors"
	"github.com/ahillspace/tadx/internal/cli/clierr"
	"strings"
	"testing"
)

func TestSnapshotRedactsAndPreservesPartialResults(t *testing.T) {
	input := map[string]any{"luid": "created-1", "credentials": map[string]string{"token": "do-not-save"}, "nested": []any{map[string]string{"pat_secret": "secret-value"}}}
	got, err := Snapshot(clierr.WithOutput(input, errors.New("verification failed")), 4096)
	if err != nil {
		t.Fatal(err)
	}
	for _, bad := range []string{"do-not-save", "secret-value"} {
		if strings.Contains(string(got), bad) {
			t.Fatal("secret persisted")
		}
	}
	if !strings.Contains(string(got), "created-1") || !strings.Contains(string(got), "verification failed") {
		t.Fatal(string(got))
	}
	if _, err := Snapshot(map[string]string{"large": strings.Repeat("x", 4096)}, 100); err == nil {
		t.Fatal("snapshot size unbounded")
	}
}

func TestSnapshotWithConfigBindsHintsAtCaptureTime(t *testing.T) {
	value := struct {
		Help             []string `json:"help"`
		CorrectiveAction string   `json:"corrective_action"`
		Resource         string   `json:"resource"`
	}{[]string{"Run tadx catalog status --full."}, "Run tadx auth status, then retry.", "tadx catalog status"}
	got, err := SnapshotWithConfig(value, 4096, `C:\work\tadx.yaml`)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if strings.Count(text, "--config") != 2 {
		t.Fatalf("hint count = %d, output = %s", strings.Count(text, "--config"), text)
	}
	if strings.Contains(text, `"resource":"tadx --config`) {
		t.Fatalf("resource field was rebound: %s", text)
	}
}
