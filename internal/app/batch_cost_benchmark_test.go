package app

import (
	"bytes"
	"fmt"
	"path/filepath"
	"testing"

	"github.com/ahillspace/tadx/internal/config"
)

func BenchmarkWorkspaceUnregisterPreviewBatch(b *testing.B) {
	for _, count := range []int{1, 10, 100} {
		b.Run(fmt.Sprintf("items_%d", count), func(b *testing.B) {
			directory := b.TempDir()
			path := filepath.Join(directory, "config.yaml")
			cfg := config.Config{Version: config.CurrentVersion, Workspaces: map[string]config.WorkspaceRegistration{}}
			names := make([]string, 100)
			for i := range names {
				names[i] = fmt.Sprintf("workspace-%03d", i)
				cfg.Workspaces[names[i]] = config.WorkspaceRegistration{ID: fmt.Sprintf("ws_%032x", i+1), Path: filepath.Join(directory, names[i])}
			}
			if err := config.Save(path, cfg); err != nil {
				b.Fatal(err)
			}
			args := append([]string{"workspace", "unregister"}, names[:count]...)
			args = append(args, "--preview", "--json")
			opts := Options{ConfigPath: path}
			b.ReportAllocs()
			b.ResetTimer()
			for b.Loop() {
				var output bytes.Buffer
				if code := Run(b.Context(), args, &output, opts); code != 0 {
					b.Fatalf("code=%d output=%s", code, &output)
				}
			}
		})
	}
}
