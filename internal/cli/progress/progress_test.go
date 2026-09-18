package progress_test

import (
	"bytes"
	"context"
	"errors"
	"io"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/cli/progress"
)

func TestRunPreservesResultAndKeepsNonTerminalOutputSilent(t *testing.T) {
	var stderr bytes.Buffer
	reporter := progress.New(&stderr, progress.WithTerminalDetector(func(io.Writer) bool { return false }))

	got, err := progress.Run(context.Background(), reporter, "Publishing workbook", func(context.Context) (string, error) {
		return "deterministic TOON", nil
	})

	if err != nil {
		t.Fatal(err)
	}
	if got != "deterministic TOON" {
		t.Fatalf("result = %q, want deterministic TOON", got)
	}
	if stderr.Len() != 0 {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
}

func TestObserverReceivesLabelsWhenTerminalProgressIsDisabled(t *testing.T) {
	var mu sync.Mutex
	var labels []string
	ctx := progress.WithObserver(context.Background(), func(label string) {
		mu.Lock()
		defer mu.Unlock()
		labels = append(labels, label)
	})
	reporter := progress.New(io.Discard, progress.WithTerminalDetector(func(io.Writer) bool { return false }))
	operation := reporter.Start(ctx, "Preparing workbook publication")
	ctx = progress.WithOperation(ctx, operation)
	if !progress.SetLabel(ctx, "Selecting workbook source") {
		t.Fatal("SetLabel did not notify the observer")
	}
	operation.Stop()

	mu.Lock()
	defer mu.Unlock()
	want := []string{"Preparing workbook publication", "Selecting workbook source"}
	if len(labels) != len(want) {
		t.Fatalf("labels = %#v, want %#v", labels, want)
	}
	for i := range want {
		if labels[i] != want[i] {
			t.Fatalf("labels = %#v, want %#v", labels, want)
		}
	}
}

func TestStartWritesImmediateHonestActivityToInteractiveStderr(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC))
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)

	operation := reporter.Start(context.Background(), "Publishing workbook")
	waitForWrite(t, stderr)
	operation.Stop()

	got := stderr.String()
	if !strings.Contains(got, "Publishing workbook... elapsed 0s") {
		t.Fatalf("stderr = %q, want initial activity and elapsed time", got)
	}
	if strings.Contains(got, "%") {
		t.Fatalf("stderr = %q, want no fabricated percentage", got)
	}
}

func TestStartEmitsPeriodicElapsedHeartbeats(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC))
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
		progress.WithInterval(2*time.Second),
	)
	operation := reporter.Start(context.Background(), "Uploading datasource")
	waitForWrite(t, stderr)

	clock.Advance(2 * time.Second)
	waitForWrite(t, stderr)
	clock.Advance(63 * time.Second)
	waitForWrite(t, stderr)
	operation.Stop()

	got := stderr.String()
	for _, want := range []string{
		"Uploading datasource... elapsed 0s",
		"Uploading datasource... elapsed 2s",
		"Uploading datasource... elapsed 1m5s",
	} {
		if !strings.Contains(got, want) {
			t.Fatalf("stderr = %q, want %q", got, want)
		}
	}
	if clock.Interval() != 2*time.Second {
		t.Fatalf("ticker interval = %s, want 2s", clock.Interval())
	}
}

func TestStartUsesOneSecondDefaultHeartbeat(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC))
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	operation := reporter.Start(context.Background(), "Publishing workbook")
	waitForWrite(t, stderr)
	defer operation.Stop()

	clock.Advance(time.Second)
	waitForWrite(t, stderr)
	if clock.Interval() != time.Second {
		t.Fatalf("ticker interval = %s, want 1s", clock.Interval())
	}
}

func TestSetLabelUpdatesTheSharedOperationFromContext(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC))
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	operation := reporter.Start(context.Background(), "Publishing workbook")
	waitForWrite(t, stderr)
	defer operation.Stop()

	ctx := progress.WithOperation(context.Background(), operation)
	if !progress.SetLabel(ctx, "Waiting for workbook acceptance") {
		t.Fatal("SetLabel reported that the operation was unavailable")
	}
	waitForWrite(t, stderr)
	if !strings.Contains(stderr.String(), "Waiting for workbook acceptance... elapsed 0s") {
		t.Fatalf("stderr = %q, want updated phase label", stderr.String())
	}
}

func TestSetLabelClearsTrailingCharactersFromThePreviousPhase(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC))
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	operation := reporter.Start(context.Background(), "Waiting for Tableau publication")
	waitForWrite(t, stderr)
	defer operation.Stop()

	if !operation.SetLabel("Publishing") {
		t.Fatal("SetLabel reported that the operation was unavailable")
	}
	waitForWrite(t, stderr)
	previous := "Waiting for Tableau publication... elapsed 0s"
	current := "Publishing... elapsed 0s"
	want := "\r" + current + strings.Repeat(" ", len(previous)-len(current))
	if got := stderr.String(); !strings.Contains(got, want) {
		t.Fatalf("stderr = %q, want shorter phase padded to clear prior text with %q", got, want)
	}
}

func TestHeartbeatAndLabelChangesAreSerialized(t *testing.T) {
	var stderr bytes.Buffer
	reporter := progress.New(&stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithInterval(time.Millisecond),
	)
	operation := reporter.Start(context.Background(), "Preparing publication")
	defer operation.Stop()

	for index := range 200 {
		label := "Selecting source A"
		if index%2 == 0 {
			label = "Selecting source B"
		}
		if !operation.SetLabel(label) {
			t.Fatal("SetLabel reported that the operation was unavailable")
		}
		time.Sleep(100 * time.Microsecond)
	}
}

func TestSetLabelDoesNotRepaintAnUnchangedLabel(t *testing.T) {
	clock := newFakeClock(time.Date(2026, time.September, 6, 12, 0, 0, 0, time.UTC))
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	operation := reporter.Start(context.Background(), "Publishing workbook")
	waitForWrite(t, stderr)
	defer operation.Stop()

	before := stderr.String()
	if !progress.SetLabel(progress.WithOperation(context.Background(), operation), "Publishing workbook") {
		t.Fatal("SetLabel reported that the operation was unavailable")
	}
	if got := stderr.String(); got != before {
		t.Fatalf("unchanged label repainted stderr from %q to %q", before, got)
	}
}

func TestSetLabelRejectsAnOperationAfterStop(t *testing.T) {
	clock := newFakeClock(time.Now())
	reporter := progress.New(io.Discard,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	operation := reporter.Start(context.Background(), "Publishing workbook")
	operation.Stop()
	if progress.SetLabel(progress.WithOperation(context.Background(), operation), "done") {
		t.Fatal("SetLabel succeeded after Stop")
	}
}

func TestStopStopsTickerAndClearsInteractiveLine(t *testing.T) {
	clock := newFakeClock(time.Now())
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	operation := reporter.Start(context.Background(), "Publishing flow")
	waitForWrite(t, stderr)

	operation.Stop()
	waitForTickerStop(t, clock)
	got := stderr.String()
	clearedLine := "\r" + strings.Repeat(" ", len("Publishing flow... elapsed 0s")) + "\r"
	if !strings.HasSuffix(got, clearedLine) {
		t.Fatalf("stderr = %q, want cleared terminal line", got)
	}

	// Stop is idempotent and waits for the heartbeat goroutine to exit.
	operation.Stop()
	if gotAfterSecondStop := stderr.String(); gotAfterSecondStop != got {
		t.Fatalf("second Stop changed stderr from %q to %q", got, gotAfterSecondStop)
	}
}

func TestCancellationStopsAndClearsProgressWithoutCallerStop(t *testing.T) {
	clock := newFakeClock(time.Now())
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	ctx, cancel := context.WithCancel(context.Background())
	operation := reporter.Start(ctx, "Publishing workbook")
	waitForWrite(t, stderr)

	cancel()
	waitForTickerStop(t, clock)
	operation.Stop()
	if !strings.HasSuffix(stderr.String(), "\r") {
		t.Fatalf("stderr = %q, want cleared line after cancellation", stderr.String())
	}
}

func TestRunReturnsOriginalErrorAndStopsProgress(t *testing.T) {
	clock := newFakeClock(time.Now())
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	wantErr := errors.New("Tableau rejected upload")

	_, gotErr := progress.Run(context.Background(), reporter, "Publishing workbook", func(context.Context) (struct{}, error) {
		return struct{}{}, wantErr
	})

	if !errors.Is(gotErr, wantErr) {
		t.Fatalf("error = %v, want %v", gotErr, wantErr)
	}
	waitForTickerStop(t, clock)
}

func TestAlreadyCanceledContextDoesNotStartProgress(t *testing.T) {
	clock := newFakeClock(time.Now())
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	operation := reporter.Start(ctx, "Publishing workbook")
	operation.Stop()

	if stderr.String() != "" {
		t.Fatalf("stderr = %q, want empty", stderr.String())
	}
	if clock.TickerCreated() {
		t.Fatal("ticker created for an already canceled context")
	}
}

func TestActivityLabelCannotInjectTerminalControlLines(t *testing.T) {
	clock := newFakeClock(time.Now())
	stderr := newRecordingWriter()
	reporter := progress.New(stderr,
		progress.WithTerminalDetector(func(io.Writer) bool { return true }),
		progress.WithClock(clock),
	)
	operation := reporter.Start(context.Background(), "Publishing\nworkbook\x1b[2J")
	waitForWrite(t, stderr)
	operation.Stop()

	got := stderr.String()
	if strings.Contains(got, "\n") || strings.Contains(got, "\x1b") {
		t.Fatalf("stderr = %q, want terminal controls removed from label", got)
	}
	if !strings.Contains(got, "Publishing workbook [2J... elapsed 0s") {
		t.Fatalf("stderr = %q, want normalized label", got)
	}
}

func TestInteractiveTerminalRejectsOrdinaryWriters(t *testing.T) {
	if progress.InteractiveTerminal(io.Discard) {
		t.Fatal("io.Discard detected as an interactive terminal")
	}
	if progress.InteractiveTerminal(&bytes.Buffer{}) {
		t.Fatal("bytes.Buffer detected as an interactive terminal")
	}
}

type fakeClock struct {
	mu       sync.Mutex
	now      time.Time
	ticker   *fakeTicker
	interval time.Duration
}

func newFakeClock(now time.Time) *fakeClock {
	return &fakeClock{now: now}
}

func (c *fakeClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *fakeClock) NewTicker(interval time.Duration) progress.Ticker {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.interval = interval
	c.ticker = &fakeTicker{ticks: make(chan time.Time, 8), stopped: make(chan struct{})}
	return c.ticker
}

func (c *fakeClock) Advance(elapsed time.Duration) {
	c.mu.Lock()
	c.now = c.now.Add(elapsed)
	now := c.now
	ticker := c.ticker
	c.mu.Unlock()
	if ticker == nil {
		panic("Advance called before NewTicker")
	}
	ticker.ticks <- now
}

func (c *fakeClock) Interval() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.interval
}

func (c *fakeClock) TickerCreated() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.ticker != nil
}

type fakeTicker struct {
	ticks   chan time.Time
	stopped chan struct{}
	once    sync.Once
}

func (t *fakeTicker) C() <-chan time.Time { return t.ticks }
func (t *fakeTicker) Stop()               { t.once.Do(func() { close(t.stopped) }) }

type recordingWriter struct {
	mu     sync.Mutex
	buffer bytes.Buffer
	writes chan struct{}
}

func newRecordingWriter() *recordingWriter {
	return &recordingWriter{writes: make(chan struct{}, 16)}
}

func (w *recordingWriter) Write(p []byte) (int, error) {
	w.mu.Lock()
	n, err := w.buffer.Write(p)
	w.mu.Unlock()
	w.writes <- struct{}{}
	return n, err
}

func (w *recordingWriter) String() string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return w.buffer.String()
}

func waitForWrite(t *testing.T, writer *recordingWriter) {
	t.Helper()
	select {
	case <-writer.writes:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for progress write")
	}
}

func waitForTickerStop(t *testing.T, clock *fakeClock) {
	t.Helper()
	clock.mu.Lock()
	ticker := clock.ticker
	clock.mu.Unlock()
	if ticker == nil {
		t.Fatal("ticker was not created")
	}
	select {
	case <-ticker.stopped:
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for ticker stop")
	}
}
