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
