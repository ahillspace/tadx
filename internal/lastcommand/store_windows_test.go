package lastcommand

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/value"
)

// A parallel command can read the saved result while another command saves.
func TestSaveReplacesResultAfterTransientReaderCloses(t *testing.T) {
	s := Store{filepath.Join(t.TempDir(), "last.json")}
	ctx := context.Background()
	if err := s.Save(ctx, value.SavedExecution{Operation: "first", RecordedAt: time.Now(), Result: json.RawMessage(`{}`)}); err != nil {
		t.Fatal(err)
	}
	reader, err := os.Open(s.Path)
	if err != nil {
		t.Fatal(err)
	}
	defer reader.Close()
	done := make(chan struct{})
	go func() {
		time.Sleep(200 * time.Millisecond)
		_ = reader.Close()
		close(done)
	}()
	defer func() { <-done }()

	if err := s.Save(ctx, value.SavedExecution{Operation: "second", RecordedAt: time.Now(), Result: json.RawMessage(`{}`)}); err != nil {
		t.Fatalf("Save() while a reader held the result error = %v", err)
	}
	got, err := s.Read(ctx)
	if err != nil || got.Operation != "second" {
		t.Fatalf("Read() = %#v, %v", got, err)
	}
}
