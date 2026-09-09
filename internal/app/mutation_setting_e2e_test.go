package app_test

import (
	"bytes"
	"context"
	"github.com/ahillspace/tadx/internal/app"
	"path/filepath"
	"strings"
	"testing"
)

func TestPersistentMutationSettingThroughCLI(t *testing.T) {
	path := filepath.Join(t.TempDir(), "config.yaml")
	invoke := func(args ...string) string {
		t.Helper()
		var out bytes.Buffer
		if code := app.Run(context.Background(), args, &out, app.Options{ConfigPath: path}); code != 0 {
			t.Fatalf("code=%d %s", code, out.String())
		}
		return out.String()
	}
	if got := invoke("mutation", "status"); !strings.Contains(got, "enabled: false") {
		t.Fatal(got)
	}
	if got := invoke("mutation", "set", "--enabled=true"); !strings.Contains(got, "enabled: true") {
		t.Fatal(got)
	}
	if got := invoke("mutation", "status"); !strings.Contains(got, "saved_user_setting") {
		t.Fatal(got)
	}
	var overridden bytes.Buffer
	if code := app.Run(context.Background(), []string{"mutation", "status"}, &overridden, app.Options{ConfigPath: path, MutationEnvironment: func() (string, bool) { return "0", true }}); code != 0 || !strings.Contains(overridden.String(), "enabled: false") || !strings.Contains(overridden.String(), "process_environment") {
		t.Fatal(overridden.String())
	}
	invoke("mutation", "set", "--enabled=false")
	if got := invoke("mutation", "status"); !strings.Contains(got, "enabled: false") {
		t.Fatal(got)
	}
}
