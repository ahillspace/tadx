package lastcommand

import (
	"context"
	"encoding/json"
	"errors"
	"reflect"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

func TestRecorderPreservesExitOutcomeAndCapabilityEvidence(t *testing.T) {
	var sequence []string
	var record value.SavedExecution
	var writeContext context.Context
	now := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("fixture", 3600))
	result := struct{ Status string }{"partial"}
	recorder := NewRecorder(RecorderPorts{
		Snapshot: func(got any, limit int) (json.RawMessage, error) {
			sequence = append(sequence, "snapshot")
			if got != result || limit != MaxBytes/2 {
				t.Fatalf("snapshot input=%v limit=%d", got, limit)
			}
			return json.RawMessage(`{"status":"partial"}`), nil
		},
		Now:                  func() time.Time { sequence = append(sequence, "clock"); return now },
		RequiredCapabilities: func() []string { sequence = append(sequence, "capabilities"); return []string{"workbook.inspect"} },
		Write: func(ctx context.Context, got value.SavedExecution) error {
			sequence = append(sequence, "write")
			record, writeContext = got, ctx
			if deadline, ok := ctx.Deadline(); !ok || time.Until(deadline) > 2*time.Second {
				t.Fatal("saved-result write has no bounded deadline")
			}
			return nil
		},
	})
	saved, err := recorder.Save("workbook.publish", result, 1)
	if err != nil || !saved || record.ExitCode != 1 || record.Operation != "workbook.publish" || record.Unavailable != "" || string(record.Result) != `{"status":"partial"}` {
		t.Fatalf("saved=%v record=%+v err=%v", saved, record, err)
	}
	if !record.RecordedAt.Equal(now) || record.RecordedAt.Location() != time.UTC || !reflect.DeepEqual(record.RequiredCapabilities, []string{"workbook.inspect"}) || !reflect.DeepEqual(sequence, []string{"snapshot", "clock", "capabilities", "write"}) {
		t.Fatalf("record=%+v sequence=%v", record, sequence)
	}
	select {
	case <-writeContext.Done():
	default:
		t.Fatal("write context was not released")
	}
}

func TestRecorderSnapshotFailureStoresUnavailableEvidence(t *testing.T) {
	want := errors.New("snapshot too large")
	var record value.SavedExecution
	recorder := NewRecorder(RecorderPorts{
		Snapshot: func(any, int) (json.RawMessage, error) { return json.RawMessage(`{"incomplete":true}`), want },
		Write:    func(_ context.Context, got value.SavedExecution) error { record = got; return nil },
	})
	saved, err := recorder.Save("", "result", 0)
	if saved || !errors.Is(err, want) || record.Operation != "cli" || record.ExitCode != 0 || record.Result != nil || record.Unavailable != "The previous result could not be saved within the expanded-output bound." {
		t.Fatalf("saved=%v record=%+v err=%v", saved, record, err)
	}
}

func TestRecorderWriteFailureTakesPrecedenceOverSnapshotFailure(t *testing.T) {
	writeErr := errors.New("cannot save")
	for _, snapshotErr := range []error{nil, errors.New("cannot snapshot")} {
		recorder := NewRecorder(RecorderPorts{
			Snapshot: func(any, int) (json.RawMessage, error) { return json.RawMessage(`{}`), snapshotErr },
			Write:    func(context.Context, value.SavedExecution) error { return writeErr },
		})
		if saved, err := recorder.Save("version.get", "result", 0); saved || !errors.Is(err, writeErr) {
			t.Fatalf("saved=%v err=%v", saved, err)
		}
	}
}

func TestRecorderSkipsAbsentResult(t *testing.T) {
	if saved, err := NewRecorder(RecorderPorts{}).Save("help", nil, 0); saved || err != nil {
		t.Fatalf("saved=%v err=%v", saved, err)
	}
}
