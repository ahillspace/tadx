package app_test

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/app"
)

func TestExplorationPublishAndSearchGuidance(t *testing.T) {
	for _, tc := range []struct {
		name string
		args []string
		want []string
	}{
		{"workbook dependencies", []string{"content", "workbook", "publish", "--help"}, []string{"Publish missing referenced datasources first", "does not publish dependencies or rebind"}},
		{"admin search scope", []string{"search", "--help"}, []string{"--type admin includes users and groups", "--type user or --type group"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var out bytes.Buffer
			code := app.Run(t.Context(), tc.args, &out, app.Options{ConfigPath: filepath.Join(t.TempDir(), "absent.yaml")})
			if code != 0 {
				t.Fatalf("help exit=%d: %s", code, &out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("help missing %q: %s", want, &out)
				}
			}
		})
	}
}

func TestExplorationDoctorWarningGrammar(t *testing.T) {
	for _, tc := range []struct {
		level string
		want  string
	}{
		{"debug", "0 warnings"},
		{"", "1 warning,"},
	} {
		t.Run(tc.want, func(t *testing.T) {
			t.Setenv("TADX_LOG_LEVEL", tc.level)
			var out bytes.Buffer
			code := app.Run(t.Context(), []string{"doctor", "--json"}, &out, app.Options{ConfigPath: filepath.Join(t.TempDir(), "absent.yaml")})
			if code != 1 || !strings.Contains(out.String(), tc.want) {
				t.Fatalf("doctor exit=%d, want %q: %s", code, tc.want, &out)
			}
		})
	}
}
