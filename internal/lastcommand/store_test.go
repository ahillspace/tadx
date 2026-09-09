package lastcommand

import (
	"context"
	"encoding/json"
	"github.com/ahillspace/tadx/internal/value"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestStoreReplacesOneResultAndMarksOversize(t *testing.T) {
	s := Store{filepath.Join(t.TempDir(), "last.json")}
	ctx := context.Background()
	for _, id := range []string{"first", "second"} {
		if err := s.Save(ctx, value.SavedExecution{Operation: id, RecordedAt: time.Now(), Result: json.RawMessage(`{"ok":true}`)}); err != nil {
			t.Fatal(err)
		}
	}
	got, err := s.Read(ctx)
	if err != nil || got.Operation != "second" {
		t.Fatal(got, err)
	}
	data, _ := json.Marshal(strings.Repeat("x", MaxBytes))
	if err := s.Save(ctx, value.SavedExecution{Operation: "large", RecordedAt: time.Now(), Result: data}); err != nil {
		t.Fatal(err)
	}
	got, err = s.Read(ctx)
	if err != nil || got.Operation != "large" || got.Unavailable == "" || len(got.Result) != 0 {
		t.Fatal(got, err)
	}
}
