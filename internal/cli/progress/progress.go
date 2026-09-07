// Package progress reports elapsed time for long-running CLI operations.
package progress

import (
	"context"
	"fmt"
	"io"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/mattn/go-isatty"
)

const defaultInterval = 5 * time.Second

// Ticker supplies periodic clock events.
type Ticker interface {
	C() <-chan time.Time
	Stop()
}

// Clock supplies the current time and creates tickers.
// Implementations must support concurrent calls.
type Clock interface {
	Now() time.Time
	NewTicker(time.Duration) Ticker
}

// Option configures a Reporter.
type Option func(*Reporter)

// WithInterval sets the time between progress heartbeats.
// New ignores nonpositive intervals.
func WithInterval(interval time.Duration) Option {
	return func(reporter *Reporter) {
		if interval > 0 {
			reporter.interval = interval
		}
	}
}

// WithTerminalDetector replaces interactive terminal detection.
// This option supports deterministic command tests.
func WithTerminalDetector(detector func(io.Writer) bool) Option {
	return func(reporter *Reporter) {
		if detector != nil {
			reporter.isTerminal = detector
		}
	}
}

// WithClock replaces the wall clock and ticker source.
// This option supports deterministic timing tests.
func WithClock(clock Clock) Option {
	return func(reporter *Reporter) {
		if clock != nil {
			reporter.clock = clock
		}
	}
}

// Reporter writes transient progress to one standard error stream.
// Use one Reporter for one active command-level operation.
type Reporter struct {
	stderr     io.Writer
	interval   time.Duration
	isTerminal func(io.Writer) bool
	clock      Clock
	writeMu    sync.Mutex
}

// New creates a long-operation progress reporter.
// The reporter stays silent unless stderr is an interactive terminal.
func New(stderr io.Writer, options ...Option) *Reporter {
	reporter := &Reporter{
		stderr:     stderr,
		interval:   defaultInterval,
		isTerminal: InteractiveTerminal,
		clock:      wallClock{},
	}
	for _, option := range options {
		if option != nil {
			option(reporter)
		}
	}
	return reporter
}

// Start reports an immediate activity line and periodic elapsed-time heartbeats.
// Call Stop when the operation returns. Context cancellation also stops reporting.
func (reporter *Reporter) Start(ctx context.Context, label string) *Operation {
	if reporter == nil {
		return inactiveOperation()
	}
	ctx = nonNilContext(ctx)
	if ctx.Err() != nil {
		return inactiveOperation()
	}
	if reporter.stderr == nil || !reporter.isTerminal(reporter.stderr) {
		return inactiveOperation()
	}

	operation := &Operation{
		reporter: reporter,
		label:    normalizeLabel(label),
		started:  reporter.clock.Now(),
		done:     make(chan struct{}),
		stopped:  make(chan struct{}),
	}
	if !operation.report() {
		operation.deactivate()
		return operation
	}

	operation.ticker = reporter.clock.NewTicker(reporter.interval)
	go operation.heartbeat(ctx)
	return operation
}

// Do wraps an error-only operation with progress reporting.
func (reporter *Reporter) Do(ctx context.Context, label string, action func(context.Context) error) error {
	ctx = nonNilContext(ctx)
	operation := reporter.Start(ctx, label)
	defer operation.Stop()
	return action(ctx)
}

// Run wraps a value-returning operation with progress reporting.
// It returns the action result and error without modification.
func Run[T any](ctx context.Context, reporter *Reporter, label string, action func(context.Context) (T, error)) (T, error) {
	ctx = nonNilContext(ctx)
	if reporter == nil {
		return action(ctx)
	}
	operation := reporter.Start(ctx, label)
	defer operation.Stop()
	return action(ctx)
}

// Operation owns one progress heartbeat.
type Operation struct {
	reporter *Reporter
	label    string
	started  time.Time
	ticker   Ticker
	done     chan struct{}
	stopped  chan struct{}
	once     sync.Once

	inactive bool
	finished bool
	maxWidth int
}

// Stop clears the activity line and waits for the heartbeat goroutine to exit.
// Stop is safe to call more than one time.
func (operation *Operation) Stop() {
	if operation == nil || operation.inactive {
		return
	}
	operation.finish(true)
	<-operation.stopped
}

func (operation *Operation) heartbeat(ctx context.Context) {
	defer close(operation.stopped)
	for {
		select {
		case <-operation.done:
			return
		case <-ctx.Done():
			operation.finish(true)
			return
		case <-operation.ticker.C():
			if !operation.report() {
				operation.finish(false)
				return
			}
		}
	}
}

func (operation *Operation) report() bool {
	elapsed := operation.reporter.clock.Now().Sub(operation.started)
	if elapsed < 0 {
		elapsed = 0
	}
	elapsed = elapsed.Truncate(time.Second)
	line := fmt.Sprintf("%s... elapsed %s", operation.label, elapsed)

	operation.reporter.writeMu.Lock()
	defer operation.reporter.writeMu.Unlock()
	if operation.finished {
		return true
	}
	written, err := fmt.Fprintf(operation.reporter.stderr, "\r%s", line)
	if width := len(line); width > operation.maxWidth {
		operation.maxWidth = width
	}
	return err == nil && written == len(line)+1
}

func (operation *Operation) finish(clear bool) {
	operation.once.Do(func() {
		if operation.ticker != nil {
			operation.ticker.Stop()
		}
		close(operation.done)

		operation.reporter.writeMu.Lock()
		operation.finished = true
		if clear && operation.maxWidth > 0 {
			_, _ = fmt.Fprintf(operation.reporter.stderr, "\r%s\r", strings.Repeat(" ", operation.maxWidth))
		}
		operation.reporter.writeMu.Unlock()
	})
}

func (operation *Operation) deactivate() {
	operation.reporter.writeMu.Lock()
	operation.finished = true
	operation.reporter.writeMu.Unlock()
	close(operation.done)
	close(operation.stopped)
	operation.inactive = true
}

func inactiveOperation() *Operation {
	return &Operation{inactive: true}
}

func nonNilContext(ctx context.Context) context.Context {
	if ctx == nil {
		return context.Background()
	}
	return ctx
}

func normalizeLabel(label string) string {
	label = strings.Map(func(character rune) rune {
		if unicode.IsControl(character) {
			return ' '
		}
		return character
	}, label)
	label = strings.Join(strings.Fields(label), " ")
	if label == "" {
		return "Operation in progress"
	}
	return label
}

// InteractiveTerminal reports whether writer is a native or MSYS/Cygwin terminal.
func InteractiveTerminal(writer io.Writer) bool {
	fdWriter, ok := writer.(interface{ Fd() uintptr })
	if !ok {
		return false
	}
	fd := fdWriter.Fd()
	return isatty.IsTerminal(fd) || isatty.IsCygwinTerminal(fd)
}

type wallClock struct{}

func (wallClock) Now() time.Time { return time.Now() }
func (wallClock) NewTicker(interval time.Duration) Ticker {
	return realTicker{Ticker: time.NewTicker(interval)}
}

type realTicker struct{ *time.Ticker }

func (ticker realTicker) C() <-chan time.Time { return ticker.Ticker.C }
