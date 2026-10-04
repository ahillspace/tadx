package lastcommand

import (
	"context"
	"encoding/json"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

// RecorderPorts binds storage and presentation without importing command plumbing.
type RecorderPorts struct {
	Write                func(context.Context, value.SavedExecution) error
	Snapshot             func(any, int) (json.RawMessage, error)
	Now                  func() time.Time
	RequiredCapabilities func() []string
}

// Recorder saves bounded evidence without redefining the operation outcome.
type Recorder struct{ ports RecorderPorts }

func NewRecorder(ports RecorderPorts) *Recorder {
	if ports.Now == nil {
		ports.Now = time.Now
	}
	return &Recorder{ports: ports}
}

// Save reports whether the full snapshot was saved, independently of its exit code.
func (r *Recorder) Save(operation string, result any, exitCode int) (bool, error) {
	if result == nil {
		return false, nil
	}
	data, snapshotErr := r.ports.Snapshot(result, MaxBytes/2)
	record := value.SavedExecution{RecordedAt: r.ports.Now().UTC(), Operation: operation, ExitCode: exitCode, Result: data}
	if r.ports.RequiredCapabilities != nil {
		record.RequiredCapabilities = r.ports.RequiredCapabilities()
	}
	if record.Operation == "" {
		record.Operation = "cli"
	}
	if snapshotErr != nil {
		record.Result = nil
		record.Unavailable = "The previous result could not be saved within the expanded-output bound."
	}
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := r.ports.Write(ctx, record); err != nil {
		return false, err
	}
	return snapshotErr == nil, snapshotErr
}
