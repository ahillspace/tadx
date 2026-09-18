package contentbatch

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAcceptedJobsSubmitAllItemsBeforeWaiting(t *testing.T) {
	var events []string
	out, err := Run(t.Context(), "workbook.publish", []string{"a", "b"}, func(ctx context.Context, selector string) (string, error) {
		events = append(events, "submit:"+selector)
		if !Bulk(ctx) {
			t.Fatal("two items must enter the pool immediately")
		}
		if !DeferCompletion(ctx, func(context.Context) (any, error) {
			events = append(events, "wait:"+selector)
			if selector == "b" {
				return "accepted:b", errors.New("monitoring interrupted")
			}
			return "confirmed:a", nil
		}) {
			t.Fatal("completion not retained")
		}
		return "accepted:" + selector, nil
	})
	if err == nil || out.Succeeded != 1 || out.Failed != 1 || out.Items[0].Result != "confirmed:a" || out.Items[1].Result != "accepted:b" {
		t.Fatalf("out=%+v err=%v", out, err)
	}
	if !reflect.DeepEqual(events, []string{"submit:a", "submit:b", "wait:a", "wait:b"}) {
		t.Fatal(events)
	}
}

func TestOneItemDoesNotDeferItsFirstMinute(t *testing.T) {
	_, err := Run(t.Context(), "workbook.publish", []string{"a"}, func(ctx context.Context, _ string) (string, error) {
		if Bulk(ctx) || DeferCompletion(ctx, func(context.Context) (any, error) { return nil, nil }) {
			t.Fatal("single item entered bulk lifecycle")
		}
		return "confirmed", nil
	})
	if err != nil {
		t.Fatal(err)
	}
}
