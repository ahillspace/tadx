package app

import (
	"context"
	"github.com/ahillspace/tadx/internal/lastcommand"
	"github.com/ahillspace/tadx/internal/output"
	"github.com/ahillspace/tadx/internal/value"
	"path/filepath"
	"time"
)

type lastCapture struct {
	store      lastcommand.Store
	now        func() time.Time
	operation  string
	value      any
	enabled    bool
	hintConfig func() string
}

func newLastCapture(r *runtimeDependencies) *lastCapture {
	return &lastCapture{store: lastcommand.Store{Path: filepath.Join(filepath.Dir(r.configPath), "last-result.json")}, now: r.now, enabled: true}
}
func (c *lastCapture) save(code int) error {
	if !c.enabled || c.value == nil {
		return nil
	}
	configPath := ""
	if c.hintConfig != nil {
		configPath = c.hintConfig()
	}
	data, err := output.SnapshotWithConfig(c.value, lastcommand.MaxBytes/2, configPath)
	record := value.SavedExecution{RecordedAt: c.now().UTC(), Operation: c.operation, ExitCode: code, Result: data}
	if record.Operation == "" {
		record.Operation = "cli"
	}
	if err != nil {
		record.Result = nil
		record.Unavailable = "The previous result could not be saved within the expanded-output bound."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	return c.store.Save(ctx, record)
}
