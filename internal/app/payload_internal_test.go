package app

import (
	"os"
	"path/filepath"
	"testing"
)

// PayloadFile writes content as a downloaded payload, the way a pull hands
// one to the artifact layer, and returns its path.
func PayloadFile(t testing.TB, content []byte) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "payload")
	if err := os.WriteFile(path, content, 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}
