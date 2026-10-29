// Package contentbatch runs explicitly selected content operations and reports
// their outcomes in selection order.
package contentbatch

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"sync"

	"github.com/ahillspace/tadx/internal/errs"
)

// MaxItems bounds both work and the aggregate result document.
const MaxItems = 100

// Validate checks the complete selection before any operation starts.
func Validate(selectors []string) error {
	if len(selectors) == 0 || len(selectors) > MaxItems {
		return fmt.Errorf("select between 1 and %d items", MaxItems)
	}
	seen := make(map[string]int, len(selectors))
	for index, selector := range selectors {
		normalized := strings.TrimSpace(selector)
		if normalized == "" {
			return fmt.Errorf("batch item %d selector must not be empty", index+1)
		}
		if previous, exists := seen[normalized]; exists {
			return fmt.Errorf("batch item %d duplicates batch item %d after trimming selector %q; choose unique selectors", index+1, previous, normalized)
		}
		seen[normalized] = index + 1
	}
	return nil
}

// Item records one selected operation and its independently rendered result.
type Item struct {
	Selector string        `json:"selector"`
	Status   string        `json:"status"`
	Result   any           `json:"result,omitempty"`
	Error    *errs.Payload `json:"error,omitempty"`
}

// Output preserves every bounded item outcome, including failures and cancellations.
type Output struct {
	Operation string   `json:"operation"`
	Status    string   `json:"status"`
	Total     int      `json:"total"`
	Succeeded int      `json:"succeeded"`
	Failed    int      `json:"failed"`
	Skipped   int      `json:"skipped"`
	Pending   int      `json:"pending,omitzero"`
	Items     []Item   `json:"items"`
	Details   string   `json:"details,omitempty"`
	Help      []string `json:"help"`
}

func (o Output) CompactOutput() any { return o.project(false) }
func (o Output) FullOutput() any    { return o.project(true) }

func (o Output) project(full bool) Output {
	o.Items = append([]Item(nil), o.Items...)
	for i := range o.Items {
		if full {
			if p, ok := o.Items[i].Result.(interface{ FullOutput() any }); ok {
				o.Items[i].Result = p.FullOutput()
			}
			continue
		}
		if p, ok := o.Items[i].Result.(interface{ CompactOutput() any }); ok {
			o.Items[i].Result = p.CompactOutput()
		}
	}
	if full {
		o.Details = ""
	} else {
		o.Details = "--full"
	}
	return o
}

// Run starts every selected item at once and never retries an uncertain
// operation. Results stay in selection order, and observer notifications are
// serialized. A canceled context starts no further item.
func Run[T any](ctx context.Context, operation string, selectors []string, execute func(context.Context, string) (T, error)) (Output, error) {
	if err := Validate(selectors); err != nil {
		return Output{}, &errs.Error{Kind: errs.KindUsage, Operation: operation, Summary: err.Error()}
	}
	out := Output{Operation: operation, Status: "running", Total: len(selectors), Items: make([]Item, len(selectors)), Help: []string{"Items run in parallel; results are listed in selection order. Review failed or skipped items before starting another command; successful items are not retried automatically."}}
	for i, selector := range selectors {
		out.Items[i] = Item{Selector: selector, Status: "queued"}
	}
	recount(&out)
	notify(ctx, out)
	var mu sync.Mutex
	update := func(index int, item Item) {
		mu.Lock()
		defer mu.Unlock()
		out.Items[index] = item
		recount(&out)
		notify(ctx, out)
	}
	var wg sync.WaitGroup
	for index, selector := range selectors {
		if ctx.Err() != nil {
			payload := errs.Structure(&errs.Error{Kind: errs.KindOperation, Operation: operation, Summary: "Item skipped because the batch was canceled."}).Error
			update(index, Item{Selector: selector, Status: "skipped", Error: &payload})
			continue
		}
		update(index, Item{Selector: selector, Status: "running"})
		wg.Go(func() { update(index, runItem(ctx, selector, execute)) })
	}
	wg.Wait()
	if out.Failed+out.Skipped != 0 {
		return out, &errs.Error{Kind: errs.KindOperation, Operation: operation, Summary: "One or more batch items failed or were skipped.", Retryable: errs.Bool(false), CorrectiveAction: "Review the per-item outcomes before retrying individual items."}
	}
	return out, nil
}

func runItem[T any](ctx context.Context, selector string, execute func(context.Context, string) (T, error)) Item {
	result, err := execute(WithObserver(ctx, nil), selector)
	if err != nil {
		payload := errs.Structure(err).Error
		item := Item{Selector: selector, Status: "failed", Error: &payload}
		if value := reflect.ValueOf(result); value.IsValid() && !value.IsZero() {
			item.Result = result
		}
		return item
	}
	return Item{Selector: selector, Status: resultStatus(result), Result: result}
}

func resultStatus(result any) string {
	if state, ok := result.(interface{ OperationStatus() string }); ok {
		switch status := state.OperationStatus(); status {
		case "pending", "running", "accepted", "queued", "unknown":
			return status
		}
	}
	return "succeeded"
}

func recount(out *Output) {
	out.Succeeded, out.Failed, out.Skipped, out.Pending = 0, 0, 0, 0
	for _, item := range out.Items {
		switch item.Status {
		case "succeeded":
			out.Succeeded++
		case "failed":
			out.Failed++
		case "skipped":
			out.Skipped++
		default:
			out.Pending++
		}
	}
	out.Status = "succeeded"
	if out.Pending > 0 {
		out.Status = "running"
	}
	if out.Failed+out.Skipped > 0 {
		out.Status = "partial_failure"
		if out.Succeeded == 0 && out.Pending == 0 {
			out.Status = "failed"
		}
	}
}
