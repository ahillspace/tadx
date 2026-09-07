package contentbatch_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"testing"

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
	var calls []string
	out, err := contentbatch.Run(context.Background(), "workbook.pull", []string{"first", "broken", "last"}, func(_ context.Context, selector string) (projected, error) {
		calls = append(calls, selector)
		if selector == "broken" {
			return projected{}, &errs.Error{Kind: errs.KindOperation, Summary: "download failed", TableauRequestID: "request-2", Retryable: errs.Bool(false)}
		}
		return projected{selector}, nil
	})
	if err == nil || errs.ExitCode(err) == 0 || out.Status != "partial_failure" || out.Total != 3 || out.Succeeded != 2 || out.Failed != 1 {
		t.Fatalf("outcome = %#v, err = %v", out, err)
	}
	if !reflect.DeepEqual(calls, []string{"first", "broken", "last"}) {
		t.Fatal(calls)
	}
	if out.Items[1].Error.TableauRequestID != "request-2" || out.Items[1].Result != nil {
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

func TestRunCancellationDoesNotStartRemainingItems(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	calls := 0
	out, err := contentbatch.Run(ctx, "flow.publish", []string{"first", "second", "third"}, func(context.Context, string) (string, error) {
		calls++
		cancel()
		return "published", nil
	})
	if err == nil || calls != 1 || out.Succeeded != 1 || out.Skipped != 2 || out.Items[2].Status != "skipped" {
		t.Fatalf("outcome = %#v, err=%v, calls=%d", out, err, calls)
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

func TestRunAllFailuresHaveFailedStatus(t *testing.T) {
	out, err := contentbatch.Run(context.Background(), "datasource.publish", []string{"first", "second"}, func(context.Context, string) (string, error) { return "", errors.New("failed") })
	if err == nil || out.Status != "failed" || out.Failed != 2 {
		t.Fatalf("outcome = %#v, err=%v", out, err)
	}
}
