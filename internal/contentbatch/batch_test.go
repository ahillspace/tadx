package contentbatch_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/ahillspace/tadx/internal/contentbatch"
	"github.com/ahillspace/tadx/internal/errs"
	"github.com/ahillspace/tadx/internal/output"
)

type projected struct{ Value string }

func (p projected) CompactOutput() any {
	return struct {
		Summary string `json:"summary"`
	}{"compact " + p.Value}
}
func (p projected) FullOutput() any {
	return struct {
		Summary string `json:"summary"`
		Detail  string `json:"detail"`
	}{"compact " + p.Value, "expanded"}
}

func TestRunPreservesOrderedOutcomesAndProjectsNestedResults(t *testing.T) {
	var mu sync.Mutex
	var calls []string
	out, err := contentbatch.Run(context.Background(), "workbook.pull", []string{"first", "broken", "last"}, func(_ context.Context, selector string) (projected, error) {
		mu.Lock()
		calls = append(calls, selector)
		mu.Unlock()
		if selector == "broken" {
			return projected{}, &errs.Error{Kind: errs.KindOperation, Summary: "download failed", TableauRequestID: "request-2", Retryable: errs.Bool(false), Phase: errs.PhaseVerification, Outcome: errs.OutcomeUnknown, Prerequisite: &errs.Prerequisite{Kind: "workbook", Resource: "broken", Summary: "Inspect the authoritative workbook before retrying."}}
		}
		return projected{selector}, nil
	})
	if err == nil || errs.ExitCode(err) == 0 || out.Status != "partial_failure" || out.Total != 3 || out.Succeeded != 2 || out.Failed != 1 {
		t.Fatalf("outcome = %#v, err = %v", out, err)
	}
	slices.Sort(calls)
	if !reflect.DeepEqual(calls, []string{"broken", "first", "last"}) {
		t.Fatal(calls)
	}
	if got := []string{out.Items[0].Selector, out.Items[1].Selector, out.Items[2].Selector}; !reflect.DeepEqual(got, []string{"first", "broken", "last"}) {
		t.Fatalf("items are not in selection order: %v", got)
	}
	if out.Items[1].Error.TableauRequestID != "request-2" || out.Items[1].Error.Phase != errs.PhaseVerification || out.Items[1].Error.Outcome != errs.OutcomeUnknown || out.Items[1].Error.Prerequisite == nil || out.Items[1].Result != nil {
		t.Fatal(out.Items[1])
	}
	for _, full := range []bool{false, true} {
		var b bytes.Buffer
		if err := output.RenderWithOptions(&b, out, output.Options{Full: full}); err != nil {
			t.Fatal(err)
		}
		text := b.String()
		if !strings.Contains(text, "compact first") || !strings.Contains(text, "compact last") || !strings.Contains(text, "request-2") || strings.Contains(text, "expanded") != full {
			t.Fatalf("full=%t: %s", full, text)
		}
		if strings.Contains(text, `details: "--full"`) == full {
			t.Fatalf("details marker, full=%t: %s", full, text)
		}
	}
}

// blockingBatch runs twelve items whose execution waits for release, so a test
// can observe how many run at once before letting them finish.
type blockingBatch struct {
	selectors []string
	started   chan string
	release   chan struct{}
	mu        sync.Mutex
	active    int
	peak      int
	calls     []string
}

func newBlockingBatch() *blockingBatch {
	b := &blockingBatch{started: make(chan string, 12), release: make(chan struct{})}
	for i := range 12 {
		b.selectors = append(b.selectors, fmt.Sprintf("item-%02d", i+1))
	}
	return b
}

func (b *blockingBatch) execute(_ context.Context, selector string) (string, error) {
	b.mu.Lock()
	b.active++
	b.peak = max(b.peak, b.active)
	b.calls = append(b.calls, selector)
	b.mu.Unlock()
	b.started <- selector
	<-b.release
	b.mu.Lock()
	b.active--
	b.mu.Unlock()
	if selector == "item-03" {
		return "", errors.New("item-03 failed")
	}
	return "done " + selector, nil
}

// awaitStarts waits for count items to start.
func (b *blockingBatch) awaitStarts(t *testing.T, count int) {
	t.Helper()
	for range count {
		select {
		case <-b.started:
		case <-time.After(5 * time.Second):
			b.mu.Lock()
			defer b.mu.Unlock()
			t.Fatalf("only %d items started concurrently", b.active)
		}
	}
}

func TestRunExecutesEveryItemAtOnceInSelectionOrder(t *testing.T) {
	batch := newBlockingBatch()
	type result struct {
		out contentbatch.Output
		err error
	}
	done := make(chan result, 1)
	go func() {
		out, err := contentbatch.Run(context.Background(), "workbook.publish", batch.selectors, batch.execute)
		done <- result{out, err}
	}()
	batch.awaitStarts(t, len(batch.selectors))
	close(batch.release)
	finished := <-done

	if batch.peak != len(batch.selectors) {
		t.Fatalf("peak concurrency = %d, want %d", batch.peak, len(batch.selectors))
	}
	if finished.err == nil || finished.out.Total != 12 || finished.out.Succeeded != 11 || finished.out.Failed != 1 || finished.out.Status != "partial_failure" {
		t.Fatalf("outcome = %#v, err = %v", finished.out, finished.err)
	}
	for i, item := range finished.out.Items {
		if item.Selector != batch.selectors[i] {
			t.Fatalf("item %d selector = %q, want %q", i, item.Selector, batch.selectors[i])
		}
		if item.Selector == "item-03" {
			if item.Status != "failed" || item.Error == nil || item.Result != nil {
				t.Fatalf("failed item = %#v", item)
			}
			continue
		}
		if item.Status != "succeeded" || item.Result != "done "+item.Selector {
			t.Fatalf("item %d = %#v", i, item)
		}
	}
	help := strings.Join(finished.out.Help, " ")
	if !strings.Contains(help, "in parallel") || !strings.Contains(help, "selection order") || strings.Contains(help, "sequentially") {
		t.Fatalf("help = %q", help)
	}
}

func TestRunCanceledBatchStartsNoItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	var calls []string
	out, err := contentbatch.Run(ctx, "flow.publish", []string{"first", "second"}, func(_ context.Context, selector string) (string, error) {
		calls = append(calls, selector)
		return selector, nil
	})
	if len(calls) != 0 {
		t.Fatalf("calls = %v, want none after cancellation", calls)
	}
	if err == nil || out.Status != "failed" || out.Skipped != 2 {
		t.Fatalf("outcome = %#v, err = %v", out, err)
	}
	for _, item := range out.Items {
		if item.Status != "skipped" || item.Error == nil || !strings.Contains(item.Error.Summary, "canceled") {
			t.Fatalf("item = %#v", item)
		}
	}
}

func TestRunValidatesCompleteSelectionBeforeAnyWork(t *testing.T) {
	oversized := make([]string, contentbatch.MaxItems+1)
	for i := range oversized {
		oversized[i] = fmt.Sprint(i)
	}
	for _, selectors := range [][]string{nil, {"first", ""}, {"first", "first"}, oversized} {
		_, err := contentbatch.Run(context.Background(), "datasource.pull", selectors, func(context.Context, string) (string, error) { t.Fatal("unexpected work"); return "", nil })
		if err == nil || errs.ExitCode(err) != 2 {
			t.Fatalf("validation error = %v", err)
		}
	}
}

func TestValidateReportsNormalizedDuplicateRows(t *testing.T) {
	err := contentbatch.Validate([]string{"wb-1", "  wb-1  "})
	if err == nil || !strings.Contains(err.Error(), "batch item 2 duplicates batch item 1") || !strings.Contains(err.Error(), `"wb-1"`) {
		t.Fatalf("error = %v", err)
	}
}

func TestValidateReportsEmptyRow(t *testing.T) {
	err := contentbatch.Validate([]string{"wb-1", "  "})
	if err == nil || !strings.Contains(err.Error(), "batch item 2 selector must not be empty") {
		t.Fatalf("error = %v", err)
	}
}

func TestRunAllFailuresHaveFailedStatus(t *testing.T) {
	out, err := contentbatch.Run(context.Background(), "datasource.publish", []string{"first", "second"}, func(context.Context, string) (string, error) { return "", errors.New("failed") })
	if err == nil || out.Status != "failed" || out.Failed != 2 {
		t.Fatalf("outcome = %#v, err=%v", out, err)
	}
}
