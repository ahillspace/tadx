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
	runtime     *runtimeDependencies
	store       lastcommand.Store
	now         func() time.Time
	operation   string
	value       any
	enabled     bool
	saved       bool
	renderError bool
	hintConfig  func() string
}

type savedResultWarning struct {
	Code                 string `json:"code"`
	Summary              string `json:"summary"`
	LastPotentiallyStale bool   `json:"last_potentially_stale"`
	CorrectiveAction     string `json:"corrective_action"`
}

func lastResultWarning() any {
	return struct {
		Warning savedResultWarning `json:"warning"`
	}{Warning: savedResultWarning{
		Code: "last_result_save_failed", Summary: "The current result could not be saved; the operation outcome is unchanged.",
		LastPotentiallyStale: true,
		CorrectiveAction:     "Retain this output. The previous last result can be stale; do not repeat a mutation to recover its receipt.",
	}}
}

func newLastCapture(r *runtimeDependencies) *lastCapture {
	return &lastCapture{runtime: r, store: lastcommand.Store{Path: filepath.Join(filepath.Dir(r.configPath), "last-result.json")}, now: r.now, enabled: true}
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
	if c.runtime != nil {
		record.RequiredCapabilities = c.runtime.managedChecks.snapshot()
	}
	if record.Operation == "" {
		record.Operation = "cli"
	}
	if err != nil {
		record.Result = nil
		record.Unavailable = "The previous result could not be saved within the expanded-output bound."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if saveErr := c.store.Save(ctx, record); saveErr != nil {
		return saveErr
	}
	c.saved = err == nil
	return err
}
