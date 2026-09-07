// Package contentbatch runs explicitly selected content operations in order.
package contentbatch

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/ahillspace/tadx/internal/errs"
)

// MaxItems bounds both work and the aggregate result document.
const MaxItems = 100

// Validate checks the complete selection before any operation starts.
func Validate(selectors []string) error {
	if len(selectors) == 0 || len(selectors) > MaxItems {
		return fmt.Errorf("select between 1 and %d items", MaxItems)
	}
	seen := make(map[string]bool, len(selectors))
	for _, selector := range selectors {
		if strings.TrimSpace(selector) == "" {
			return errors.New("selectors must not be empty")
		}
		if seen[selector] {
			return errors.New("duplicate selectors are not supported")
		}
		seen[selector] = true
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
		} else {
			if p, ok := o.Items[i].Result.(interface{ CompactOutput() any }); ok {
				o.Items[i].Result = p.CompactOutput()
			}
		}
	}
	if full {
		o.Details = ""
	} else {
		o.Details = "--full"
	}
	return o
}

// Run invokes one item at a time and never retries an uncertain operation.
// A canceled context skips remaining items without starting more work.
func Run[T any](ctx context.Context, operation string, selectors []string, execute func(context.Context, string) (T, error)) (Output, error) {
	if err := Validate(selectors); err != nil {
		return Output{}, &errs.Error{Kind: errs.KindUsage, Operation: operation, Summary: err.Error()}
	}
	out := Output{Operation: operation, Status: "succeeded", Total: len(selectors), Items: make([]Item, 0, len(selectors)), Help: []string{"Items run sequentially in selection order. Review failed or skipped items before starting another command; successful items are not retried automatically."}}
	for _, selector := range selectors {
		item := Item{Selector: selector, Status: "succeeded"}
		if err := ctx.Err(); err != nil {
			payload := errs.Structure(&errs.Error{Kind: errs.KindOperation, Operation: operation, Summary: "Item skipped because the batch was canceled."}).Error
			item.Status, item.Error = "skipped", &payload
			out.Skipped++
		} else {
			result, err := execute(ctx, selector)
			if err != nil {
				payload := errs.Structure(err).Error
				item.Status, item.Error = "failed", &payload
				out.Failed++
			} else {
				item.Result = result
				out.Succeeded++
			}
		}
		out.Items = append(out.Items, item)
	}
	if out.Failed+out.Skipped != 0 {
		out.Status = "partial_failure"
		if out.Succeeded == 0 {
			out.Status = "failed"
		}
		return out, &errs.Error{Kind: errs.KindOperation, Operation: operation, Summary: "One or more batch items failed or were skipped.", Retryable: errs.Bool(false), CorrectiveAction: "Review the per-item outcomes before retrying individual items."}
	}
	return out, nil
}
