package output_test

import (
	"strings"
	"testing"

	"github.com/ahillspace/tadx/internal/output"
)

func TestBoundWarningsUnderLimitPassthrough(t *testing.T) {
	t.Parallel()

	values := []string{"first", "second", "third"}
	got := output.BoundWarnings(values)
	if len(got) != len(values) {
		t.Fatalf("length = %d, want %d", len(got), len(values))
	}
	for index, value := range values {
		if got[index] != value {
			t.Fatalf("index %d = %q, want %q", index, got[index], value)
		}
	}
}

func TestBoundWarningsCapsCount(t *testing.T) {
	t.Parallel()

	values := make([]string, 25)
	for index := range values {
		values[index] = "warning"
	}
	got := output.BoundWarnings(values)
	if len(got) != 20 {
		t.Fatalf("length = %d, want 20", len(got))
	}
}

func TestBoundWarningsTruncatesLongEntry(t *testing.T) {
	t.Parallel()

	long := strings.Repeat("a", 600)
	got := output.BoundWarnings([]string{long})
	if len(got) != 1 {
		t.Fatalf("length = %d, want 1", len(got))
	}
	runes := []rune(got[0])
	if len(runes) != 512 {
		t.Fatalf("rune count = %d, want 512", len(runes))
	}
	if !strings.HasSuffix(got[0], "...") {
		t.Fatalf("entry = %q, want %q suffix", got[0], "...")
	}
	want := strings.Repeat("a", 512-3) + "..."
	if got[0] != want {
		t.Fatalf("entry = %q, want %q", got[0], want)
	}
}
