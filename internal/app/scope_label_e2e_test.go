package app_test

import (
	"bytes"
	"context"
	"github.com/ahillspace/tadx/internal/app"
	"path/filepath"
	"strings"
	"testing"
)

func TestCapabilitiesSeparateOutOfScope(t *testing.T) {
	var out bytes.Buffer
	code := app.Run(context.Background(), []string{"capability", "list", "--limit", "10000"}, &out, app.Options{ConfigPath: filepath.Join(t.TempDir(), "config.yaml")})
	if code != 0 || strings.Contains(out.String(), "delegated") || !strings.Contains(out.String(), "Out of scope") || !strings.Contains(out.String(), "out_of_scope") {
		t.Fatal(code, out.String())
	}
}
