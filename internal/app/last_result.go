package app

import (
	"context"
	"encoding/json"
	"path/filepath"

	"github.com/ahillspace/tadx/internal/lastcommand"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/value"
)

// lastCapture holds process rendering state; lastcommand owns persistence.
type lastCapture struct {
	store       func() lastcommand.Store
	recorder    *lastcommand.Recorder
	operation   string
	value       any
	enabled     bool
	saved       bool
	renderError bool
	hintConfig  func() string
}

func newLastCapture(r *runtimeDependencies) *lastCapture {
	capture := &lastCapture{store: func() lastcommand.Store {
		return lastcommand.Store{Path: filepath.Join(filepath.Dir(r.configPath), "last-result.json")}
	}, enabled: true}
	capture.recorder = lastcommand.NewRecorder(lastcommand.RecorderPorts{
		Write: func(ctx context.Context, record value.SavedExecution) error {
			return capture.store().Save(ctx, record)
		},
		Snapshot: func(result any, limit int) (json.RawMessage, error) {
			configPath := ""
			if capture.hintConfig != nil {
				configPath = capture.hintConfig()
			}
			return output.SnapshotWithConfig(result, limit, configPath)
		},
		Now: r.now, RequiredCapabilities: r.managedChecks.snapshot,
	})
	return capture
}
